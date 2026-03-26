// Package backup This file provides PITR planning logic that validates target recovery times against available backups and WAL segments. RestorePlanner computes the base backup and WAL chain needed to restore a database to a given point in time.
package backup

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RestorePlan is the computed physical PITR execution plan for a target time.
type RestorePlan struct {
	BaseBackup          *BackupRecord
	WALSegments         []WALSegment
	EarliestRecoverable time.Time
	LatestRecoverable   time.Time
	EstimatedWALBytes   int64
}

// RestorePlanner computes whether a target timestamp is recoverable and what
// base backup + WAL chain are required.
type RestorePlanner struct {
	repo         Repo
	walRepo      WALSegmentRepo
	manifestRepo ManifestRepo
}

func NewRestorePlanner(repo Repo, walRepo WALSegmentRepo, manifestRepo ManifestRepo) *RestorePlanner {
	return &RestorePlanner{
		repo:         repo,
		walRepo:      walRepo,
		manifestRepo: manifestRepo,
	}
}

// ValidateWindow checks whether targetTime is within the recoverable window for the given project and database, returning a RestorePlan containing the base backup and WAL segments required to restore to that time, or an error if recovery is not possible.
func (p *RestorePlanner) ValidateWindow(ctx context.Context, projectID, databaseID string, targetTime time.Time) (*RestorePlan, error) {
	backups, err := p.repo.ListPhysicalCompleted(ctx, projectID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("listing completed physical backups: %w", err)
	}
	if len(backups) == 0 {
		return nil, fmt.Errorf("no completed physical backups found for project=%s database=%s", projectID, databaseID)
	}

	earliest, err := earliestCompletedAt(backups)
	if err != nil {
		return nil, err
	}
	if targetTime.Before(earliest) {
		return nil, fmt.Errorf("target time %s is before earliest recoverable time %s", targetTime.UTC().Format(time.RFC3339), earliest.UTC().Format(time.RFC3339))
	}

	latestSegment, err := p.walRepo.LatestByProject(ctx, projectID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("querying latest WAL segment for project=%s database=%s: %w", projectID, databaseID, err)
	}
	if latestSegment == nil {
		return nil, fmt.Errorf("no archived WAL segments found for project=%s database=%s", projectID, databaseID)
	}
	latestRecoverable := latestSegment.ArchivedAt
	if targetTime.After(latestRecoverable) {
		return nil, fmt.Errorf("target time %s is after latest recoverable time %s", targetTime.UTC().Format(time.RFC3339), latestRecoverable.UTC().Format(time.RFC3339))
	}

	baseBackup, err := pickNearestPrecedingBackup(backups, targetTime)
	if err != nil {
		return nil, err
	}
	if baseBackup.EndLSN == nil || *baseBackup.EndLSN == "" {
		return nil, fmt.Errorf("chosen base backup %s has empty end_lsn", baseBackup.ID)
	}

	manifest, err := p.manifestRepo.GetByBackupID(ctx, baseBackup.ID)
	if err != nil {
		return nil, fmt.Errorf("loading manifest for base backup %s: %w", baseBackup.ID, err)
	}
	if manifest == nil {
		return nil, fmt.Errorf("manifest for base backup %s not found", baseBackup.ID)
	}

	segments, err := p.walRepo.ListRange(ctx, projectID, databaseID, *baseBackup.EndLSN, latestSegment.EndLSN)
	if err != nil {
		return nil, fmt.Errorf("listing WAL segments for replay range: %w", err)
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("no WAL segments found between base backup end_lsn %s and latest end_lsn %s", *baseBackup.EndLSN, latestSegment.EndLSN)
	}

	sort.Slice(segments, func(i, j int) bool {
		iLSN, _ := lsnUint64(segments[i].StartLSN)
		jLSN, _ := lsnUint64(segments[j].StartLSN)
		return iLSN < jLSN
	})

	if err := ensureContiguous(*baseBackup.EndLSN, segments); err != nil {
		return nil, err
	}

	if !hasSegmentAtOrAfter(segments, targetTime) {
		return nil, fmt.Errorf("WAL segments do not cover target time %s", targetTime.UTC().Format(time.RFC3339))
	}

	var estimatedWALBytes int64
	for _, segment := range segments {
		estimatedWALBytes += segment.SizeBytes
	}

	plan := &RestorePlan{
		BaseBackup:          baseBackup,
		WALSegments:         segments,
		EarliestRecoverable: earliest,
		LatestRecoverable:   latestRecoverable,
		EstimatedWALBytes:   estimatedWALBytes,
	}
	return plan, nil
}

func pickNearestPrecedingBackup(backups []BackupRecord, targetTime time.Time) (*BackupRecord, error) {
	for i := range backups {
		backup := backups[i]
		if backup.CompletedAt == nil {
			continue
		}
		if !backup.CompletedAt.After(targetTime) {
			selected := backup
			return &selected, nil
		}
	}
	return nil, fmt.Errorf("target time %s is before earliest recoverable backup", targetTime.UTC().Format(time.RFC3339))
}

// earliestCompletedAt returns the earliest completion time among the provided backups, or an error if no backups have a completion timestamp.
func earliestCompletedAt(backups []BackupRecord) (time.Time, error) {
	var earliest *time.Time
	for _, backup := range backups {
		if backup.CompletedAt == nil {
			continue
		}
		if earliest == nil || backup.CompletedAt.Before(*earliest) {
			ts := *backup.CompletedAt
			earliest = &ts
		}
	}
	if earliest == nil {
		return time.Time{}, fmt.Errorf("completed physical backups are missing completed_at timestamps")
	}
	return *earliest, nil
}

func ensureContiguous(expectedStartLSN string, segments []WALSegment) error {
	expected := expectedStartLSN
	for _, seg := range segments {
		eq, err := lsnEqual(seg.StartLSN, expected)
		if err != nil {
			return fmt.Errorf("parsing WAL LSN for contiguity check: %w", err)
		}
		if !eq {
			return fmt.Errorf("WAL gap detected: expected start_lsn %s but got %s in segment %s", expected, seg.StartLSN, seg.SegmentName)
		}
		expected = seg.EndLSN
	}
	return nil
}

func hasSegmentAtOrAfter(segments []WALSegment, targetTime time.Time) bool {
	for _, seg := range segments {
		if !seg.ArchivedAt.Before(targetTime) {
			return true
		}
	}
	return false
}

func lsnEqual(a, b string) (bool, error) {
	aVal, err := lsnUint64(a)
	if err != nil {
		return false, err
	}
	bVal, err := lsnUint64(b)
	if err != nil {
		return false, err
	}
	return aVal == bVal, nil
}

func lsnUint64(lsn string) (uint64, error) {
	parts := strings.Split(lsn, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid LSN %q", lsn)
	}
	high, err := strconv.ParseUint(parts[0], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid high LSN %q: %w", lsn, err)
	}
	low, err := strconv.ParseUint(parts[1], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid low LSN %q: %w", lsn, err)
	}
	return (high << 32) + low, nil
}
