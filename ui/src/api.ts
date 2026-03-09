import type {
  SchemaCache,
  ListResponse,
  SqlResult,
  WebhookResponse,
  WebhookRequest,
  WebhookTestResult,
  DeliveryListResponse,
  UserListResponse,
  AppResponse,
  AppListResponse,
  APIKeyListResponse,
  APIKeyCreateResponse,
  ApiExplorerResponse,
  StorageListResponse,
  StorageObject,
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
  EmailTemplateListResponse,
  EmailTemplateEffective,
  UpsertEmailTemplateRequest,
  UpsertEmailTemplateResponse,
  SetEmailTemplateEnabledResponse,
  PreviewEmailTemplateRequest,
  PreviewEmailTemplateResponse,
  SendTemplateEmailRequest,
  SendTemplateEmailResponse,
  MatviewListResponse,
  MatviewRegistration,
  MatviewRefreshResult,
} from "./types";

const TOKEN_KEY = "ayb_admin_token";

function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

function emitUnauthorized() {
  clearToken();
  window.dispatchEvent(new Event("ayb:unauthorized"));
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    ...(init?.headers as Record<string, string>),
  };
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(path, { ...init, headers });
  if (!res.ok) {
    if (res.status === 401) {
      emitUnauthorized();
    }
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
  return res.json();
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

export async function getAdminStatus(): Promise<{ auth: boolean }> {
  return request("/api/admin/status");
}

export async function adminLogin(password: string): Promise<string> {
  const res = await request<{ token: string }>("/api/admin/auth", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ password }),
  });
  setToken(res.token);
  return res.token;
}

export async function getSchema(): Promise<SchemaCache> {
  return request("/api/schema");
}

export async function getRows(
  table: string,
  params: {
    page?: number;
    perPage?: number;
    sort?: string;
    filter?: string;
    search?: string;
    expand?: string;
  } = {},
): Promise<ListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  if (params.sort) qs.set("sort", params.sort);
  if (params.filter) qs.set("filter", params.filter);
  if (params.search) qs.set("search", params.search);
  if (params.expand) qs.set("expand", params.expand);
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/collections/${table}${suffix}`);
}

export async function createRecord(
  table: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  return request(`/api/collections/${table}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function updateRecord(
  table: string,
  id: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  return request(`/api/collections/${table}/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function executeSQL(query: string): Promise<SqlResult> {
  return request("/api/admin/sql/", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query }),
  });
}

export async function deleteRecord(
  table: string,
  id: string,
): Promise<void> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`/api/collections/${table}/${id}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
}

// --- Batch ---

export interface BatchOperation {
  method: "create" | "update" | "delete";
  id?: string;
  body?: Record<string, unknown>;
}

export interface BatchResult {
  index: number;
  status: number;
  body?: Record<string, unknown>;
}

export async function batchRecords(
  table: string,
  operations: BatchOperation[],
): Promise<BatchResult[]> {
  return request(`/api/collections/${table}/batch`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ operations }),
  });
}

// --- RPC ---

export async function callRpc(
  functionName: string,
  args: Record<string, unknown> = {},
): Promise<{ status: number; data: unknown }> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`/api/rpc/${functionName}`, {
    method: "POST",
    headers,
    body: JSON.stringify(args),
  });
  if (res.status === 204) {
    return { status: 204, data: null };
  }
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
  const data = await res.json();
  return { status: res.status, data };
}

// --- Webhooks ---

export async function listWebhooks(): Promise<WebhookResponse[]> {
  const res = await request<{ items: WebhookResponse[] }>("/api/webhooks");
  return res.items;
}

export async function createWebhook(
  data: WebhookRequest,
): Promise<WebhookResponse> {
  return request("/api/webhooks", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function updateWebhook(
  id: string,
  data: Partial<WebhookRequest>,
): Promise<WebhookResponse> {
  return request(`/api/webhooks/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function testWebhook(id: string): Promise<WebhookTestResult> {
  return request(`/api/webhooks/${id}/test`, { method: "POST" });
}

export async function listWebhookDeliveries(
  webhookId: string,
  params: { page?: number; perPage?: number } = {},
): Promise<DeliveryListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/webhooks/${webhookId}/deliveries${suffix}`);
}

export async function deleteWebhook(id: string): Promise<void> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`/api/webhooks/${id}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
}

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
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`/api/admin/users/${id}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
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
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`/api/admin/api-keys/${id}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
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
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`/api/admin/apps/${id}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
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
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`/api/admin/oauth/clients/${clientId}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
}

export async function rotateOAuthClientSecret(
  clientId: string,
): Promise<OAuthClientRotateSecretResponse> {
  return request(`/api/admin/oauth/clients/${clientId}/rotate-secret`, {
    method: "POST",
  });
}

// --- Storage ---

export async function listStorageFiles(
  bucket: string,
  params: { prefix?: string; limit?: number; offset?: number } = {},
): Promise<StorageListResponse> {
  const qs = new URLSearchParams();
  if (params.prefix) qs.set("prefix", params.prefix);
  if (params.limit) qs.set("limit", String(params.limit));
  if (params.offset) qs.set("offset", String(params.offset));
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/storage/${bucket}${suffix}`);
}

