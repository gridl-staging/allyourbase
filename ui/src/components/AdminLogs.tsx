import { useMemo, useState } from "react";
import { listAdminLogs } from "../api_logs";
import { usePolling } from "../hooks/usePolling";
import { parseDateTimeToMs } from "../lib/dateTime";
import type { AdminLogEntry, AdminLogLevel } from "../types/logs";
import { toPanelError } from "./advisors/panelError";
import { useAppToast } from "./ToastProvider";

interface AdminLogsProps {
  pollMs?: number;
}

type LogFilterLevel = "" | AdminLogLevel;

const LEVEL_OPTIONS: ReadonlyArray<{ value: LogFilterLevel; label: string }> = [
  { value: "", label: "All levels" },
  { value: "debug", label: "DEBUG" },
  { value: "info", label: "INFO" },
  { value: "warn", label: "WARN" },
  { value: "error", label: "ERROR" },
  { value: "unknown", label: "UNKNOWN" },
];

interface FilterState {
  searchQuery: string;
  selectedLevel: LogFilterLevel;
  fromDateTime: string;
  toDateTime: string;
}

interface SerializableAdminLogEntry {
  id: string;
  time: string;
  level: AdminLogLevel;
  levelLabel: string;
  message: string;
  attrs: Record<string, unknown>;
  attrsText: string;
}

function formatEntryTime(entry: AdminLogEntry): string {
  if (entry.parsedTimeMs === null) {
    return entry.time || "-";
  }

  return new Date(entry.parsedTimeMs).toLocaleString();
}

function sortByNewestFirst(left: AdminLogEntry, right: AdminLogEntry): number {
  if (left.parsedTimeMs !== null && right.parsedTimeMs !== null) {
    return right.parsedTimeMs - left.parsedTimeMs;
  }
  if (left.parsedTimeMs !== null) {
    return -1;
  }
  if (right.parsedTimeMs !== null) {
    return 1;
  }
  return right.time.localeCompare(left.time);
}

function matchesLevelFilter(entry: AdminLogEntry, filterLevel: LogFilterLevel): boolean {
  return !filterLevel || entry.level === filterLevel;
}

function matchesTimeFilters(
  entry: AdminLogEntry,
  fromMs: number | null,
  toMs: number | null,
): boolean {
  if (fromMs === null && toMs === null) {
    return true;
  }

  if (entry.parsedTimeMs === null) {
    return false;
  }

  if (fromMs !== null && entry.parsedTimeMs < fromMs) {
    return false;
  }

  if (toMs !== null && entry.parsedTimeMs > toMs) {
    return false;
  }

  return true;
}

function filterEntries(entries: AdminLogEntry[], filters: FilterState): AdminLogEntry[] {
  const normalizedQuery = filters.searchQuery.trim().toLowerCase();
  const fromMs = parseDateTimeToMs(filters.fromDateTime);
  const toMs = parseDateTimeToMs(filters.toDateTime);

  return [...entries]
    .filter((entry) => {
      if (normalizedQuery && !entry.searchText.includes(normalizedQuery)) {
        return false;
      }
      if (!matchesLevelFilter(entry, filters.selectedLevel)) {
        return false;
      }
      return matchesTimeFilters(entry, fromMs, toMs);
    })
    .sort(sortByNewestFirst);
}

function toSerializableLogEntry(entry: AdminLogEntry): SerializableAdminLogEntry {
  return {
    id: entry.id,
    time: entry.time,
    level: entry.level,
    levelLabel: entry.levelLabel,
    message: entry.message,
    attrs: entry.attrs,
    attrsText: entry.attrsText,
  };
}

function formatAttrsValueSummary(value: unknown): string {
  if (Array.isArray(value)) {
    return `[${value.length}]`;
  }
  if (value !== null && typeof value === "object") {
    return "{...}";
  }
  const rendered = String(value);
  return rendered.length > 24 ? `${rendered.slice(0, 21)}...` : rendered;
}

function formatAttrsSummary(entry: AdminLogEntry): string {
  if (entry.attrsText === "{}") {
    return "-";
  }
  const attrEntries = Object.entries(entry.attrs);
  const summaryFields = attrEntries
    .slice(0, 2)
    .map(([key, value]) => `${key}=${formatAttrsValueSummary(value)}`);
  const remainingFields = attrEntries.length - summaryFields.length;
  if (remainingFields > 0) {
    return `${summaryFields.join(", ")}, +${remainingFields} more`;
  }
  return summaryFields.join(", ");
}

function formatExpandedAttrsJson(entry: AdminLogEntry): string {
  if (entry.attrsText === "{}") {
    return "";
  }
  try {
    return JSON.stringify(entry.attrs, null, 2);
  } catch {
    return entry.attrsText;
  }
}

