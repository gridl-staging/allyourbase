// Package backup IntegrityVerifier performs integrity checks on backup data by verifying WAL segment contiguity, WAL object existence in storage, and consistency between backup records and their manifests.
package backup

import (
	"context"
	"fmt"
	"time"
)

type IntegrityVerifier struct {
	repo                Repo
	manifestRepo        ManifestRepo
	walSegmentRepo      WALSegmentRepo
	store               Store
	integrityReportRepo IntegrityReportRepo
	notify              Notifier
	archivePrefix       string
}

// NewIntegrityVerifier constructs an IntegrityVerifier with the given repositories, store, and notifier for verifying backup integrity. If notify is nil, it uses a NoopNotifier.
func NewIntegrityVerifier(
	repo Repo,
	manifestRepo ManifestRepo,
	walSegmentRepo WALSegmentRepo,
	store Store,
	integrityReportRepo IntegrityReportRepo,
	notify Notifier,
	archivePrefix string,
) *IntegrityVerifier {
	if notify == nil {
		notify = NoopNotifier{}
	}
	return &IntegrityVerifier{
		repo:                repo,
		manifestRepo:        manifestRepo,
		walSegmentRepo:      walSegmentRepo,
		store:               store,
		integrityReportRepo: integrityReportRepo,
		notify:              notify,
		archivePrefix:       archivePrefix,
	}
}

// Verify runs all integrity checks for a project and database, including WAL contiguity, WAL object existence, and backup manifest consistency. It returns a VerificationReport with the results, saves the report to the repository, and sends notifications or failure events for any detected issues.
func (v *IntegrityVerifier) Verify(ctx context.Context, projectID, databaseID string) (*VerificationReport, error) {
	report := VerificationReport{
		ProjectID:  projectID,
		DatabaseID: databaseID,
		Status:     "pass",
		VerifiedAt: time.Now().UTC(),
	}

	report.Checks = append(report.Checks, v.checkWALContiguity(ctx, projectID, databaseID)...)
	report.Checks = append(report.Checks, v.checkWALObjectIntegrity(ctx, projectID, databaseID)...)
	report.Checks = append(report.Checks, v.checkBackupManifestIntegrity(ctx, projectID, databaseID)...)

	for _, check := range report.Checks {
		if !check.Passed {
			report.Status = "fail"
			break
		}
	}

	if err := v.integrityReportRepo.Save(ctx, report); err != nil {
		return nil, fmt.Errorf("saving integrity report: %w", err)
	}

	if report.Status == "fail" {
		for _, check := range report.Checks {
			if check.Passed {
				continue
			}
			alertType := ""
			msg := check.Message
			switch check.Name {
			case "wal_contiguity":
				alertType = "wal_gap_detected"
			case "backup_manifest_integrity":
				alertType = "checksum_mismatch"
			}
			if alertType != "" {
				v.notify.OnAlert(ctx, AlertEvent{
					ProjectID:  projectID,
					DatabaseID: databaseID,
					AlertType:  alertType,
					Message:    msg,
					Timestamp:  time.Now().UTC(),
					Metadata: map[string]string{
						"check": check.Name,
					},
				})
			}
		}
		v.notify.OnFailure(ctx, FailureEvent{
			BackupID:  "",
			DBName:    databaseID,
			Stage:     "integrity_verification",
			Err:       fmt.Errorf("integrity check failed for project %s, database %s", projectID, databaseID),
			Timestamp: time.Now().UTC(),
		})
	}

	return &report, nil
}

