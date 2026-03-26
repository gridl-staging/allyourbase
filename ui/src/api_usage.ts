import { request } from "./api_client";
import { asInteger, asRecord, asString, withQueryString } from "./lib/normalize";
import type {
  UsageBreakdownEntry,
  UsageBreakdownGroupBy,
  UsageBreakdownResponse,
  UsageGranularity,
  UsageLimitsMetricMap,
  UsageLimitsResponse,
  UsageListItem,
  UsageListResponse,
  UsageMetric,
  UsageMetricLimit,
  UsageQueryState,
  UsageTrendPoint,
  UsageTrendResponse,
} from "./types/usage";
import { USAGE_METRIC_VALUES } from "./types/usage";

const DEFAULT_LIMIT_METRIC: UsageMetricLimit = {
  limit: 0,
  used: 0,
  remaining: 0,
};

function asIntegerOrFallback(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? Math.trunc(value) : fallback;
}


function asMetric(value: unknown, fallback: UsageMetric): UsageMetric {
  return typeof value === "string" && USAGE_METRIC_VALUES.includes(value as UsageMetric)
    ? (value as UsageMetric)
    : fallback;
}

function asGranularity(value: unknown, fallback: UsageGranularity): UsageGranularity {
  return value === "hour" || value === "day" || value === "week" || value === "month"
    ? value
    : fallback;
}

function asGroupBy(value: unknown, fallback: UsageBreakdownGroupBy): UsageBreakdownGroupBy {
  return value === "tenant" || value === "endpoint" || value === "status_code" ? value : fallback;
}

function serializeSort(query: UsageQueryState): string {
  return `${query.sort.column}:${query.sort.direction}`;
}

function applyPeriodOrDateRange(params: URLSearchParams, query: UsageQueryState): void {
  if (query.from && query.to) {
    params.set("from", query.from);
    params.set("to", query.to);
    return;
  }
  params.set("period", query.period);
}

function normalizeListItem(rawItem: unknown): UsageListItem {
  const item = asRecord(rawItem);
  return {
    tenantId: asString(item?.tenantId),
    tenantName: asString(item?.tenantName),
    requestCount: asInteger(item?.requestCount),
    storageBytesUsed: asInteger(item?.storageBytesUsed),
    bandwidthBytes: asInteger(item?.bandwidthBytes),
    functionInvocations: asInteger(item?.functionInvocations),
    realtimePeakConnections: asInteger(item?.realtimePeakConnections),
    jobRuns: asInteger(item?.jobRuns),
  };
}

export function normalizeUsageListPayload(payload: unknown, query: UsageQueryState): UsageListResponse {
  const record = asRecord(payload);
  const rawItems = Array.isArray(record?.items) ? record.items : [];
  return {
    items: rawItems.map((rawItem) => normalizeListItem(rawItem)),
    total: asInteger(record?.total),
    limit: asIntegerOrFallback(record?.limit, query.pagination.limit),
    offset: asIntegerOrFallback(record?.offset, query.pagination.offset),
  };
}

function normalizeTrendPoint(rawPoint: unknown): UsageTrendPoint {
  const point = asRecord(rawPoint);
  return {
    timestamp: asString(point?.timestamp),
    value: asInteger(point?.value),
  };
}

export function normalizeUsageTrendPayload(payload: unknown, query: UsageQueryState): UsageTrendResponse {
  const record = asRecord(payload);
  const rawItems = Array.isArray(record?.items) ? record.items : [];
  return {
    metric: asMetric(record?.metric, query.metric),
    granularity: asGranularity(record?.granularity, query.granularity),
    items: rawItems.map((rawItem) => normalizeTrendPoint(rawItem)),
  };
}

function normalizeBreakdownEntry(rawEntry: unknown): UsageBreakdownEntry {
  const entry = asRecord(rawEntry);
  return {
    key: asString(entry?.key),
    value: asInteger(entry?.value),
  };
}

export function normalizeUsageBreakdownPayload(
  payload: unknown,
  query: UsageQueryState,
): UsageBreakdownResponse {
  const record = asRecord(payload);
  const rawItems = Array.isArray(record?.items) ? record.items : [];
  return {
    metric: asMetric(record?.metric, query.metric),
    groupBy: asGroupBy(record?.groupBy, query.groupBy),
    items: rawItems.map((rawItem) => normalizeBreakdownEntry(rawItem)),
  };
}

function normalizeMetricLimit(rawMetricLimit: unknown): UsageMetricLimit {
  const metricLimit = asRecord(rawMetricLimit);
  return {
    limit: asInteger(metricLimit?.limit),
    used: asInteger(metricLimit?.used),
    remaining: asInteger(metricLimit?.remaining),
  };
}

function createDefaultLimitsMetricMap(): UsageLimitsMetricMap {
  return Object.fromEntries(
    USAGE_METRIC_VALUES.map((metric) => [metric, { ...DEFAULT_LIMIT_METRIC }]),
  ) as UsageLimitsMetricMap;
}

export function normalizeUsageLimitsPayload(payload: unknown): UsageLimitsResponse {
  const record = asRecord(payload);
  const metrics = createDefaultLimitsMetricMap();
  const rawMetrics = asRecord(record?.metrics);

  for (const metric of USAGE_METRIC_VALUES) {
    const rawMetric = rawMetrics?.[metric];
    metrics[metric] = normalizeMetricLimit(rawMetric);
  }

  return {
    plan: asString(record?.plan),
    metrics,
  };
}

export async function fetchUsageList(query: UsageQueryState): Promise<UsageListResponse> {
  const params = new URLSearchParams();
  applyPeriodOrDateRange(params, query);
  params.set("sort", serializeSort(query));
  params.set("limit", String(query.pagination.limit));
  params.set("offset", String(query.pagination.offset));

  const payload = await request<unknown>(withQueryString("/api/admin/usage", params));
  return normalizeUsageListPayload(payload, query);
}

export async function fetchUsageTrends(query: UsageQueryState): Promise<UsageTrendResponse> {
  const params = new URLSearchParams();
  params.set("metric", query.metric);
  params.set("granularity", query.granularity);
  applyPeriodOrDateRange(params, query);

  const payload = await request<unknown>(withQueryString("/api/admin/usage/trends", params));
  return normalizeUsageTrendPayload(payload, query);
}

export async function fetchUsageBreakdown(query: UsageQueryState): Promise<UsageBreakdownResponse> {
  const params = new URLSearchParams();
  params.set("metric", query.metric);
  params.set("group_by", query.groupBy);
  applyPeriodOrDateRange(params, query);
  params.set("limit", String(query.pagination.limit));

  const payload = await request<unknown>(withQueryString("/api/admin/usage/breakdown", params));
  return normalizeUsageBreakdownPayload(payload, query);
}

export async function fetchUsageLimits(query: UsageQueryState): Promise<UsageLimitsResponse> {
  if (!query.selectedTenantId) {
    throw new Error("selectedTenantId is required to fetch usage limits");
  }

  const params = new URLSearchParams();
  applyPeriodOrDateRange(params, query);

  const tenantID = encodeURIComponent(query.selectedTenantId);
  const payload = await request<unknown>(withQueryString(`/api/admin/usage/${tenantID}/limits`, params));
  return normalizeUsageLimitsPayload(payload);
}
