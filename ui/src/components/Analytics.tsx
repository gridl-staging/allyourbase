import { useCallback, useEffect, useState } from "react";
import { AlertCircle, Loader2 } from "lucide-react";
import type { RequestLogListResponse, QueryAnalyticsResponse } from "../types/analytics";
import { listRequestLogs, listQueryStats } from "../api_analytics";
import { useDraftFilters } from "../hooks/useDraftFilters";
import { AdminTable, type Column } from "./shared/AdminTable";
import { FilterBar, type FilterField } from "./shared/FilterBar";
import { StatusBadge } from "./shared/StatusBadge";
import { cn } from "../lib/utils";

type Tab = "requests" | "queries";

const STATUS_VARIANT_MAP: Record<string, "success" | "error" | "warning" | "info"> = {
  "2": "success",
  "3": "info",
  "4": "warning",
  "5": "error",
};

function statusVariant(code: number): "success" | "error" | "warning" | "info" | "default" {
  const prefix = String(code)[0];
  return STATUS_VARIANT_MAP[prefix] ?? "default";
}

const REQUEST_FILTER_FIELDS: FilterField[] = [
  {
    name: "method",
    label: "Method",
    type: "select",
    options: [
      { value: "", label: "All methods" },
      { value: "GET", label: "GET" },
      { value: "POST", label: "POST" },
      { value: "PUT", label: "PUT" },
      { value: "PATCH", label: "PATCH" },
      { value: "DELETE", label: "DELETE" },
    ],
  },
  { name: "path", label: "Path", type: "text", placeholder: "/api/*" },
  { name: "status", label: "Status Code", type: "text", placeholder: "200" },
  { name: "from", label: "From", type: "date" },
  { name: "to", label: "To", type: "date" },
];

const INITIAL_REQUEST_FILTER_VALUES = {
  method: "",
  path: "",
  status: "",
  from: "",
  to: "",
};

type RequestRow = RequestLogListResponse["items"][number];
type QueryRow = QueryAnalyticsResponse["items"][number];

const REQUEST_COLUMNS: Column<RequestRow>[] = [
  { key: "method", header: "Method" },
  { key: "path", header: "Path" },
  {
    key: "status_code",
    header: "Status",
    render: (row) => (
      <StatusBadge
        status={String(row.status_code)}
        variantMap={{
          [String(row.status_code)]: statusVariant(row.status_code),
        }}
      />
    ),
  },
  {
    key: "duration_ms",
    header: "Duration",
    render: (row) => `${row.duration_ms}ms`,
  },
  {
    key: "timestamp",
    header: "Time",
    render: (row) => new Date(row.timestamp).toLocaleString(),
  },
];

const QUERY_COLUMNS: Column<QueryRow>[] = [
  {
    key: "query",
    header: "Query",
    render: (row) => (
      <code className="text-xs break-all max-w-[400px] block truncate" title={row.query}>
        {row.query}
      </code>
    ),
  },
  { key: "calls", header: "Calls", render: (row) => String(row.calls) },
  {
    key: "mean_exec_time",
    header: "Avg (ms)",
    render: (row) => row.mean_exec_time.toFixed(2),
  },
  {
    key: "total_exec_time",
    header: "Total (ms)",
    render: (row) => row.total_exec_time.toFixed(1),
  },
  { key: "rows", header: "Rows", render: (row) => String(row.rows) },
  {
    key: "index_suggestions",
    header: "Index Suggestions",
    render: (row) =>
      row.index_suggestions?.map((s, i) => (
        <div key={i} className="text-xs">
          <code className="bg-yellow-50 dark:bg-yellow-900/20 px-1 py-0.5 rounded text-yellow-700 dark:text-yellow-300">
            {s.statement}
          </code>
          <span className="ml-1 text-gray-400">({s.confidence})</span>
        </div>
      )) ?? null,
  },
];

const TAB_CLASS =
  "px-4 py-2 text-sm font-medium rounded-t border-b-2 transition-colors";