// checkWALContiguity verifies that the WAL segment chain is contiguous between the latest completed backup's end LSN and the latest WAL segment, detecting any gaps in the write-ahead log sequence.
func (v *IntegrityVerifier) checkWALContiguity(ctx context.Context, projectID, databaseID string) []CheckResult {
	var results []CheckResult

	backups, err := v.repo.ListPhysicalCompleted(ctx, projectID, databaseID)
	if err != nil {
		return []CheckResult{{Name: "wal_contiguity", Passed: false, Message: fmt.Sprintf("failed to list backups: %v", err)}}
	}

	if len(backups) == 0 {
		results = append(results, CheckResult{Name: "wal_contiguity", Passed: true, Message: "no completed physical backups to verify"})
		return results
	}

	latestBackup := backups[0]
	if latestBackup.EndLSN == nil {
		results = append(results, CheckResult{Name: "wal_contiguity", Passed: false, Message: "latest backup missing end_lsn"})
		return results
	}

	latestSegment, err := v.walSegmentRepo.LatestByProject(ctx, projectID, databaseID)
	if err != nil {
		if isNoRowsError(err) {
			results = append(results, CheckResult{Name: "wal_contiguity", Passed: false, Message: "no WAL segments found for verification"})
			return results
		}
		results = append(results, CheckResult{
			Name:    "wal_contiguity",
			Passed:  false,
			Message: fmt.Sprintf("failed to get latest WAL segment: %v", err),
		})
		return results
	}
	if latestSegment == nil {
		results = append(results, CheckResult{Name: "wal_contiguity", Passed: false, Message: "no WAL segments found for verification"})
		return results
	}

	segments, err := v.walSegmentRepo.ListRange(ctx, projectID, databaseID, *latestBackup.EndLSN, latestSegment.EndLSN)
	if err != nil {
		msg := fmt.Sprintf("failed to list WAL segments: %v", err)
		results = append(results, CheckResult{Name: "wal_contiguity", Passed: false, Message: msg})
		return results
	}

	var gaps []string
	for i := 0; i+1 < len(segments); i++ {
		if segments[i].EndLSN != segments[i+1].StartLSN {
			gapMsg := fmt.Sprintf("gap between %s and %s", segments[i].EndLSN, segments[i+1].StartLSN)
			gaps = append(gaps, gapMsg)
		}
	}

	if len(gaps) > 0 {
		msg := fmt.Sprintf("WAL gaps detected: %v", gaps)
		results = append(results, CheckResult{Name: "wal_contiguity", Passed: false, Message: msg})
	} else {
		results = append(results, CheckResult{Name: "wal_contiguity", Passed: true, Message: "WAL chain is contiguous"})
	}

	return results
}

// checkWALObjectIntegrity verifies that all WAL segment objects referenced in the database exist in the backup store.
func (v *IntegrityVerifier) checkWALObjectIntegrity(ctx context.Context, projectID, databaseID string) []CheckResult {
	var results []CheckResult

	segments, err := v.walSegmentRepo.ListRange(ctx, projectID, databaseID, "0/0", "FFFFFFFF/FFFFFFFF")
	if err != nil {
		return []CheckResult{{Name: "wal_object_integrity", Passed: false, Message: fmt.Sprintf("failed to list WAL segments: %v", err)}}
	}

	if len(segments) == 0 {
		results = append(results, CheckResult{Name: "wal_object_integrity", Passed: true, Message: "no WAL segments to verify"})
		return results
	}

	var failures []string
	for _, seg := range segments {
		key := WALSegmentKey(v.archivePrefix, projectID, databaseID, seg.Timeline, seg.SegmentName)
		_, err := v.store.HeadObject(ctx, key)
		if err != nil {
			failures = append(failures, fmt.Sprintf("missing object %s", key))
		}
	}

	if len(failures) > 0 {
		msg := fmt.Sprintf("WAL object integrity failures: %v", failures)
		results = append(results, CheckResult{Name: "wal_object_integrity", Passed: false, Message: msg})
	} else {
		results = append(results, CheckResult{Name: "wal_object_integrity", Passed: true, Message: "All WAL objects exist in S3"})
	}

	return results
}

// checkBackupManifestIntegrity verifies that each completed backup has a corresponding manifest with matching object keys and checksums.
func (v *IntegrityVerifier) checkBackupManifestIntegrity(ctx context.Context, projectID, databaseID string) []CheckResult {
	var results []CheckResult

	backups, err := v.repo.ListPhysicalCompleted(ctx, projectID, databaseID)
	if err != nil {
		return []CheckResult{{Name: "backup_manifest_integrity", Passed: false, Message: fmt.Sprintf("failed to list backups: %v", err)}}
	}

	if len(backups) == 0 {
		results = append(results, CheckResult{Name: "backup_manifest_integrity", Passed: true, Message: "no completed physical backups to verify"})
		return results
	}

	var failures []string
	for _, backup := range backups {
		manifest, err := v.manifestRepo.GetByBackupID(ctx, backup.ID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("backup %s: error fetching manifest: %v", backup.ID, err))
			continue
		}
		if manifest == nil {
			failures = append(failures, fmt.Sprintf("backup %s: manifest not found", backup.ID))
			continue
		}

		if manifest.ObjectKey != backup.ObjectKey {
			msg := fmt.Sprintf("backup %s: object key mismatch (backup: %s, manifest: %s)", backup.ID, backup.ObjectKey, manifest.ObjectKey)
			failures = append(failures, msg)
		}
		if manifest.Checksum != backup.Checksum {
			msg := fmt.Sprintf("backup %s: checksum mismatch (backup: %s, manifest: %s)", backup.ID, backup.Checksum, manifest.Checksum)
			failures = append(failures, msg)
		}
	}

	if len(failures) > 0 {
		msg := fmt.Sprintf("Backup/manifest integrity failures: %v", failures)
		results = append(results, CheckResult{Name: "backup_manifest_integrity", Passed: false, Message: msg})
	} else {
		results = append(results, CheckResult{Name: "backup_manifest_integrity", Passed: true, Message: "All backups have matching manifests"})
	}

	return results
}
