import { useState, useCallback, useEffect, useRef } from "react";
import { CheckCircle2, XCircle, ChevronDown, ChevronRight, Loader2 } from "lucide-react";
import { cn } from "../../lib/utils";
import { listEdgeFunctionLogs } from "../../api";
import type { EdgeFunctionLogEntry } from "../../types";

interface FunctionLogsProps {
  functionId: string;
  logs: EdgeFunctionLogEntry[];
  onLogsUpdate: (logs: EdgeFunctionLogEntry[]) => void;
  addToast?: (type: "success" | "error", message: string) => void;
}

const TRIGGER_TYPES = [
  { label: "All", value: "" },
  { label: "HTTP", value: "http" },
  { label: "DB", value: "db" },
  { label: "Cron", value: "cron" },
  { label: "Storage", value: "storage" },
  { label: "Function", value: "function" },
] as const;

const STATUS_OPTIONS = [
  { label: "All", value: "" },
  { label: "Success", value: "success" },
  { label: "Error", value: "error" },
] as const;

const PAGE_SIZE = 20;
const POLL_INTERVAL_MS = 2000;

export function FunctionLogs({ functionId, logs, onLogsUpdate, addToast }: FunctionLogsProps) {
  const [statusFilter, setStatusFilter] = useState("");
  const [triggerFilter, setTriggerFilter] = useState("");
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [hasMore, setHasMore] = useState(logs.length >= PAGE_SIZE);
  const fetchInFlightRef = useRef(false);
  const pendingFetchRef = useRef<{
    page: number;
    status: string;
    trigger: string;
    showLoading: boolean;
  } | null>(null);

  const fetchLogs = useCallback(
    async (p: number, status: string, trigger: string, options?: { showLoading?: boolean }) => {
      const showLoading = options?.showLoading !== false;
      if (fetchInFlightRef.current) {
        const pending = pendingFetchRef.current;
        pendingFetchRef.current = {
          page: p,
          status,
          trigger,
          showLoading: showLoading || pending?.showLoading === true,
        };
        return;
      }
      fetchInFlightRef.current = true;
      if (showLoading) setLoading(true);
      try {
        const data = await listEdgeFunctionLogs(functionId, {
          page: p,
          perPage: PAGE_SIZE,
          status: status ? (status as "success" | "error") : undefined,
          trigger_type: trigger || undefined,
        });
        const entries = Array.isArray(data) ? data : [];
        onLogsUpdate(entries);
        setHasMore(entries.length >= PAGE_SIZE);
      } catch (e) {
        addToast?.("error", e instanceof Error ? e.message : "Failed to load logs");
      } finally {
        if (showLoading) setLoading(false);
        fetchInFlightRef.current = false;
        const pending = pendingFetchRef.current;
        pendingFetchRef.current = null;
        if (pending) {
          void fetchLogs(pending.page, pending.status, pending.trigger, { showLoading: pending.showLoading });
        }
      }
    },
    [functionId, onLogsUpdate, addToast],
  );

  useEffect(() => {
    void fetchLogs(page, statusFilter, triggerFilter, { showLoading: true });
    const intervalID = window.setInterval(() => {
      void fetchLogs(page, statusFilter, triggerFilter, { showLoading: false });
    }, POLL_INTERVAL_MS);

    return () => {
      window.clearInterval(intervalID);
    };
  }, [fetchLogs, page, statusFilter, triggerFilter]);

  const handleStatusChange = (val: string) => {
    setStatusFilter(val);
    setPage(1);
  };

  const handleTriggerChange = (val: string) => {
    setTriggerFilter(val);
    setPage(1);
  };

  const handlePrevPage = () => {
    if (page <= 1) return;
    setPage((prev) => prev - 1);
  };

  const handleNextPage = () => {
    if (!hasMore) return;
    setPage((prev) => prev + 1);
  };

  const filteredEmpty = logs.length === 0 && (statusFilter || triggerFilter);

  return (
    <div data-testid="function-logs">
      {/* Filters */}
      <div className="flex items-center gap-3 mb-4" data-testid="log-filters">
        <div className="flex items-center gap-1.5">
          <label className="text-xs text-gray-500 dark:text-gray-400">Status:</label>
          <select
            value={statusFilter}
            onChange={(e) => handleStatusChange(e.target.value)}
            className="text-xs px-2 py-1 border rounded bg-white dark:bg-gray-800"
            aria-label="Filter by status"
            data-testid="log-status-filter"
          >
            {STATUS_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>
        </div>
        <div className="flex items-center gap-1.5">
          <label className="text-xs text-gray-500 dark:text-gray-400">Trigger:</label>
          <select
            value={triggerFilter}
            onChange={(e) => handleTriggerChange(e.target.value)}
            className="text-xs px-2 py-1 border rounded bg-white dark:bg-gray-800"
            aria-label="Filter by trigger type"
            data-testid="log-trigger-filter"
          >
            {TRIGGER_TYPES.map((opt) => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>
        </div>
        {loading && <Loader2 className="w-3.5 h-3.5 animate-spin text-gray-400 dark:text-gray-500" />}
      </div>

      {logs.length === 0 && !filteredEmpty ? (
        <p className="text-gray-400 dark:text-gray-500 text-sm py-6 text-center" data-testid="logs-empty">
          No execution logs yet.
        </p>
      ) : logs.length === 0 && filteredEmpty ? (
        <p className="text-gray-400 dark:text-gray-500 text-sm py-6 text-center" data-testid="logs-no-match">
          No matching logs for the selected filters.
        </p>
      ) : (
        <>
          <div className="border rounded-lg overflow-hidden">
            <table className="w-full text-sm" data-testid="logs-table">
              <thead className="bg-gray-50 dark:bg-gray-800 border-b">
                <tr>
                  <th className="w-8 px-2"></th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Status</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Duration</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Method</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Path</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Trigger</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Time</th>
                </tr>
              </thead>
              <tbody>
                {logs.map((log) => (
                  <LogRow key={log.id} log={log} />
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          <div className="flex items-center justify-between mt-3" data-testid="log-pagination">
            <button
              onClick={handlePrevPage}
              disabled={page <= 1 || loading}
              className="text-xs px-3 py-1 border rounded disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
              data-testid="log-prev-page"
            >
              Previous
            </button>
            <span className="text-xs text-gray-500 dark:text-gray-400" data-testid="log-page-info">
              Page {page}
            </span>
            <button
              onClick={handleNextPage}
              disabled={!hasMore || loading}
              className="text-xs px-3 py-1 border rounded disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
              data-testid="log-next-page"
            >
              Next
            </button>
          </div>
        </>
      )}
    </div>
  );
}

function LogRow({ log }: { log: EdgeFunctionLogEntry }) {
  const [expanded, setExpanded] = useState(false);
  const hasOutput = !!(log.stdout || log.error);

  return (
    <>
      <tr
        className={cn(
          "border-b last:border-0 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800",
          hasOutput && "cursor-pointer",
        )}
        onClick={() => hasOutput && setExpanded(!expanded)}
        data-testid={`log-row-${log.id}`}
      >
        <td className="px-2 py-2 text-gray-400 dark:text-gray-500">
          {hasOutput && (
            expanded ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />
          )}
        </td>
        <td className="px-4 py-2">
          {log.status === "success" ? (
            <CheckCircle2 className="w-4 h-4 text-green-500 inline" />
          ) : (
            <XCircle className="w-4 h-4 text-red-500 inline" />
          )}
        </td>
        <td className="px-4 py-2 font-mono text-xs">{log.durationMs}ms</td>
        <td className="px-4 py-2" data-testid={`log-method-${log.id}`}>{log.requestMethod || "-"}</td>
        <td className="px-4 py-2 font-mono text-xs" data-testid={`log-path-${log.id}`}>{log.requestPath || "-"}</td>
        <td className="px-4 py-2 text-xs">
          {log.triggerType ? (
            <span className={cn(
              "px-1.5 py-0.5 rounded text-xs font-medium",
              triggerBadgeClass(log.triggerType),
            )} data-testid={`log-trigger-${log.id}`}>
              {log.triggerType}
            </span>
          ) : (
            <span className="text-gray-400 dark:text-gray-500">-</span>
          )}
        </td>
        <td className="px-4 py-2 text-gray-500 dark:text-gray-400 text-xs">
          {new Date(log.createdAt).toLocaleString()}
        </td>
      </tr>
      {expanded && hasOutput && (
        <tr className="bg-gray-50 dark:bg-gray-800">
          <td colSpan={7} className="px-4 py-3">
            {log.stdout && (
              <div className="mb-2">
                <span className="text-xs font-medium text-gray-500 dark:text-gray-400">stdout:</span>
                <pre className="text-xs font-mono bg-white dark:bg-gray-800 border rounded p-2 mt-1 whitespace-pre-wrap">
                  {log.stdout}
                </pre>
              </div>
            )}
            {log.error && (
              <div>
                <span className="text-xs font-medium text-red-500">error:</span>
                <pre className="text-xs font-mono bg-red-50 border border-red-200 rounded p-2 mt-1 whitespace-pre-wrap">
                  {log.error}
                </pre>
              </div>
            )}
            {log.triggerType && log.triggerId && (
              <div className="mt-2 text-xs text-gray-500 dark:text-gray-400">
                Trigger: {log.triggerType} ({log.triggerId})
                {log.parentInvocationId && (
                  <span className="ml-2">Parent: {log.parentInvocationId}</span>
                )}
              </div>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

function triggerBadgeClass(triggerType: string): string {
  switch (triggerType) {
    case "http": return "bg-blue-100 text-blue-700";
    case "db": return "bg-purple-100 text-purple-700";
    case "cron": return "bg-orange-100 text-orange-700";
    case "storage": return "bg-teal-100 text-teal-700";
    case "function": return "bg-indigo-100 text-indigo-700";
    default: return "bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300";
  }
}
