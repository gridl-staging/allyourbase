import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, Loader2 } from "lucide-react";
import {
  fetchUsageBreakdown,
  fetchUsageLimits,
  fetchUsageList,
  fetchUsageTrends,
} from "../api_usage";
import { formatDate } from "./shared/format";
import {
  formatMetricLimitRows,
  UsageAggregateSection,
  UsageChartsSection,
  UsageFilterControls,
  UsageHeader,
} from "./UsageMeteringSections";
import type {
  UsageBreakdownGroupBy,
  UsageBreakdownResponse,
  UsageGranularity,
  UsageLimitsResponse,
  UsageListResponse,
  UsageMetric,
  UsagePaginationState,
  UsageQueryState,
  UsageSortColumn,
  UsageSortDirection,
  UsageTrendResponse,
} from "../types/usage";

const API_REQUEST_GRANULARITY_OPTIONS: UsageGranularity[] = ["hour", "day", "week", "month"];
const NON_REQUEST_GRANULARITY_OPTIONS: UsageGranularity[] = ["day", "week", "month"];
const API_REQUEST_GROUP_OPTIONS: UsageBreakdownGroupBy[] = ["tenant", "endpoint", "status_code"];

const DEFAULT_QUERY: UsageQueryState = {
  period: "month",
  from: null,
  to: null,
  metric: "api_requests",
  granularity: "day",
  groupBy: "tenant",
  sort: {
    column: "request_count",
    direction: "desc",
  },
  pagination: {
    limit: 50,
    offset: 0,
  },
  selectedTenantId: null,
};

interface UsageOverviewState {
  listData: UsageListResponse | null;
  trendData: UsageTrendResponse | null;
  breakdownData: UsageBreakdownResponse | null;
  isLoading: boolean;
  loadError: string | null;
}

interface UsageLimitsState {
  limitsData: UsageLimitsResponse | null;
  isLimitsLoading: boolean;
  limitsError: string | null;
}

interface UsageQueryController {
  query: UsageQueryState;
  refreshVersion: number;
  granularityOptions: UsageGranularity[];
  groupByOptions: UsageBreakdownGroupBy[];
  reloadUsageData: () => void;
  onMetricChange: (metric: UsageMetric) => void;
  onGranularityChange: (granularity: UsageGranularity) => void;
  onBreakdownChange: (groupBy: UsageBreakdownGroupBy) => void;
  onPeriodChange: (period: UsageQueryState["period"]) => void;
  onSortColumnChange: (column: UsageSortColumn) => void;
  onNextPage: () => void;
  onPreviousPage: () => void;
  onSelectTenant: (tenantId: string) => void;
  applyResolvedTenantId: (tenantId: string | null) => void;
}

function allowedGranularityOptions(metric: UsageMetric): UsageGranularity[] {
  return metric === "api_requests" ? API_REQUEST_GRANULARITY_OPTIONS : NON_REQUEST_GRANULARITY_OPTIONS;
}

function allowedGroupByOptions(metric: UsageMetric): UsageBreakdownGroupBy[] {
  return metric === "api_requests" ? API_REQUEST_GROUP_OPTIONS : ["tenant"];
}

function coerceGranularity(metric: UsageMetric, granularity: UsageGranularity): UsageGranularity {
  const allowed = allowedGranularityOptions(metric);
  return allowed.includes(granularity) ? granularity : "day";
}

function coerceGroupBy(metric: UsageMetric, groupBy: UsageBreakdownGroupBy): UsageBreakdownGroupBy {
  const allowed = allowedGroupByOptions(metric);
  return allowed.includes(groupBy) ? groupBy : "tenant";
}

function resolveSelectedTenantId(
  currentTenantId: string | null,
  items: UsageListResponse["items"],
): string | null {
  if (currentTenantId && items.some((item) => item.tenantId === currentTenantId)) {
    return currentTenantId;
  }
  return items.length > 0 ? items[0].tenantId : null;
}

