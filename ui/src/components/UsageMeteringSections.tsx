import { RefreshCw } from "lucide-react";
import { formatBytes } from "./shared/format";
import type {
  UsageBreakdownEntry,
  UsageBreakdownGroupBy,
  UsageGranularity,
  UsageLimitsResponse,
  UsageListItem,
  UsageMetric,
  UsageQueryState,
  UsageSortColumn,
  UsageTrendPoint,
} from "../types/usage";
import { USAGE_METRIC_VALUES } from "../types/usage";

const METRIC_LABELS: Record<UsageMetric, string> = {
  api_requests: "API Requests",
  storage_bytes: "Storage Bytes",
  bandwidth_bytes: "Bandwidth Bytes",
  function_invocations: "Function Invocations",
};

const PERIOD_OPTIONS = [
  { value: "day", label: "Today" },
  { value: "week", label: "Last 7 days" },
  { value: "month", label: "Last 30 days" },
] as const;

const DAY_TREND_LABEL_FORMATTER = new Intl.DateTimeFormat("en-US", {
  timeZone: "UTC",
  month: "short",
  day: "numeric",
  year: "numeric",
});

const HOUR_TREND_LABEL_FORMATTER = new Intl.DateTimeFormat("en-US", {
  timeZone: "UTC",
  month: "short",
  day: "numeric",
  hour: "numeric",
  minute: "2-digit",
});

export interface MetricLimitRow {
  key: UsageMetric;
  label: string;
  used: string;
  limit: string;
  remaining: string;
}

interface UsageHeaderProps {
  onRefresh: () => void;
}

interface UsageFilterControlsProps {
  query: UsageQueryState;
  granularityOptions: UsageGranularity[];
  groupByOptions: UsageBreakdownGroupBy[];
  onMetricChange: (metric: UsageMetric) => void;
  onGranularityChange: (granularity: UsageGranularity) => void;
  onBreakdownChange: (groupBy: UsageBreakdownGroupBy) => void;
  onPeriodChange: (period: UsageQueryState["period"]) => void;
}

interface UsageAggregateSectionProps {
  query: UsageQueryState;
  rows: UsageListItem[];
  totalRows: number;
  canGoPreviousPage: boolean;
  canGoNextPage: boolean;
  onSelectTenant: (tenantId: string) => void;
  onSortColumnChange: (column: UsageSortColumn) => void;
  onPreviousPage: () => void;
  onNextPage: () => void;
}

interface TenantLimitsPanelProps {
  query: UsageQueryState;
  isLimitsLoading: boolean;
  limitsError: string | null;
  limitsData: UsageLimitsResponse | null;
  metricLimitRows: MetricLimitRow[];
}

interface UsageChartsSectionProps extends TenantLimitsPanelProps {
  query: UsageQueryState;
  trendPoints: UsageTrendPoint[];
  breakdownEntries: UsageBreakdownEntry[];
}

function formatMetricValue(metric: UsageMetric, value: number): string {
  if (metric === "storage_bytes" || metric === "bandwidth_bytes") {
    return formatBytes(value);
  }
  return value.toLocaleString();
}

function formatMetricLabel(metric: UsageMetric): string {
  return METRIC_LABELS[metric];
}

function formatUsageNumber(value: number): string {
  return value.toLocaleString();
}

function formatTrendTimestamp(timestamp: string, granularity: UsageGranularity): string {
  const parsed = new Date(timestamp);
  if (Number.isNaN(parsed.getTime())) {
    return "-";
  }
  if (granularity === "hour") {
    return `${HOUR_TREND_LABEL_FORMATTER.format(parsed)} UTC`;
  }
  return DAY_TREND_LABEL_FORMATTER.format(parsed);
}