export async function uploadStorageFile(
  bucket: string,
  file: File,
): Promise<StorageObject> {
  const formData = new FormData();
  formData.append("file", file);
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`/api/storage/${bucket}`, {
    method: "POST",
    headers,
    body: formData,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
  return res.json();
}

export async function deleteStorageFile(
  bucket: string,
  name: string,
): Promise<void> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`/api/storage/${bucket}/${name}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
}

export async function getSignedURL(
  bucket: string,
  name: string,
  expiresIn?: number,
): Promise<{ url: string }> {
  return request(`/api/storage/${bucket}/${name}/sign`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(expiresIn ? { expiresIn } : {}),
  });
}

export function storageDownloadURL(bucket: string, name: string): string {
  return `/api/storage/${bucket}/${name}`;
}

// --- API Explorer ---

export async function executeApiExplorer(
  method: string,
  path: string,
  body?: string,
): Promise<ApiExplorerResponse> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  if (body && (method === "POST" || method === "PATCH" || method === "PUT")) {
    headers["Content-Type"] = "application/json";
  }

  const start = performance.now();
  const res = await fetch(path, {
    method,
    headers,
    body: body && (method === "POST" || method === "PATCH" || method === "PUT") ? body : undefined,
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

// --- RLS Policies ---

export interface RlsPolicy {
  tableName: string;
  tableSchema: string;
  policyName: string;
  command: string;
  permissive: string;
  roles: string[];
  usingExpr: string | null;
  withCheckExpr: string | null;
}

export interface RlsTableStatus {
  rlsEnabled: boolean;
  forceRls: boolean;
}

export async function listRlsPolicies(table?: string): Promise<RlsPolicy[]> {
  const path = table ? `/api/admin/rls/${encodeURIComponent(table)}` : "/api/admin/rls";
  return request(path);
}

export async function getRlsStatus(table: string): Promise<RlsTableStatus> {
  return request(`/api/admin/rls/${encodeURIComponent(table)}/status`);
}

export async function createRlsPolicy(data: {
  table: string;
  schema?: string;
  name: string;
  command: string;
  permissive?: boolean;
  roles?: string[];
  using?: string;
  withCheck?: string;
}): Promise<{ message: string }> {
  return request("/api/admin/rls", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteRlsPolicy(
  table: string,
  policy: string,
): Promise<void> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(
    `/api/admin/rls/${encodeURIComponent(table)}/${encodeURIComponent(policy)}`,
    { method: "DELETE", headers },
  );
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
}

export async function enableRls(table: string): Promise<{ message: string }> {
  return request(`/api/admin/rls/${encodeURIComponent(table)}/enable`, {
    method: "POST",
  });
}

export async function disableRls(table: string): Promise<{ message: string }> {
  return request(`/api/admin/rls/${encodeURIComponent(table)}/disable`, {
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
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(`/api/admin/schedules/${id}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
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

// --- Email Templates ---

export async function listEmailTemplates(): Promise<EmailTemplateListResponse> {
  return request("/api/admin/email/templates");
}

export async function getEmailTemplate(
  key: string,
): Promise<EmailTemplateEffective> {
  return request(`/api/admin/email/templates/${encodeURIComponent(key)}`);
}

export async function upsertEmailTemplate(
  key: string,
  data: UpsertEmailTemplateRequest,
): Promise<UpsertEmailTemplateResponse> {
  return request(`/api/admin/email/templates/${encodeURIComponent(key)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteEmailTemplate(key: string): Promise<void> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(`/api/admin/email/templates/${encodeURIComponent(key)}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
}

export async function setEmailTemplateEnabled(
  key: string,
  enabled: boolean,
): Promise<SetEmailTemplateEnabledResponse> {
  return request(`/api/admin/email/templates/${encodeURIComponent(key)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  });
}

export async function previewEmailTemplate(
  key: string,
  data: PreviewEmailTemplateRequest,
): Promise<PreviewEmailTemplateResponse> {
  return request(`/api/admin/email/templates/${encodeURIComponent(key)}/preview`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function sendTemplateEmail(
  data: SendTemplateEmailRequest,
): Promise<SendTemplateEmailResponse> {
  return request("/api/admin/email/send", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
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
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(`/api/admin/matviews/${id}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) emitUnauthorized();
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
}

export async function refreshMatview(id: string): Promise<MatviewRefreshResult> {
  return request(`/api/admin/matviews/${id}/refresh`, { method: "POST" });
}