function sortDirectionForColumn(
  currentColumn: UsageSortColumn,
  currentDirection: UsageSortDirection,
  nextColumn: UsageSortColumn,
): UsageSortDirection {
  if (currentColumn === nextColumn) {
    return currentDirection === "asc" ? "desc" : "asc";
  }
  return "asc";
}

function resetPaginationOffset(pagination: UsagePaginationState): UsagePaginationState {
  return {
    ...pagination,
    offset: 0,
  };
}

function updateMetricQuery(previous: UsageQueryState, metric: UsageMetric): UsageQueryState {
  return {
    ...previous,
    metric,
    granularity: coerceGranularity(metric, previous.granularity),
    groupBy: coerceGroupBy(metric, previous.groupBy),
    pagination: resetPaginationOffset(previous.pagination),
  };
}

function updateGranularityQuery(
  previous: UsageQueryState,
  granularity: UsageGranularity,
): UsageQueryState {
  return {
    ...previous,
    granularity: coerceGranularity(previous.metric, granularity),
  };
}

function updateGroupByQuery(
  previous: UsageQueryState,
  groupBy: UsageBreakdownGroupBy,
): UsageQueryState {
  return {
    ...previous,
    groupBy: coerceGroupBy(previous.metric, groupBy),
  };
}

function updatePeriodQuery(
  previous: UsageQueryState,
  period: UsageQueryState["period"],
): UsageQueryState {
  return {
    ...previous,
    period,
    pagination: resetPaginationOffset(previous.pagination),
  };
}

function updateSortQuery(previous: UsageQueryState, column: UsageSortColumn): UsageQueryState {
  return {
    ...previous,
    sort: {
      column,
      direction: sortDirectionForColumn(previous.sort.column, previous.sort.direction, column),
    },
    pagination: resetPaginationOffset(previous.pagination),
  };
}

function advancePageQuery(previous: UsageQueryState): UsageQueryState {
  return {
    ...previous,
    pagination: {
      ...previous.pagination,
      offset: previous.pagination.offset + previous.pagination.limit,
    },
  };
}

function retreatPageQuery(previous: UsageQueryState): UsageQueryState {
  return {
    ...previous,
    pagination: {
      ...previous.pagination,
      offset: Math.max(0, previous.pagination.offset - previous.pagination.limit),
    },
  };
}

function updateSelectedTenantQuery(previous: UsageQueryState, tenantId: string | null): UsageQueryState {
  return {
    ...previous,
    selectedTenantId: tenantId,
  };
}

function useUsageQueryState(): UsageQueryController {
  const [query, setQuery] = useState<UsageQueryState>(DEFAULT_QUERY);
  const [refreshVersion, setRefreshVersion] = useState(0);

  const reloadUsageData = useCallback(() => {
    setRefreshVersion((version) => version + 1);
  }, []);

  const applyResolvedTenantId = useCallback((tenantId: string | null) => {
    setQuery((previous) => {
      if (previous.selectedTenantId === tenantId) {
        return previous;
      }
      return updateSelectedTenantQuery(previous, tenantId);
    });
  }, []);

  const onMetricChange = useCallback((metric: UsageMetric) => {
    setQuery((previous) => updateMetricQuery(previous, metric));
  }, []);

  const onGranularityChange = useCallback((granularity: UsageGranularity) => {
    setQuery((previous) => updateGranularityQuery(previous, granularity));
  }, []);

  const onBreakdownChange = useCallback((groupBy: UsageBreakdownGroupBy) => {
    setQuery((previous) => updateGroupByQuery(previous, groupBy));
  }, []);

  const onPeriodChange = useCallback((period: UsageQueryState["period"]) => {
    setQuery((previous) => updatePeriodQuery(previous, period));
  }, []);

  const onSortColumnChange = useCallback((column: UsageSortColumn) => {
    setQuery((previous) => updateSortQuery(previous, column));
  }, []);

  const onNextPage = useCallback(() => {
    setQuery((previous) => advancePageQuery(previous));
  }, []);

  const onPreviousPage = useCallback(() => {
    setQuery((previous) => retreatPageQuery(previous));
  }, []);

  const onSelectTenant = useCallback((tenantId: string) => {
    setQuery((previous) => updateSelectedTenantQuery(previous, tenantId));
  }, []);

  return {
    query,
    refreshVersion,
    granularityOptions: allowedGranularityOptions(query.metric),
    groupByOptions: allowedGroupByOptions(query.metric),
    reloadUsageData,
    onMetricChange,
    onGranularityChange,
    onBreakdownChange,
    onPeriodChange,
    onSortColumnChange,
    onNextPage,
    onPreviousPage,
    onSelectTenant,
    applyResolvedTenantId,
  };
}

