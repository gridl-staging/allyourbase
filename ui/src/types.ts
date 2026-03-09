// Schema types matching Go's schema.SchemaCache JSON output.

export interface SchemaCache {
  tables: Record<string, Table>;
  functions?: Record<string, SchemaFunction>;
  schemas: string[];
  builtAt: string;
}

export interface Table {
  schema: string;
  name: string;
  kind: string;
  comment?: string;
  columns: Column[];
  primaryKey: string[];
  foreignKeys?: ForeignKey[];
  indexes?: Index[];
  relationships?: Relationship[];
}

export interface Column {
  name: string;
  position: number;
  type: string;
  nullable: boolean;
  default?: string;
  comment?: string;
  isPrimaryKey: boolean;
  jsonType: string;
  enumValues?: string[];
}

export interface ForeignKey {
  constraintName: string;
  columns: string[];
  referencedSchema: string;
  referencedTable: string;
  referencedColumns: string[];
  onUpdate?: string;
  onDelete?: string;
}

export interface Index {
  name: string;
  isUnique: boolean;
  isPrimary: boolean;
  method: string;
  definition: string;
}

export interface Relationship {
  name: string;
  type: string;
  fromSchema: string;
  fromTable: string;
  fromColumns: string[];
  toSchema: string;
  toTable: string;
  toColumns: string[];
  fieldName: string;
}

// SQL editor response.
export interface SqlResult {
  columns: string[];
  rows: unknown[][];
  rowCount: number;
  durationMs: number;
}

