// Package backup ManifestWriter handles writing backup manifests to object storage and metadata repositories. It validates backup records and implements conflict detection for existing manifests.
package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ManifestWriter struct {
	store          Store
	manifestRepo   ManifestRepo
	walSegmentRepo WALSegmentRepo
	cfg            PITRConfig
}

func NewManifestWriter(store Store, manifestRepo ManifestRepo, walSegmentRepo WALSegmentRepo, cfg PITRConfig) *ManifestWriter {
	return &ManifestWriter{
		store:          store,
		manifestRepo:   manifestRepo,
		walSegmentRepo: walSegmentRepo,
		cfg:            cfg,
	}
}

// WriteForBackup validates a backup record, resolves its timeline from WAL segments, and persists the backup manifest to object storage and the manifest repository. It returns an error if validation fails, no covering segment is found, or a conflicting manifest already exists for the same backup ID.
func (m *ManifestWriter) WriteForBackup(ctx context.Context, record *BackupRecord) error {
	if err := m.validateRecord(record); err != nil {
		return fmt.Errorf("validating backup record: %w", err)
	}

	seg, err := m.walSegmentRepo.CoveringSegment(ctx, record.ProjectID, record.DatabaseID, *record.StartLSN)
	if err != nil {
		return fmt.Errorf("resolving timeline for manifest: %w", err)
	}
	if seg == nil {
		return fmt.Errorf("no WAL segment covering start_lsn %s for timeline resolution", *record.StartLSN)
	}

	createdAt := time.Now().UTC()
	manifest := BackupManifest{
		BackupID:   record.ID,
		ProjectID:  record.ProjectID,
		DatabaseID: record.DatabaseID,
		ObjectKey:  record.ObjectKey,
		StartLSN:   *record.StartLSN,
		EndLSN:     *record.EndLSN,
		Checksum:   record.Checksum,
		Timeline:   seg.Timeline,
		CreatedAt:  createdAt,
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("serializing manifest to JSON: %w", err)
	}

	manifestKey := ManifestKey(m.cfg.ArchivePrefix, record.ProjectID, record.DatabaseID, createdAt)

	existingManifest, err := m.manifestRepo.GetByBackupID(ctx, record.ID)
	if err != nil {
		return fmt.Errorf("checking existing manifest: %w", err)
	}

	if existingManifest != nil {
		if existingManifest.ObjectKey == manifest.ObjectKey &&
			existingManifest.StartLSN == manifest.StartLSN &&
			existingManifest.EndLSN == manifest.EndLSN &&
			existingManifest.Checksum == manifest.Checksum &&
			existingManifest.Timeline == manifest.Timeline {
			return nil
		}
		return fmt.Errorf("manifest content mismatch for backup %s: existing manifest has different values", record.ID)
	}

	if err := m.store.PutObject(ctx, manifestKey, bytes.NewReader(manifestJSON), int64(len(manifestJSON)), "application/json"); err != nil {
		return fmt.Errorf("uploading manifest to S3: %w", err)
	}

	if err := m.manifestRepo.Upsert(ctx, manifest); err != nil {
		return fmt.Errorf("persisting manifest metadata: %w", err)
	}

	return nil
}

// validateRecord ensures the backup record contains all required fields: ObjectKey, StartLSN, EndLSN, Checksum, ProjectID, and DatabaseID. It returns an error listing the names of any missing fields.
func (m *ManifestWriter) validateRecord(record *BackupRecord) error {
	var missing []string
	if record.ObjectKey == "" {
		missing = append(missing, "ObjectKey")
	}
	if record.StartLSN == nil || *record.StartLSN == "" {
		missing = append(missing, "StartLSN")
	}
	if record.EndLSN == nil || *record.EndLSN == "" {
		missing = append(missing, "EndLSN")
	}
	if record.Checksum == "" {
		missing = append(missing, "Checksum")
	}
	if record.ProjectID == "" {
		missing = append(missing, "ProjectID")
	}
	if record.DatabaseID == "" {
		missing = append(missing, "DatabaseID")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}
