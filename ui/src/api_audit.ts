/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/api_audit.ts.
 */
import { request } from "./api_client";
import type { AuditLogListResponse } from "./types/audit";

interface ListAuditLogsParams {
  table?: string;
  user_id?: string;
  operation?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

/**
 * TODO: Document listAuditLogs.
 */
export function listAuditLogs(
  params?: ListAuditLogsParams,
): Promise<AuditLogListResponse> {
  const qs = new URLSearchParams();
  if (params?.table) qs.set("table", params.table);
  if (params?.user_id) qs.set("user_id", params.user_id);
  if (params?.operation) qs.set("operation", params.operation);
  if (params?.from) qs.set("from", params.from);
  if (params?.to) qs.set("to", params.to);
  if (params?.limit) qs.set("limit", String(params.limit));
  if (params?.offset) qs.set("offset", String(params.offset));
  const query = qs.toString();
  return request<AuditLogListResponse>(
    `/api/admin/audit${query ? `?${query}` : ""}`,
  );
}
