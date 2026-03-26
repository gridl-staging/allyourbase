/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/api_backups.ts.
 */
import { request, requestNoBody } from "./api_client";
import type {
  BackupListResponse,
  BackupTriggerResponse,
  PITRValidateResponse,
  RestoreJob,
  RestoreJobStartResponse,
  RestoreJobListResponse,
} from "./types/backups";

interface ListBackupsParams {
  status?: string;
  limit?: number;
  offset?: number;
}

const BACKUPS_PAGE_SIZE = 200;

export function listBackups(params?: ListBackupsParams): Promise<BackupListResponse> {
  const qs = new URLSearchParams();
  if (params?.status) qs.set("status", params.status);
  if (params?.limit) qs.set("limit", String(params.limit));
  if (params?.offset) qs.set("offset", String(params.offset));
  const query = qs.toString();
  return request<BackupListResponse>(`/api/admin/backups${query ? `?${query}` : ""}`);
}

/**
 * TODO: Document listAllBackups.
 */
export async function listAllBackups(
  params?: Pick<ListBackupsParams, "status">,
): Promise<BackupListResponse> {
  const backups: BackupListResponse["backups"] = [];
  let offset = 0;
  let total = 0;

  while (true) {
    const page = await listBackups({
      ...params,
      limit: BACKUPS_PAGE_SIZE,
      offset,
    });
    backups.push(...page.backups);
    total = page.total;
    if (page.backups.length === 0 || backups.length >= total) {
      break;
    }
    offset += page.backups.length;
  }

  return { backups, total };
}

export function triggerBackup(): Promise<BackupTriggerResponse> {
  return request<BackupTriggerResponse>("/api/admin/backups", { method: "POST" });
}

export function validatePITR(
  projectId: string,
  databaseId: string,
  targetTime: string,
): Promise<PITRValidateResponse> {
  return request<PITRValidateResponse>(
    `/api/admin/backups/projects/${projectId}/pitr/validate`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ database_id: databaseId, target_time: targetTime }),
    },
  );
}

/**
 * TODO: Document restorePITR.
 */
export function restorePITR(
  projectId: string,
  databaseId: string,
  targetTime: string,
  dryRun = false,
): Promise<RestoreJobStartResponse | PITRValidateResponse> {
  return request<RestoreJobStartResponse | PITRValidateResponse>(
    `/api/admin/backups/projects/${projectId}/pitr/restore`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        database_id: databaseId,
        target_time: targetTime,
        dry_run: dryRun,
      }),
    },
  );
}

export function listRestoreJobs(
  projectId: string,
  databaseId: string,
): Promise<RestoreJobListResponse> {
  const qs = new URLSearchParams({ database_id: databaseId }).toString();
  return request<RestoreJobListResponse>(
    `/api/admin/backups/projects/${projectId}/pitr/jobs?${qs}`,
  );
}

export function getRestoreJob(jobId: string): Promise<RestoreJob> {
  return request<RestoreJob>(`/api/admin/backups/restore-jobs/${jobId}`);
}

export function abandonRestoreJob(jobId: string): Promise<void> {
  return requestNoBody(`/api/admin/backups/restore-jobs/${jobId}`, {
    method: "DELETE",
  });
}