function useUsageOverviewState(
  query: UsageQueryState,
  onResolveTenantId: (tenantId: string | null) => void,
  refreshVersion: number,
): UsageOverviewState {
  const [listData, setListData] = useState<UsageListResponse | null>(null);
  const [trendData, setTrendData] = useState<UsageTrendResponse | null>(null);
  const [breakdownData, setBreakdownData] = useState<UsageBreakdownResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const loadSequenceRef = useRef(0);
  const selectedTenantIdRef = useRef<string | null>(query.selectedTenantId);

  useEffect(() => {
    selectedTenantIdRef.current = query.selectedTenantId;
  }, [query.selectedTenantId]);

  useEffect(() => {
    const sequence = loadSequenceRef.current + 1;
    loadSequenceRef.current = sequence;
    let isStale = false;

    setIsLoading(true);
    setLoadError(null);

    Promise.all([fetchUsageList(query), fetchUsageTrends(query), fetchUsageBreakdown(query)])
      .then(([nextList, nextTrend, nextBreakdown]) => {
        if (isStale || loadSequenceRef.current !== sequence) {
          return;
        }
        setListData(nextList);
        setTrendData(nextTrend);
        setBreakdownData(nextBreakdown);
        onResolveTenantId(resolveSelectedTenantId(selectedTenantIdRef.current, nextList.items));
      })
      .catch((error: unknown) => {
        if (isStale || loadSequenceRef.current !== sequence) {
          return;
        }
        setLoadError(error instanceof Error ? error.message : "Failed to load usage data");
      })
      .finally(() => {
        if (!isStale && loadSequenceRef.current === sequence) {
          setIsLoading(false);
        }
      });

    return () => {
      isStale = true;
    };
  }, [
    onResolveTenantId,
    query.period,
    query.from,
    query.to,
    query.metric,
    query.granularity,
    query.groupBy,
    query.sort.column,
    query.sort.direction,
    query.pagination.limit,
    query.pagination.offset,
    refreshVersion,
  ]);

  return {
    listData,
    trendData,
    breakdownData,
    isLoading,
    loadError,
  };
}

function useUsageLimitsState(query: UsageQueryState, refreshVersion: number): UsageLimitsState {
  const [limitsData, setLimitsData] = useState<UsageLimitsResponse | null>(null);
  const [isLimitsLoading, setIsLimitsLoading] = useState(false);
  const [limitsError, setLimitsError] = useState<string | null>(null);

  const limitsLoadSequenceRef = useRef(0);
  const previousTenantIdRef = useRef<string | null>(query.selectedTenantId);

  useEffect(() => {
    const tenantId = query.selectedTenantId;
    if (!tenantId) {
      previousTenantIdRef.current = null;
      setLimitsData(null);
      setLimitsError(null);
      setIsLimitsLoading(false);
      return;
    }

    const tenantChanged = previousTenantIdRef.current !== tenantId;
    previousTenantIdRef.current = tenantId;
    if (tenantChanged) {
      setLimitsData(null);
    }

    const sequence = limitsLoadSequenceRef.current + 1;
    limitsLoadSequenceRef.current = sequence;
    let isStale = false;

    setIsLimitsLoading(true);
    setLimitsError(null);

    fetchUsageLimits(query)
      .then((limits) => {
        if (isStale || limitsLoadSequenceRef.current !== sequence) {
          return;
        }
        setLimitsData(limits);
      })
      .catch((error: unknown) => {
        if (isStale || limitsLoadSequenceRef.current !== sequence) {
          return;
        }
        setLimitsError(error instanceof Error ? error.message : "Failed to load tenant limits");
      })
      .finally(() => {
        if (!isStale && limitsLoadSequenceRef.current === sequence) {
          setIsLimitsLoading(false);
        }
      });

    return () => {
      isStale = true;
    };
  }, [query.selectedTenantId, query.period, query.from, query.to, refreshVersion]);

  return {
    limitsData,
    isLimitsLoading,
    limitsError,
  };
}

