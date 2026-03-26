/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/api_admin.ts.
 */
import type {
  UserListResponse,
  AppResponse,
  AppListResponse,
  APIKeyListResponse,
  APIKeyCreateResponse,
  ApiExplorerResponse,
  SMSHealthResponse,
  SMSMessageListResponse,
  SMSSendResponse,
  OAuthClientResponse,
  OAuthClientListResponse,
  OAuthClientCreateResponse,
  OAuthClientRotateSecretResponse,
  JobListResponse,
  JobResponse,
  QueueStats,
  ScheduleListResponse,
  ScheduleResponse,
  CreateScheduleRequest,
  UpdateScheduleRequest,
  MatviewListResponse,
  MatviewRegistration,
  MatviewRefreshResult,
  BranchRecord,
  RealtimeInspectorSnapshot,
  RealtimeSubscriptionRow,
  SecurityAdvisorReport,
  PerformanceAdvisorReport,
  DashboardTimeRange,
} from "./types";
import {
  request,
  requestNoBody,
  fetchAdmin,
} from "./api_client";

// --- Admin Users ---

export async function listUsers(
  params: { page?: number; perPage?: number; search?: string } = {},
): Promise<UserListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  if (params.search) qs.set("search", params.search);
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/users${suffix}`);
}

export async function deleteUser(id: string): Promise<void> {
  return requestNoBody(`/api/admin/users/${id}`, {
    method: "DELETE",
  });
}

// --- API Keys ---

export async function listApiKeys(
  params: { page?: number; perPage?: number } = {},
): Promise<APIKeyListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/api-keys${suffix}`);
}

