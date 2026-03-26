export interface EdgeFunctionResponse {
  id: string;
  name: string;
  entryPoint: string;
  timeout: number;
  lastInvokedAt?: string | null;
  envVars?: Record<string, string>;
  public: boolean;
  source: string;
  compiledJs: string;
  createdAt: string;
  updatedAt: string;
}

export interface EdgeFunctionLogEntry {
  id: string;
  functionId: string;
  invocationId: string;
  status: "success" | "error";
  durationMs: number;
  stdout?: string;
  error?: string;
  requestMethod?: string;
  requestPath?: string;
  triggerType?: string;
  triggerId?: string;
  parentInvocationId?: string;
  createdAt: string;
}

export interface EdgeFunctionDeployRequest {
  name: string;
  source: string;
  entry_point?: string;
  timeout_ms?: number;
  env_vars?: Record<string, string>;
  public?: boolean;
}

export interface EdgeFunctionUpdateRequest {
  source: string;
  entry_point?: string;
  timeout_ms?: number;
  env_vars?: Record<string, string>;
  public?: boolean;
}

export interface EdgeFunctionInvokeRequest {
  method: string;
  path?: string;
  headers?: Record<string, string[]>;
  body?: string;
}

export interface EdgeFunctionInvokeResponse {
  statusCode: number;
  headers?: Record<string, string[]>;
  body?: string;
}

export type DBTriggerEvent = "INSERT" | "UPDATE" | "DELETE";

export interface DBTriggerResponse {
  id: string;
  functionId: string;
  tableName: string;
  schema: string;
  events: DBTriggerEvent[];
  filterColumns?: string[];
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface CreateDBTriggerRequest {
  table_name: string;
  schema?: string;
  events: DBTriggerEvent[];
  filter_columns?: string[];
}

export interface CronTriggerResponse {
  id: string;
  functionId: string;
  scheduleId: string;
  cronExpr: string;
  timezone: string;
  payload: unknown;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface CreateCronTriggerRequest {
  cron_expr: string;
  timezone?: string;
  payload?: unknown;
}

export interface StorageTriggerResponse {
  id: string;
  functionId: string;
  bucket: string;
  eventTypes: string[];
  prefixFilter?: string;
  suffixFilter?: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface CreateStorageTriggerRequest {
  bucket: string;
  event_types: string[];
  prefix_filter?: string;
  suffix_filter?: string;
}

export interface ManualRunResponse {
  statusCode: number;
  body?: string;
}