// API list response envelope.
export interface ListResponse {
  items: Record<string, unknown>[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

// Webhook CRUD types (matches Go webhookResponse).
export interface WebhookResponse {
  id: string;
  url: string;
  hasSecret: boolean;
  events: string[];
  tables: string[];
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface WebhookRequest {
  url: string;
  secret?: string;
  events?: string[];
  tables?: string[];
  enabled?: boolean;
}

export interface WebhookTestResult {
  success: boolean;
  statusCode?: number;
  durationMs: number;
  error?: string;
}

// Webhook delivery log types.
export interface WebhookDelivery {
  id: string;
  webhookId: string;
  eventAction: string;
  eventTable: string;
  success: boolean;
  statusCode: number;
  attempt: number;
  durationMs: number;
  error?: string;
  requestBody?: string;
  responseBody?: string;
  deliveredAt: string;
}

export interface DeliveryListResponse {
  items: WebhookDelivery[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

// Admin user management types.
export interface AdminUser {
  id: string;
  email: string;
  emailVerified: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface UserListResponse {
  items: AdminUser[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

// RPC function types (matches Go schema.Function).
export interface FuncParam {
  name: string;
  type: string;
  position: number;
}

export interface SchemaFunction {
  schema: string;
  name: string;
  comment?: string;
  parameters: FuncParam[] | null;
  returnType: string;
  returnsSet: boolean;
  isVoid: boolean;
}

// App types (matches Go auth.App).
export interface AppResponse {
  id: string;
  name: string;
  description: string;
  ownerUserId: string;
  rateLimitRps: number;
  rateLimitWindowSeconds: number;
  createdAt: string;
  updatedAt: string;
}

export interface AppListResponse {
  items: AppResponse[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

// API key types.
export interface APIKeyResponse {
  id: string;
  userId: string;
  name: string;
  keyPrefix: string;
  scope: string;
  allowedTables: string[] | null;
  appId: string | null;
  lastUsedAt: string | null;
  expiresAt: string | null;
  createdAt: string;
  revokedAt: string | null;
}

export interface APIKeyListResponse {
  items: APIKeyResponse[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export interface APIKeyCreateResponse {
  key: string;
  apiKey: APIKeyResponse;
}

// API Explorer types.
export interface ApiExplorerRequest {
  method: string;
  path: string;
  body?: string;
}

export interface ApiExplorerResponse {
  status: number;
  statusText: string;
  headers: Record<string, string>;
  body: string;
  durationMs: number;
}

export interface ApiExplorerHistoryEntry {
  method: string;
  path: string;
  body?: string;
  status: number;
  durationMs: number;
  timestamp: string;
}

// RLS policy types.
export interface RlsPolicy {
  tableSchema: string;
  tableName: string;
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

// SMS types.
export interface SMSWindowStats {
  sent: number;
  confirmed: number;
  failed: number;
  conversion_rate: number;
}

export interface SMSHealthResponse {
  today: SMSWindowStats;
  last_7d: SMSWindowStats;
  last_30d: SMSWindowStats;
  warning?: string;
}

export interface SMSMessage {
  id: string;
  to: string;
  body: string;
  provider: string;
  message_id: string;
  status: string;
  created_at: string;
  updated_at: string;
  error_message?: string;
  user_id?: string;
}

export interface SMSMessageListResponse {
  items: SMSMessage[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export interface SMSSendResponse {
  id?: string;
  message_id: string;
  status: string;
  to: string;
}

// OAuth client types (matches Go auth.OAuthClient).
export interface OAuthClientResponse {
  id: string;
  appId: string;
  clientId: string;
  name: string;
  redirectUris: string[];
  scopes: string[];
  clientType: string;
  createdAt: string;
  updatedAt: string;
  revokedAt: string | null;
  activeAccessTokenCount: number;
  activeRefreshTokenCount: number;
  totalGrants: number;
  lastTokenIssuedAt: string | null;
}

export interface OAuthClientListResponse {
  items: OAuthClientResponse[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export interface OAuthClientCreateResponse {
  clientSecret: string;
  client: OAuthClientResponse;
}

export interface OAuthClientRotateSecretResponse {
  clientSecret: string;
}

// Job queue types (matches Go jobs.Job / jobs.Schedule).
export type JobState = "queued" | "running" | "completed" | "failed" | "canceled";

export interface JobResponse {
  id: string;
  type: string;
  payload: Record<string, unknown>;
  state: JobState;
  runAt: string;
  leaseUntil: string | null;
  workerId: string | null;
  attempts: number;
  maxAttempts: number;
  lastError: string | null;
  lastRunAt: string | null;
  idempotencyKey: string | null;
  scheduleId: string | null;
  createdAt: string;
  updatedAt: string;
  completedAt: string | null;
  canceledAt: string | null;
}

export interface JobListResponse {
  items: JobResponse[];
  count: number;
}

export interface QueueStats {
  queued: number;
  running: number;
  completed: number;
  failed: number;
  canceled: number;
  oldestQueuedAgeSec: number | null;
}

export interface ScheduleResponse {
  id: string;
  name: string;
  jobType: string;
  payload: Record<string, unknown>;
  cronExpr: string;
  timezone: string;
  enabled: boolean;
  maxAttempts: number;
  nextRunAt: string | null;
  lastRunAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface ScheduleListResponse {
  items: ScheduleResponse[];
  count: number;
}

export interface CreateScheduleRequest {
  name: string;
  jobType: string;
  cronExpr: string;
  timezone: string;
  payload?: Record<string, unknown>;
  enabled?: boolean;
  maxAttempts?: number;
}

export interface UpdateScheduleRequest {
  cronExpr?: string;
  timezone?: string;
  payload?: Record<string, unknown>;
  enabled?: boolean;
}

// Email templates types (matches admin email template handlers).
export type EmailTemplateSource = "builtin" | "custom";

export interface EmailTemplateListItem {
  templateKey: string;
  source: EmailTemplateSource;
  subjectTemplate: string;
  enabled: boolean;
  updatedAt?: string;
}

export interface EmailTemplateListResponse {
  items: EmailTemplateListItem[];
  count: number;
}

export interface EmailTemplateEffective {
  source: EmailTemplateSource;
  templateKey: string;
  subjectTemplate: string;
  htmlTemplate: string;
  enabled: boolean;
  variables?: string[];
}

export interface UpsertEmailTemplateRequest {
  subjectTemplate: string;
  htmlTemplate: string;
}

export interface UpsertEmailTemplateResponse {
  templateKey: string;
  subjectTemplate: string;
  htmlTemplate: string;
  enabled: boolean;
}

export interface SetEmailTemplateEnabledResponse {
  templateKey: string;
  enabled: boolean;
}

export interface PreviewEmailTemplateRequest {
  subjectTemplate: string;
  htmlTemplate: string;
  variables: Record<string, string>;
}

export interface PreviewEmailTemplateResponse {
  subject: string;
  html: string;
  text: string;
}

export interface SendTemplateEmailRequest {
  templateKey: string;
  to: string;
  variables: Record<string, string>;
}

export interface SendTemplateEmailResponse {
  status: string;
}

// Materialized view types (matches Go matview.Registration / matview.RefreshResult).
export type MatviewRefreshMode = "standard" | "concurrent";
export type MatviewRefreshStatus = "success" | "error";

export interface MatviewRegistration {
  id: string;
  schemaName: string;
  viewName: string;
  refreshMode: MatviewRefreshMode;
  lastRefreshAt: string | null;
  lastRefreshDurationMs: number | null;
  lastRefreshStatus: MatviewRefreshStatus | null;
  lastRefreshError: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface MatviewListResponse {
  items: MatviewRegistration[];
  count: number;
}

export interface MatviewRefreshResult {
  registration: MatviewRegistration;
  durationMs: number;
}

// Storage types.
export interface StorageObject {
  id: string;
  bucket: string;
  name: string;
  size: number;
  contentType: string;
  createdAt: string;
}

export interface StorageListResponse {
  items: StorageObject[];
  totalItems: number;
}