export function UsageMetering() {
  const usageQuery = useUsageQueryState();
  const overview = useUsageOverviewState(
    usageQuery.query,
    usageQuery.applyResolvedTenantId,
    usageQuery.refreshVersion,
  );
  const limits = useUsageLimitsState(usageQuery.query, usageQuery.refreshVersion);

  const metricLimitRows = useMemo(() => formatMetricLimitRows(limits.limitsData), [limits.limitsData]);
  const rows = overview.listData?.items ?? [];
  const trendPoints = overview.trendData?.items ?? [];
  const breakdownEntries = overview.breakdownData?.items ?? [];
  const totalRows = overview.listData?.total ?? 0;
  const canGoPreviousPage = usageQuery.query.pagination.offset > 0;
  const canGoNextPage =
    usageQuery.query.pagination.offset + usageQuery.query.pagination.limit < totalRows;

  if (overview.isLoading && !overview.listData && !overview.trendData && !overview.breakdownData) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-500 dark:text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading usage metering...
      </div>
    );
  }

  if (overview.loadError && !overview.listData) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center space-y-3">
          <AlertCircle className="w-8 h-8 text-red-500 mx-auto" />
          <p className="text-sm text-red-600 dark:text-red-400">{overview.loadError}</p>
          <button
            onClick={usageQuery.reloadUsageData}
            className="px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-600 hover:bg-gray-100 dark:hover:bg-gray-800"
          >
            Retry usage data
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      <UsageHeader onRefresh={usageQuery.reloadUsageData} />
      <UsageFilterControls
        query={usageQuery.query}
        granularityOptions={usageQuery.granularityOptions}
        groupByOptions={usageQuery.groupByOptions}
        onMetricChange={usageQuery.onMetricChange}
        onGranularityChange={usageQuery.onGranularityChange}
        onBreakdownChange={usageQuery.onBreakdownChange}
        onPeriodChange={usageQuery.onPeriodChange}
      />
      <UsageAggregateSection
        query={usageQuery.query}
        rows={rows}
        totalRows={totalRows}
        canGoPreviousPage={canGoPreviousPage}
        canGoNextPage={canGoNextPage}
        onSelectTenant={usageQuery.onSelectTenant}
        onSortColumnChange={usageQuery.onSortColumnChange}
        onPreviousPage={usageQuery.onPreviousPage}
        onNextPage={usageQuery.onNextPage}
      />
      <UsageChartsSection
        query={usageQuery.query}
        trendPoints={trendPoints}
        breakdownEntries={breakdownEntries}
        isLimitsLoading={limits.isLimitsLoading}
        limitsError={limits.limitsError}
        limitsData={limits.limitsData}
        metricLimitRows={metricLimitRows}
      />

      {overview.isLoading && (
        <p className="text-xs text-gray-500 dark:text-gray-400 inline-flex items-center gap-2">
          <Loader2 className="w-3.5 h-3.5 animate-spin" />
          Refreshing usage data...
        </p>
      )}

      {!overview.isLoading && overview.loadError && overview.listData && (
        <p className="text-xs text-amber-700 dark:text-amber-300 inline-flex items-center gap-2">
          <AlertCircle className="w-3.5 h-3.5" />
          {overview.loadError}
        </p>
      )}

      {trendPoints.length > 0 && (
        <p className="text-xs text-gray-500 dark:text-gray-400">
          Trend sample starts {formatDate(trendPoints[0]?.timestamp ?? null)}
        </p>
      )}
    </div>
  );
}
