-- Migration 134: Add UNIQUE constraint on backup_id in _ayb_backup_manifests.
-- This ensures one manifest per physical backup. The idempotent create
-- in PgManifestRepo depends on this constraint for ON CONFLICT semantics.

ALTER TABLE _ayb_backup_manifests
ADD CONSTRAINT _ayb_backup_manifests_backup_id_key UNIQUE (backup_id);
