// Package backup PITR retention policies that manage deletion of base backups and WAL segments based on age, compliance requirements, and WAL segment supersession.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// PITRRetentionResult summarises PITR retention activity.
type PITRRetentionResult struct {
	DeletedBackups []string
	DeletedWAL     []string
	Errors         []error
	DryRun         bool
}

// PITRRetentionJob applies WAL-aware PITR retention policies.
type PITRRetentionJob struct {
	cfg           PITRConfig
	store         Store
	repo          Repo
	walRepo       WALSegmentRepo
	manifestRepo  ManifestRepo
	notify        Notifier
	logger        *slog.Logger
	archivePrefix string
	nowFn         func() time.Time
}

// NewPITRRetentionJob builds a PITRRetentionJob.
func NewPITRRetentionJob(
	cfg PITRConfig,
	store Store,
	repo Repo,
	walRepo WALSegmentRepo,
	manifestRepo ManifestRepo,
	notify Notifier,
	logger *slog.Logger,
	archivePrefix string,
) *PITRRetentionJob {
	if notify == nil {
		notify = NoopNotifier{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.EnvironmentClass == "" {
		cfg.EnvironmentClass = "non-prod"
	}
	return &PITRRetentionJob{
		cfg:           cfg,
		store:         store,
		repo:          repo,
		walRepo:       walRepo,
		manifestRepo:  manifestRepo,
		notify:        notify,
		logger:        logger,
		archivePrefix: archivePrefix,
		nowFn:         func() time.Time { return time.Now().UTC() },
	}
}

// Run applies PITR retention for one project/database.
func (j *PITRRetentionJob) Run(ctx context.Context, projectID, databaseID string, dryRun bool) (PITRRetentionResult, error) {
	result := PITRRetentionResult{DryRun: dryRun}
	if !j.cfg.Enabled {
		return result, nil
	}

	backups, err := j.repo.ListPhysicalCompleted(ctx, projectID, databaseID)
	if err != nil {
		return PITRRetentionResult{}, fmt.Errorf("listing physical backups: %w", err)
	}

	walCutoff := j.nowFn().AddDate(0, 0, -j.cfg.WALRetentionDays)
	walSegments, err := j.walRepo.ListOlderThan(ctx, projectID, databaseID, walCutoff)
	if err != nil {
		return PITRRetentionResult{}, fmt.Errorf("listing old WAL segments: %w", err)
	}

	deletableWAL := j.selectDeletableWAL(ctx, walSegments, backups)
	for _, segment := range deletableWAL {
		if dryRun {
			result.DeletedWAL = append(result.DeletedWAL, segment.ID)
			j.logger.Info("dry run: would delete WAL segment", "segment_id", segment.ID, "project", projectID, "database", databaseID)
			continue
		}

		if err := j.deleteWALSegment(ctx, projectID, databaseID, segment); err != nil {
			result.Errors = append(result.Errors, err)
			j.notify.OnFailure(ctx, FailureEvent{
				BackupID:  segment.ID,
				DBName:    databaseID,
				Stage:     "pitr_retention",
				Err:       err,
				Timestamp: j.nowFn(),
			})
			continue
		}
		result.DeletedWAL = append(result.DeletedWAL, segment.ID)
	}

	baseCutoff := j.nowFn().AddDate(0, 0, -j.cfg.BaseBackupRetentionDays)
	deletableBackups := j.selectDeletableBackups(ctx, backups, baseCutoff)
	for _, rec := range deletableBackups {
		if dryRun {
			result.DeletedBackups = append(result.DeletedBackups, rec.ID)
			j.logger.Info("dry run: would delete base backup", "backup_id", rec.ID, "project", projectID, "database", databaseID)
			continue
		}

		if err := j.deleteBaseBackup(ctx, rec); err != nil {
			result.Errors = append(result.Errors, err)
			j.notify.OnFailure(ctx, FailureEvent{
				BackupID:  rec.ID,
				DBName:    databaseID,
				Stage:     "pitr_retention",
				Err:       err,
				Timestamp: j.nowFn(),
			})
			continue
		}
		result.DeletedBackups = append(result.DeletedBackups, rec.ID)
	}

	return result, nil
}

func (j *PITRRetentionJob) selectDeletableWAL(_ context.Context, segments []WALSegment, backups []BackupRecord) []WALSegment {
	if j.cfg.WALRetentionDays <= 0 {
		return nil
	}
	out := make([]WALSegment, 0, len(segments))
	for _, segment := range segments {
		if segment.ID == "" {
			continue
		}
		if j.isSegmentSuperseded(segment, backups) {
			out = append(out, segment)
		}
	}
	return out
}

// isSegmentSuperseded reports whether a WAL segment is superseded by any of the provided base backups. A segment is superseded if a backup's start LSN is greater than or equal to the segment's end LSN, meaning the backup covers all data the segment contains.
func (j *PITRRetentionJob) isSegmentSuperseded(segment WALSegment, backups []BackupRecord) bool {
	if segment.EndLSN == "" {
		return false
	}
	segmentEnd, err := lsnUint64(segment.EndLSN)
	if err != nil {
		return false
	}

	for _, backup := range backups {
		if backup.StartLSN == nil || *backup.StartLSN == "" {
			continue
		}
		backupStart, err := lsnUint64(*backup.StartLSN)
		if err != nil {
			continue
		}
		if backupStart >= segmentEnd {
			return true
		}
	}
	return false
}

// selectDeletableBackups identifies base backups eligible for deletion. It excludes the newest backup, backups newer than the baseCutoff time, and in production environments, backups required by compliance retention. Returns nil if retention is disabled or fewer than two backups exist.
func (j *PITRRetentionJob) selectDeletableBackups(_ context.Context, backups []BackupRecord, baseCutoff time.Time) []BackupRecord {
	if j.cfg.BaseBackupRetentionDays <= 0 {
		return nil
	}
	if len(backups) <= 1 {
		return nil
	}

	copies := append([]BackupRecord(nil), backups...)
	sort.Slice(copies, func(i, k int) bool {
		if copies[i].CompletedAt == nil {
			return false
		}
		if copies[k].CompletedAt == nil {
			return true
		}
		return copies[i].CompletedAt.Before(*copies[k].CompletedAt)
	})

	keepByCompliance := j.complianceBackupIDs(copies)
	out := make([]BackupRecord, 0)
	for i, rec := range copies {
		if i == len(copies)-1 {
			// never delete the newest backup.
			continue
		}
		if rec.CompletedAt == nil {
			continue
		}
		if rec.CompletedAt.After(baseCutoff) {
			continue
		}
		if strings.EqualFold(j.cfg.EnvironmentClass, "prod") && keepByCompliance[rec.ID] {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// complianceBackupIDs returns a set of backup IDs that must be retained for compliance purposes. Returns an empty map if not in production or compliance retention is disabled. In production, it keeps at least one backup per month, looking back ComplianceSnapshotMonths from the most recent backup.
func (j *PITRRetentionJob) complianceBackupIDs(backups []BackupRecord) map[string]bool {
	result := map[string]bool{}
	if !strings.EqualFold(j.cfg.EnvironmentClass, "prod") || j.cfg.ComplianceSnapshotMonths <= 0 {
		return result
	}
	if len(backups) == 0 {
		return result
	}

	latest := backups[0].CompletedAt
	for _, backup := range backups {
		if backup.CompletedAt == nil {
			continue
		}
		if latest == nil || backup.CompletedAt.After(*latest) {
			latest = backup.CompletedAt
		}
	}
	if latest == nil {
		return result
	}

	retentionCutoff := firstOfMonth(*latest).AddDate(0, -j.cfg.ComplianceSnapshotMonths+1, 0)
	seenMonth := map[string]bool{}
	for _, backup := range backups {
		if backup.CompletedAt == nil {
			continue
		}
		if backup.CompletedAt.Before(retentionCutoff) {
			continue
		}
		month := firstOfMonth(*backup.CompletedAt).Format("2006-01")
		if seenMonth[month] {
			continue
		}
		seenMonth[month] = true
		result[backup.ID] = true
	}
	return result
}

func firstOfMonth(ts time.Time) time.Time {
	t := ts.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func (j *PITRRetentionJob) deleteWALSegment(ctx context.Context, projectID, databaseID string, segment WALSegment) error {
	key := WALSegmentKey(j.archivePrefix, projectID, databaseID, segment.Timeline, segment.SegmentName)
	if err := j.store.DeleteObject(ctx, key); err != nil {
		return fmt.Errorf("delete wal object %q: %w", key, err)
	}
	if err := j.walRepo.Delete(ctx, segment.ID); err != nil {
		return fmt.Errorf("delete wal metadata %q: %w", segment.ID, err)
	}
	return nil
}

func (j *PITRRetentionJob) deleteBaseBackup(ctx context.Context, rec BackupRecord) error {
	if rec.ObjectKey == "" {
		return fmt.Errorf("backup %q has empty object key", rec.ID)
	}
	if err := j.store.DeleteObject(ctx, rec.ObjectKey); err != nil {
		return fmt.Errorf("delete base backup object %q: %w", rec.ObjectKey, err)
	}
	if err := j.repo.UpdateStatus(ctx, rec.ID, StatusDeleted, ""); err != nil {
		return fmt.Errorf("mark base backup %q deleted: %w", rec.ID, err)
	}
	return nil
}