function exportFilteredEntries(entries: AdminLogEntry[]): void {
  if (entries.length === 0) {
    return;
  }
  const content = JSON.stringify(entries.map((entry) => toSerializableLogEntry(entry)), null, 2);
  const blob = new Blob([content], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  try {
    const link = document.createElement("a");
    link.href = url;
    link.download = "admin_logs_filtered.json";
    link.click();
  } finally {
    URL.revokeObjectURL(url);
  }
}

interface AdminLogsFiltersProps {
  filters: FilterState;
  onSearchQueryChange: (value: string) => void;
  onLevelChange: (value: LogFilterLevel) => void;
  onFromDateTimeChange: (value: string) => void;
  onToDateTimeChange: (value: string) => void;
}

function AdminLogsFilters({
  filters,
  onSearchQueryChange,
  onLevelChange,
  onFromDateTimeChange,
  onToDateTimeChange,
}: AdminLogsFiltersProps) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-4 gap-3 rounded border border-gray-200 dark:border-gray-700 p-3">
      <input
        aria-label="Search logs"
        placeholder="Search messages or attrs"
        value={filters.searchQuery}
        onChange={(event) => onSearchQueryChange(event.target.value)}
        className="border border-gray-300 dark:border-gray-700 rounded px-2 py-1.5 text-sm bg-white dark:bg-gray-900"
      />
      <select
        aria-label="Level"
        value={filters.selectedLevel}
        onChange={(event) => onLevelChange(event.target.value as LogFilterLevel)}
        className="border border-gray-300 dark:border-gray-700 rounded px-2 py-1.5 text-sm bg-white dark:bg-gray-900"
      >
        {LEVEL_OPTIONS.map((option) => (
          <option key={option.label} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      <input
        aria-label="From"
        type="datetime-local"
        value={filters.fromDateTime}
        onChange={(event) => onFromDateTimeChange(event.target.value)}
        className="border border-gray-300 dark:border-gray-700 rounded px-2 py-1.5 text-sm bg-white dark:bg-gray-900"
      />
      <input
        aria-label="To"
        type="datetime-local"
        value={filters.toDateTime}
        onChange={(event) => onToDateTimeChange(event.target.value)}
        className="border border-gray-300 dark:border-gray-700 rounded px-2 py-1.5 text-sm bg-white dark:bg-gray-900"
      />
    </div>
  );
}

interface AdminLogsTableProps {
  entries: AdminLogEntry[];
  onCopyRow: (entry: AdminLogEntry) => Promise<void>;
}

function AdminLogsTable({ entries, onCopyRow }: AdminLogsTableProps) {
  const [expandedAttrEntryIds, setExpandedAttrEntryIds] = useState<Set<string>>(new Set());

  const toggleAttrInspect = (entryId: string) => {
    setExpandedAttrEntryIds((prev) => {
      const next = new Set(prev);
      if (next.has(entryId)) {
        next.delete(entryId);
      } else {
        next.add(entryId);
      }
      return next;
    });
  };

  return (
    <div className="rounded border border-gray-200 dark:border-gray-700 overflow-x-auto">
      <table className="w-full text-xs">
        <thead>
          <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
            <th className="py-2 px-3">Time</th>
            <th className="py-2 px-3">Level</th>
            <th className="py-2 px-3">Message</th>
            <th className="py-2 px-3">Attributes</th>
            <th className="py-2 px-3">Actions</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry) => {
            const hasAttrs = entry.attrsText !== "{}";
            const isInspectOpen = expandedAttrEntryIds.has(entry.id);
            return (
              <tr key={entry.id} className="border-b last:border-b-0 border-gray-200 dark:border-gray-700">
                <td className="py-2 px-3 whitespace-nowrap">{formatEntryTime(entry)}</td>
                <td className="py-2 px-3 whitespace-nowrap">{entry.levelLabel}</td>
                <td className="py-2 px-3">{entry.message || "-"}</td>
                <td className="py-2 px-3 align-top">
                  <div
                    className="font-mono break-all text-[11px]"
                    data-testid={`admin-log-attrs-summary-${entry.id}`}
                  >
                    {formatAttrsSummary(entry)}
                  </div>
                  {hasAttrs ? (
                    <div className="mt-1 space-y-1">
                      <button
                        type="button"
                        aria-expanded={isInspectOpen}
                        aria-label={`Inspect attrs ${entry.id}`}
                        onClick={() => toggleAttrInspect(entry.id)}
                        className="text-[11px] text-blue-700 dark:text-blue-300 underline"
                      >
                        {isInspectOpen ? "Hide JSON" : "Inspect JSON"}
                      </button>
                      {isInspectOpen ? (
                        <pre
                          className="rounded bg-gray-100 dark:bg-gray-900 p-2 text-[11px] overflow-x-auto"
                          data-testid={`admin-log-attrs-json-${entry.id}`}
                        >
                          {formatExpandedAttrsJson(entry)}
                        </pre>
                      ) : null}
                    </div>
                  ) : null}
                </td>
                <td className="py-2 px-3 align-top">
                  <button
                    type="button"
                    aria-label={`Copy log ${entry.id}`}
                    onClick={() => void onCopyRow(entry)}
                    className="px-2 py-1 text-[11px] rounded border border-gray-300 dark:border-gray-600"
                  >
                    Copy
                  </button>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export function AdminLogs({ pollMs = 10000 }: AdminLogsProps) {
  const { addToast } = useAppToast();
  const [filters, setFilters] = useState<FilterState>({
    searchQuery: "",
    selectedLevel: "",
    fromDateTime: "",
    toDateTime: "",
  });
  const [isPaused, setIsPaused] = useState(false);
  const { data, loading, error, refresh } = usePolling(() => listAdminLogs(), pollMs, {
    enabled: !isPaused,
  });
  const filteredEntries = useMemo(
    () => filterEntries(data?.entries ?? [], filters),
    [data?.entries, filters],
  );
  const showEmptyState = Boolean(data) && filteredEntries.length === 0;
  const showTable = Boolean(data) && filteredEntries.length > 0;

  const handleRefresh = async () => {
    await refresh();
  };

  const handleCopyRow = async (entry: AdminLogEntry) => {
    if (!navigator.clipboard?.writeText) {
      addToast("error", "Clipboard is unavailable");
      return;
    }
    try {
      const serializedEntry = JSON.stringify(toSerializableLogEntry(entry), null, 2);
      await navigator.clipboard.writeText(serializedEntry);
      addToast("success", "Log row copied");
    } catch {
      addToast("error", "Failed to copy log row");
    }
  };

  const handleExportFiltered = () => {
    if (filteredEntries.length === 0) {
      addToast("warning", "No logs available for export");
      return;
    }
    try {
      exportFilteredEntries(filteredEntries);
      addToast("success", `Exported ${filteredEntries.length} log row(s)`);
    } catch {
      addToast("error", "Failed to export logs");
    }
  };

  return (
    <div className="p-6 space-y-4" data-testid="admin-logs-panel">
      <div className="flex items-center gap-2 flex-wrap">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Admin Logs</h2>
        <span
          className="text-xs text-gray-500 dark:text-gray-400"
          data-testid="admin-logs-polling-status"
        >
          {isPaused ? "Auto-refresh paused" : `Auto-refresh every ${pollMs}ms`}
        </span>
        <button
          type="button"
          onClick={() => setIsPaused((prev) => !prev)}
          className="ml-auto px-3 py-1 text-xs rounded border border-gray-300 dark:border-gray-600"
        >
          {isPaused ? "Resume auto-refresh" : "Pause auto-refresh"}
        </button>
        <button
          type="button"
          onClick={handleExportFiltered}
          className="px-3 py-1 text-xs rounded border border-gray-300 dark:border-gray-600"
        >
          Export filtered JSON
        </button>
        <button
          type="button"
          onClick={() => void handleRefresh()}
          className="px-3 py-1 text-xs rounded bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900"
        >
          Refresh
        </button>
      </div>
      {Boolean(error) && <div className="text-sm text-red-600">{toPanelError(error)}</div>}
      {loading && !data && <div className="text-sm text-gray-500">Loading admin logs...</div>}
      {data && !data.bufferingEnabled && (
        <div className="text-sm text-amber-700 dark:text-amber-300" data-testid="admin-logs-buffering-message">
          {data.message ?? "Log buffering is not enabled"}
        </div>
      )}
      <AdminLogsFilters
        filters={filters}
        onSearchQueryChange={(searchQuery) => setFilters((prev) => ({ ...prev, searchQuery }))}
        onLevelChange={(selectedLevel) => setFilters((prev) => ({ ...prev, selectedLevel }))}
        onFromDateTimeChange={(fromDateTime) => setFilters((prev) => ({ ...prev, fromDateTime }))}
        onToDateTimeChange={(toDateTime) => setFilters((prev) => ({ ...prev, toDateTime }))}
      />
      {showEmptyState ? <p className="text-sm text-gray-500">No log entries found</p> : null}
      {showTable ? <AdminLogsTable entries={filteredEntries} onCopyRow={handleCopyRow} /> : null}
    </div>
  );
}
