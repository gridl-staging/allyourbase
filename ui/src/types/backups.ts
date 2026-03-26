/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/types/backups.ts.
 */
/**
 * TODO: Document BackupRecord.
 */
export interface BackupRecord {
  id: string;
  db_name: string;
  object_key: string;
  size_bytes: number;
  checksum: string;
  started_at: string;
  completed_at?: string;
  status: string;
  error_message?: string;
  triggered_by: string;
  restore_source_id?: string;
  backup_type: string;
  start_lsn?: string;
  end_lsn?: string;
  project_id?: string;
  database_id?: string;
}

export interface BackupListResponse {
  backups: BackupRecord[];
  total: number;
}

export interface BackupTriggerResponse {
  backup_id: string;
  status: string;
}

/**
 * TODO: Document RestoreJob.
 */
export interface RestoreJob {
  id: string;
  project_id: string;
  database_id: string;
  environment: string;
  target_time: string;
  base_backup_id: string;
  wal_segments_needed: number;
  verification_result?: unknown;
  logs: string;
  requested_by: string;
  status: string;
  phase: string;
  started_at: string;
  completed_at?: string;
  error_message: string;
}

export interface RestoreJobListResponse {
  jobs: RestoreJob[];
  count: number;
}

export interface RestoreJobStartResponse {
  job_id: string;
  status: string;
  phase: string;
}

export interface WALSegment {
  name: string;
  size_bytes: number;
  modified_at: string;
}

export interface PITRValidateResponse {
  base_backup?: BackupRecord;
  earliest_recoverable: string;
  latest_recoverable: string;
  estimated_wal_bytes: number;
  wal_segments_count: number;
}

export interface RestorePlan {
  target_time: string;
  backup_id: string;
  wal_segments_needed: number;
  estimated_duration_sec: number;
}
