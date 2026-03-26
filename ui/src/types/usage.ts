export const USAGE_PERIOD_VALUES = ["day", "week", "month"] as const;
export type UsagePeriod = (typeof USAGE_PERIOD_VALUES)[number];

export const USAGE_METRIC_VALUES = [
  "api_requests",
  "storage_bytes",
  "bandwidth_bytes",
  "function_invocations",
] as const;
export type UsageMetric = (typeof USAGE_METRIC_VALUES)[number];

export const USAGE_GRANULARITY_VALUES = ["hour", "day", "week", "month"] as const;
export type UsageGranularity = (typeof USAGE_GRANULARITY_VALUES)[number];

export const USAGE_BREAKDOWN_GROUP_VALUES = ["tenant", "endpoint", "status_code"] as const;
export type UsageBreakdownGroupBy = (typeof USAGE_BREAKDOWN_GROUP_VALUES)[number];

export const USAGE_SORT_COLUMN_VALUES = [
  "request_count",
  "storage_bytes",
  "storage_bytes_used",
  "bandwidth_bytes",
  "function_invocations",
  "realtime_peak_connections",
  "job_runs",
  "tenant_id",
  "tenant_name",
] as const;
export type UsageSortColumn = (typeof USAGE_SORT_COLUMN_VALUES)[number];

export const USAGE_SORT_DIRECTION_VALUES = ["asc", "desc"] as const;
export type UsageSortDirection = (typeof USAGE_SORT_DIRECTION_VALUES)[number];

export interface UsageSortSpec {
  column: UsageSortColumn;
  direction: UsageSortDirection;
}

export interface UsagePaginationState {
  limit: number;
  offset: number;
}

export interface UsageQueryState {
  period: UsagePeriod;
  from: string | null;
  to: string | null;
  metric: UsageMetric;
  granularity: UsageGranularity;
  groupBy: UsageBreakdownGroupBy;
  sort: UsageSortSpec;
  pagination: UsagePaginationState;
  selectedTenantId: string | null;
}

export interface UsageListItem {
  tenantId: string;
  tenantName: string;
  requestCount: number;
  storageBytesUsed: number;
  bandwidthBytes: number;
  functionInvocations: number;
  realtimePeakConnections: number;
  jobRuns: number;
}

export interface UsageListResponse {
  items: UsageListItem[];
  total: number;
  limit: number;
  offset: number;
}

export interface UsageTrendPoint {
  timestamp: string;
  value: number;
}

export interface UsageTrendResponse {
  metric: UsageMetric;
  granularity: UsageGranularity;
  items: UsageTrendPoint[];
}

export interface UsageBreakdownEntry {
  key: string;
  value: number;
}

export interface UsageBreakdownResponse {
  metric: UsageMetric;
  groupBy: UsageBreakdownGroupBy;
  items: UsageBreakdownEntry[];
}

export interface UsageMetricLimit {
  limit: number;
  used: number;
  remaining: number;
}

export type UsageLimitsMetricMap = Record<UsageMetric, UsageMetricLimit>;

export interface UsageLimitsResponse {
  plan: string;
  metrics: UsageLimitsMetricMap;
}