export async function createApiKey(data: {
  userId: string;
  name: string;
  scope?: string;
  allowedTables?: string[];
  appId?: string;
}): Promise<APIKeyCreateResponse> {
  return request("/api/admin/api-keys", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function revokeApiKey(id: string): Promise<void> {
  return requestNoBody(`/api/admin/api-keys/${id}`, {
    method: "DELETE",
  });
}

// --- Apps ---

export async function listApps(
  params: { page?: number; perPage?: number } = {},
): Promise<AppListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/apps${suffix}`);
}

export async function createApp(data: {
  name: string;
  description?: string;
  ownerUserId: string;
}): Promise<AppResponse> {
  return request("/api/admin/apps", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function updateApp(
  id: string,
  data: {
    name: string;
    description?: string;
    rateLimitRps?: number;
    rateLimitWindowSeconds?: number;
  },
): Promise<AppResponse> {
  return request(`/api/admin/apps/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteApp(id: string): Promise<void> {
  return requestNoBody(`/api/admin/apps/${id}`, {
    method: "DELETE",
  });
}

// --- OAuth Consent ---

export interface OAuthConsentPrompt {
  requires_consent: boolean;
  redirect_to?: string;
  client_id: string;
  client_name: string;
  redirect_uri: string;
  scope: string;
  state: string;
  code_challenge: string;
  code_challenge_method: string;
  allowed_tables?: string[];
}

export interface OAuthConsentResult {
  redirect_to: string;
}

export async function checkOAuthAuthorize(
  params: URLSearchParams,
): Promise<OAuthConsentPrompt> {
  return request(`/api/auth/authorize?${params.toString()}`);
}

/**
 * Submits user approval or denial of an OAuth authorization request, completing the OAuth code flow. @param data - Consent decision (approve or deny) with OAuth parameters: client ID, redirect URI, scope, code challenge, and optional table access restrictions. @returns Object containing the redirect URL for the post-consent OAuth flow.
 */
export async function submitOAuthConsent(data: {
  decision: "approve" | "deny";
  response_type: string;
  client_id: string;
  redirect_uri: string;
  scope: string;
  state: string;
  code_challenge: string;
  code_challenge_method: string;
  allowed_tables?: string[];
}): Promise<OAuthConsentResult> {
  return request("/api/auth/authorize/consent", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

// --- OAuth Clients ---

export async function listOAuthClients(
  params: { page?: number; perPage?: number } = {},
): Promise<OAuthClientListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/oauth/clients${suffix}`);
}

export async function createOAuthClient(data: {
  appId: string;
  name: string;
  clientType: string;
  redirectUris: string[];
  scopes: string[];
}): Promise<OAuthClientCreateResponse> {
  return request("/api/admin/oauth/clients", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function updateOAuthClient(
  clientId: string,
  data: { name: string; redirectUris: string[]; scopes: string[] },
): Promise<OAuthClientResponse> {
  return request(`/api/admin/oauth/clients/${clientId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function revokeOAuthClient(clientId: string): Promise<void> {
  return requestNoBody(`/api/admin/oauth/clients/${clientId}`, {
    method: "DELETE",
  });
}

export async function rotateOAuthClientSecret(
  clientId: string,
): Promise<OAuthClientRotateSecretResponse> {
  return request(`/api/admin/oauth/clients/${clientId}/rotate-secret`, {
    method: "POST",
  });
}

// --- SMS ---

export async function getSMSHealth(): Promise<SMSHealthResponse> {
  return request("/api/admin/sms/health");
}

export async function listAdminSMSMessages(
  params: { page?: number; perPage?: number } = {},
): Promise<SMSMessageListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/sms/messages${suffix}`);
}

export async function adminSendSMS(
  to: string,
  body: string,
): Promise<SMSSendResponse> {
  return request("/api/admin/sms/send", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ to, body }),
  });
}

// --- Job Queue ---

export async function listJobs(params: {
  state?: string;
  type?: string;
  limit?: number;
  offset?: number;
} = {}): Promise<JobListResponse> {
  const qs = new URLSearchParams();
  if (params.state) qs.set("state", params.state);
  if (params.type) qs.set("type", params.type);
  if (params.limit) qs.set("limit", String(params.limit));
  if (params.offset) qs.set("offset", String(params.offset));
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/jobs${suffix}`);
}

export async function getJob(id: string): Promise<JobResponse> {
  return request(`/api/admin/jobs/${id}`);
}

export async function retryJob(id: string): Promise<JobResponse> {
  return request(`/api/admin/jobs/${id}/retry`, { method: "POST" });
}

export async function cancelJob(id: string): Promise<JobResponse> {
  return request(`/api/admin/jobs/${id}/cancel`, { method: "POST" });
}

export async function getQueueStats(): Promise<QueueStats> {
  return request("/api/admin/jobs/stats");
}

export async function listSchedules(): Promise<ScheduleListResponse> {
  return request("/api/admin/schedules");
}

export async function createSchedule(
  data: CreateScheduleRequest,
): Promise<ScheduleResponse> {
  return request("/api/admin/schedules", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function updateSchedule(
  id: string,
  data: UpdateScheduleRequest,
): Promise<ScheduleResponse> {
  return request(`/api/admin/schedules/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteSchedule(id: string): Promise<void> {
  return requestNoBody(`/api/admin/schedules/${id}`, {
    method: "DELETE",
  });
}

export async function enableSchedule(
  id: string,
): Promise<ScheduleResponse> {
  return request(`/api/admin/schedules/${id}/enable`, { method: "POST" });
}

export async function disableSchedule(
  id: string,
): Promise<ScheduleResponse> {
  return request(`/api/admin/schedules/${id}/disable`, { method: "POST" });
}

// --- Materialized Views ---

export async function listMatviews(): Promise<MatviewListResponse> {
  return request("/api/admin/matviews");
}

export async function registerMatview(data: {
  schema: string;
  viewName: string;
  refreshMode: string;
}): Promise<MatviewRegistration> {
  return request("/api/admin/matviews", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function updateMatview(
  id: string,
  data: { refreshMode: string },
): Promise<MatviewRegistration> {
  return request(`/api/admin/matviews/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteMatview(id: string): Promise<void> {
  return requestNoBody(`/api/admin/matviews/${id}`, {
    method: "DELETE",
  });
}

export async function refreshMatview(id: string): Promise<MatviewRefreshResult> {
  return request(`/api/admin/matviews/${id}/refresh`, { method: "POST" });
}

// --- Branches ---

export async function listBranches(): Promise<BranchRecord[]> {
  const res = await request<{ branches: BranchRecord[] }>("/api/admin/branches");
  return res.branches;
}

export async function createBranch(name: string, from?: string): Promise<BranchRecord> {
  return request("/api/admin/branches", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, from }),
  });
}

export async function deleteBranch(name: string): Promise<void> {
  return requestNoBody(`/api/admin/branches/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

// --- API Explorer ---

/**
 * Executes an HTTP request via the API explorer endpoint, measuring duration and capturing response metadata. @param method - HTTP method (GET, POST, PUT, PATCH, DELETE, etc.). @param path - Request path. @param body - Optional JSON request body; sent only for POST, PATCH, PUT methods. @returns Response data including status code, status text, response headers map, body as text, and request duration in milliseconds.
 */
export async function executeApiExplorer(
  method: string,
  path: string,
  body?: string,
): Promise<ApiExplorerResponse> {
  const requestBody =
    body && (method === "POST" || method === "PATCH" || method === "PUT") ? body : undefined;
  const headers = requestBody ? { "Content-Type": "application/json" } : undefined;

  const start = performance.now();
  const res = await fetchAdmin(path, {
    method,
    headers,
    body: requestBody,
  });
  const durationMs = Math.round(performance.now() - start);

  const responseHeaders: Record<string, string> = {};
  res.headers.forEach((value, key) => {
    responseHeaders[key] = value;
  });

  const responseBody = await res.text();

  return {
    status: res.status,
    statusText: res.statusText,
    headers: responseHeaders,
    body: responseBody,
    durationMs,
  };
}

// --- Realtime Inspector + Advisors ---

/** Raw backend shape from /api/admin/realtime/stats (realtime.Snapshot). */
interface RawRealtimeSnapshot {
  version: string;
  timestamp: string;
  connections: { sse: number; ws: number; total: number };
  subscriptions: {
    tables: Record<string, number>;
    channels: {
      broadcast: Record<string, number>;
      presence: Record<string, number>;
    };
  };
  counters: { dropped_messages: number; heartbeat_failures: number };
}

/**
 * TODO: Document normalizeRealtimeSnapshot.
 */
function normalizeRealtimeSnapshot(raw: RawRealtimeSnapshot): RealtimeInspectorSnapshot {
  const rows: RealtimeSubscriptionRow[] = [];
  for (const [name, count] of Object.entries(raw.subscriptions.tables)) {
    rows.push({ name, type: "table", count });
  }
  for (const [name, count] of Object.entries(raw.subscriptions.channels.broadcast)) {
    rows.push({ name, type: "broadcast", count });
  }
  for (const [name, count] of Object.entries(raw.subscriptions.channels.presence)) {
    rows.push({ name, type: "presence", count });
  }
  return {
    version: raw.version,
    timestamp: raw.timestamp,
    connections: raw.connections,
    subscriptions: rows,
    counters: {
      droppedMessages: raw.counters.dropped_messages,
      heartbeatFailures: raw.counters.heartbeat_failures,
    },
  };
}

export async function getRealtimeInspectorSnapshot(): Promise<RealtimeInspectorSnapshot> {
  const raw = await request<RawRealtimeSnapshot>("/api/admin/realtime/stats");
  return normalizeRealtimeSnapshot(raw);
}

export async function getSecurityAdvisorReport(params: {
  range?: DashboardTimeRange;
} = {}): Promise<SecurityAdvisorReport> {
  const qs = new URLSearchParams();
  if (params.range) qs.set("range", params.range);
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/advisors/security${suffix}`);
}

export async function getPerformanceAdvisorReport(params: {
  range?: DashboardTimeRange;
} = {}): Promise<PerformanceAdvisorReport> {
  const qs = new URLSearchParams();
  if (params.range) qs.set("range", params.range);
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/advisors/performance${suffix}`);
}