const TAB_ACTIVE =
  "border-blue-600 text-blue-600 dark:text-blue-400 dark:border-blue-400";
const TAB_INACTIVE =
  "border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400";

export function Analytics() {
  const [tab, setTab] = useState<Tab>("requests");
  const [requestData, setRequestData] = useState<RequestLogListResponse | null>(null);
  const [queryData, setQueryData] = useState<QueryAnalyticsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const {
    draftValues: draftFilterValues,
    appliedValues: appliedFilterValues,
    setDraftValue: setDraftFilterValue,
    applyValues: applyFilterValues,
    resetValues: resetFilterValues,
  } = useDraftFilters(INITIAL_REQUEST_FILTER_VALUES);
  const [querySort, setQuerySort] = useState("total_time");

  const loadRequests = useCallback(async (filters: Record<string, string>) => {
    try {
      setError(null);
      const status = filters.status?.trim();
      const result = await listRequestLogs({
        method: filters.method || undefined,
        path: filters.path || undefined,
        status: status || undefined,
        from: filters.from || undefined,
        to: filters.to || undefined,
      });
      setRequestData(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load request logs");
    } finally {
      setLoading(false);
    }
  }, []);

  const loadQueries = useCallback(async (sort: string) => {
    try {
      setError(null);
      const result = await listQueryStats({ sort });
      setQueryData(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load query stats");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (tab === "requests") {
      loadRequests(appliedFilterValues);
    } else {
      loadQueries(querySort);
    }
  }, [appliedFilterValues, loadQueries, loadRequests, querySort, tab]);

  if (loading && !requestData && !queryData) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading analytics...
      </div>
    );
  }

  if (error && !requestData && !queryData) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{error}</p>
          <button
            onClick={() => { setLoading(true); if (tab === "requests") { loadRequests(appliedFilterValues); } else { loadQueries(querySort); } }}
            className="mt-2 text-sm text-blue-600 hover:underline"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="text-lg font-semibold">Analytics</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
          Request logs and query performance insights
        </p>
      </div>

      <div className="flex gap-1 mb-4 border-b">
        <button
          onClick={() => setTab("requests")}
          className={cn(TAB_CLASS, tab === "requests" ? TAB_ACTIVE : TAB_INACTIVE)}
        >
          Request Logs
        </button>
        <button
          onClick={() => setTab("queries")}
          className={cn(TAB_CLASS, tab === "queries" ? TAB_ACTIVE : TAB_INACTIVE)}
        >
          Query Performance
        </button>
      </div>

      {tab === "requests" ? (
        <>
          <FilterBar
            fields={REQUEST_FILTER_FIELDS}
            values={draftFilterValues}
            onChange={setDraftFilterValue}
            onApply={(vals) => {
              setLoading(true);
              applyFilterValues(vals);
            }}
            onReset={() => {
              resetFilterValues();
              setLoading(true);
            }}
          />
          <AdminTable
            columns={REQUEST_COLUMNS}
            rows={requestData?.items ?? []}
            rowKey="id"
            emptyMessage="No request logs found"
          />
        </>
      ) : (
        <>
          <div className="mb-4 flex items-center gap-2">
            <label htmlFor="query-sort" className="text-xs text-gray-600 dark:text-gray-300">
              Sort by
            </label>
            <select
              id="query-sort"
              value={querySort}
              onChange={(e) => { setQuerySort(e.target.value); setLoading(true); }}
              className="border rounded px-2 py-1.5 text-sm bg-white dark:bg-gray-800"
            >
              <option value="total_time">Total Time</option>
              <option value="calls">Calls</option>
              <option value="mean_time">Mean Time</option>
            </select>
          </div>
          <AdminTable
            columns={QUERY_COLUMNS}
            rows={queryData?.items ?? []}
            rowKey="queryid"
            emptyMessage="No query statistics available"
          />
        </>
      )}
    </div>
  );
}