function UsageTrendChart({
  metric,
  granularity,
  points,
}: {
  metric: UsageMetric;
  granularity: UsageGranularity;
  points: UsageTrendPoint[];
}) {
  if (points.length === 0) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">No trend data</p>;
  }

  const values = points.map((point) => point.value);
  const maxValue = Math.max(...values, 1);
  const minValue = Math.min(...values, 0);
  const valueRange = Math.max(maxValue - minValue, 1);
  const stepX = points.length > 1 ? 280 / (points.length - 1) : 0;

  const polyline = points
    .map((point, index) => {
      const x = 20 + index * stepX;
      const y = 140 - ((point.value - minValue) / valueRange) * 110;
      return `${x},${y}`;
    })
    .join(" ");

  return (
    <div className="space-y-3">
      <svg
        viewBox="0 0 320 160"
        className="w-full h-40 rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900"
        aria-label="Usage trend chart"
      >
        <line x1="20" y1="140" x2="300" y2="140" stroke="currentColor" className="text-gray-300 dark:text-gray-700" />
        <polyline
          points={polyline}
          fill="none"
          stroke="currentColor"
          strokeWidth="3"
          className="text-blue-600 dark:text-blue-400"
        />
      </svg>
      <ul className="space-y-1 text-xs text-gray-600 dark:text-gray-300">
        {points.map((point) => (
          <li key={`${point.timestamp}-${point.value}`} className="flex justify-between">
            <span>{formatTrendTimestamp(point.timestamp, granularity)}</span>
            <span>{formatMetricValue(metric, point.value)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function UsageBreakdownChart({ metric, entries }: { metric: UsageMetric; entries: UsageBreakdownEntry[] }) {
  if (entries.length === 0) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">No breakdown data</p>;
  }

  const maxValue = Math.max(...entries.map((entry) => entry.value), 1);

  return (
    <div className="space-y-2">
      {entries.map((entry) => {
        const widthPercent = (entry.value / maxValue) * 100;
        return (
          <div key={`${entry.key}-${entry.value}`} className="space-y-1">
            <div className="flex justify-between text-xs text-gray-600 dark:text-gray-300">
              <span className="truncate pr-3">{entry.key || "(empty)"}</span>
              <span>{formatMetricValue(metric, entry.value)}</span>
            </div>
            <div className="h-2 rounded bg-gray-200 dark:bg-gray-700 overflow-hidden" aria-label="Usage breakdown chart">
              <div className="h-full bg-teal-500 dark:bg-teal-400" style={{ width: `${widthPercent}%` }} />
            </div>
          </div>
        );
      })}
    </div>
  );
}

export function formatMetricLimitRows(limits: UsageLimitsResponse | null): MetricLimitRow[] {
  if (!limits) {
    return [];
  }

  return USAGE_METRIC_VALUES.map((metricKey) => {
    const metricData = limits.metrics[metricKey];
    return {
      key: metricKey,
      label: formatMetricLabel(metricKey),
      used: formatMetricValue(metricKey, metricData.used),
      limit: formatMetricValue(metricKey, metricData.limit),
      remaining: formatMetricValue(metricKey, metricData.remaining),
    };
  });
}

export function UsageHeader({ onRefresh }: UsageHeaderProps) {
  return (
    <header className="flex items-center justify-between gap-4">
      <div>
        <h1 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Usage Metering</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
          Shared usage contract across aggregate list, trends, breakdown, and per-tenant limits.
        </p>
      </div>
      <button
        onClick={onRefresh}
        className="inline-flex items-center gap-2 px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-600 hover:bg-gray-100 dark:hover:bg-gray-800"
      >
        <RefreshCw className="w-4 h-4" />
        Refresh
      </button>
    </header>
  );
}

export function UsageFilterControls({
  query,
  granularityOptions,
  groupByOptions,
  onMetricChange,
  onGranularityChange,
  onBreakdownChange,
  onPeriodChange,
}: UsageFilterControlsProps) {
  return (
    <section className="grid grid-cols-1 md:grid-cols-4 gap-3">
      <label className="text-sm text-gray-700 dark:text-gray-300" htmlFor="usage-period">
        <span className="block mb-1">Period</span>
        <select
          id="usage-period"
          className="w-full rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900 px-2 py-1.5"
          value={query.period}
          onChange={(event) => onPeriodChange(event.target.value as UsageQueryState["period"])}
        >
          {PERIOD_OPTIONS.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </label>

      <label className="text-sm text-gray-700 dark:text-gray-300" htmlFor="usage-metric">
        <span className="block mb-1">Metric</span>
        <select
          id="usage-metric"
          className="w-full rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900 px-2 py-1.5"
          value={query.metric}
          onChange={(event) => onMetricChange(event.target.value as UsageMetric)}
        >
          {(Object.keys(METRIC_LABELS) as UsageMetric[]).map((metric) => (
            <option key={metric} value={metric}>
              {METRIC_LABELS[metric]}
            </option>
          ))}
        </select>
      </label>

      <label className="text-sm text-gray-700 dark:text-gray-300" htmlFor="usage-granularity">
        <span className="block mb-1">Granularity</span>
        <select
          id="usage-granularity"
          className="w-full rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900 px-2 py-1.5"
          value={query.granularity}
          onChange={(event) => onGranularityChange(event.target.value as UsageGranularity)}
        >
          {granularityOptions.map((granularity) => (
            <option key={granularity} value={granularity}>
              {granularity}
            </option>
          ))}
        </select>
      </label>

      <label className="text-sm text-gray-700 dark:text-gray-300" htmlFor="usage-breakdown">
        <span className="block mb-1">Breakdown</span>
        <select
          id="usage-breakdown"
          className="w-full rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900 px-2 py-1.5"
          value={query.groupBy}
          onChange={(event) => onBreakdownChange(event.target.value as UsageBreakdownGroupBy)}
        >
          {groupByOptions.map((groupBy) => (
            <option key={groupBy} value={groupBy}>
              {groupBy}
            </option>
          ))}
        </select>
      </label>
    </section>
  );
}

export function UsageAggregateSection({
  query,
  rows,
  totalRows,
  canGoPreviousPage,
  canGoNextPage,
  onSelectTenant,
  onSortColumnChange,
  onPreviousPage,
  onNextPage,
}: UsageAggregateSectionProps) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100">Aggregate Usage</h2>
        <p className="text-xs text-gray-500 dark:text-gray-400">
          Showing {rows.length} of {formatUsageNumber(totalRows)} tenants
        </p>
      </div>

      {rows.length === 0 ? (
        <div className="rounded border border-dashed border-gray-300 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/40 px-4 py-8 text-sm text-gray-500 dark:text-gray-400 text-center">
          No tenant usage rows
        </div>
      ) : (
        <div className="rounded border border-gray-200 dark:border-gray-700 overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700">
              <tr>
                <th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
                  <button
                    type="button"
                    onClick={() => onSortColumnChange("tenant_name")}
                    className="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-gray-100"
                    aria-label="Sort by tenant name"
                  >
                    Tenant
                    <span className="text-xs">{query.sort.column === "tenant_name" ? query.sort.direction : "-"}</span>
                  </button>
                </th>
                <th className="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Requests</th>
                <th className="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Storage</th>
                <th className="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Bandwidth</th>
                <th className="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Functions</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const selectedRow = row.tenantId === query.selectedTenantId;
                return (
                  <tr
                    key={row.tenantId}
                    className={`border-b border-gray-100 dark:border-gray-800 last:border-b-0 cursor-pointer ${selectedRow ? "bg-blue-50 dark:bg-blue-900/20" : "hover:bg-gray-50 dark:hover:bg-gray-900/40"}`}
                    onClick={() => onSelectTenant(row.tenantId)}
                  >
                    <td className="px-3 py-2 text-gray-900 dark:text-gray-100">{row.tenantName || row.tenantId}</td>
                    <td className="px-3 py-2 text-right text-gray-700 dark:text-gray-200">{formatUsageNumber(row.requestCount)}</td>
                    <td className="px-3 py-2 text-right text-gray-700 dark:text-gray-200">{formatBytes(row.storageBytesUsed)}</td>
                    <td className="px-3 py-2 text-right text-gray-700 dark:text-gray-200">{formatBytes(row.bandwidthBytes)}</td>
                    <td className="px-3 py-2 text-right text-gray-700 dark:text-gray-200">{formatUsageNumber(row.functionInvocations)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onPreviousPage}
          disabled={!canGoPreviousPage}
          aria-label="Previous page"
          className="px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-700 disabled:opacity-40"
        >
          Previous
        </button>
        <button
          type="button"
          onClick={onNextPage}
          disabled={!canGoNextPage}
          aria-label="Next page"
          className="px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-700 disabled:opacity-40"
        >
          Next
        </button>
      </div>
    </section>
  );
}

function TenantLimitsPanel({
  query,
  isLimitsLoading,
  limitsError,
  limitsData,
  metricLimitRows,
}: TenantLimitsPanelProps) {
  if (!query.selectedTenantId) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">Select a tenant to view limits</p>;
  }

  if (isLimitsLoading && !limitsData) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">Loading tenant limits...</p>;
  }

  if (limitsError) {
    return <p className="text-sm text-red-600 dark:text-red-400">{limitsError}</p>;
  }

  if (!limitsData) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">No limits data</p>;
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-gray-600 dark:text-gray-300">Plan: {limitsData.plan || "-"}</p>
      <div className="space-y-2">
        {metricLimitRows.map((row) => (
          <div key={row.key} className="rounded border border-gray-200 dark:border-gray-700 px-3 py-2">
            <p className="text-sm font-medium text-gray-900 dark:text-gray-100">{row.label}</p>
            <p className="text-xs text-gray-500 dark:text-gray-400">
              Used {row.used} of {row.limit} (remaining {row.remaining})
            </p>
          </div>
        ))}
      </div>
    </div>
  );
}

export function UsageChartsSection({
  query,
  trendPoints,
  breakdownEntries,
  isLimitsLoading,
  limitsError,
  limitsData,
  metricLimitRows,
}: UsageChartsSectionProps) {
  return (
    <section className="grid grid-cols-1 xl:grid-cols-3 gap-4">
      <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4">
        <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-3">Usage Trend</h2>
        <UsageTrendChart metric={query.metric} granularity={query.granularity} points={trendPoints} />
      </div>

      <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4">
        <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-3">Usage Breakdown</h2>
        <UsageBreakdownChart metric={query.metric} entries={breakdownEntries} />
      </div>

      <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4">
        <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-3">Tenant Limits</h2>
        <TenantLimitsPanel
          query={query}
          isLimitsLoading={isLimitsLoading}
          limitsError={limitsError}
          limitsData={limitsData}
          metricLimitRows={metricLimitRows}
        />
      </div>
    </section>
  );
}
