/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/api_analytics.ts.
 */
import { request } from "./api_client";
import type {
  RequestLogListResponse,
  QueryAnalyticsResponse,
} from "./types/analytics";

interface ListRequestLogsParams {
  method?: string;
  path?: string;
  status?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

/**
 * TODO: Document listRequestLogs.
 */
export function listRequestLogs(
  params?: ListRequestLogsParams,
): Promise<RequestLogListResponse> {
  const qs = new URLSearchParams();
  if (params?.method) qs.set("method", params.method);
  if (params?.path) qs.set("path", params.path);
  if (params?.status) qs.set("status", params.status);
  if (params?.from) qs.set("from", params.from);
  if (params?.to) qs.set("to", params.to);
  if (params?.limit) qs.set("limit", String(params.limit));
  if (params?.offset) qs.set("offset", String(params.offset));
  const query = qs.toString();
  return request<RequestLogListResponse>(
    `/api/admin/analytics/requests${query ? `?${query}` : ""}`,
  );
}

interface ListQueryStatsParams {
  sort?: string;
  limit?: number;
}

export function listQueryStats(
  params?: ListQueryStatsParams,
): Promise<QueryAnalyticsResponse> {
  const qs = new URLSearchParams();
  if (params?.sort) qs.set("sort", params.sort);
  if (params?.limit) qs.set("limit", String(params.limit));
  const query = qs.toString();
  return request<QueryAnalyticsResponse>(
    `/api/admin/analytics/queries${query ? `?${query}` : ""}`,
  );
}
