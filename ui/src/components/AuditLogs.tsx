import { useCallback, useEffect, useState } from "react";
import { listAuditLogs } from "../api_audit";
import type { AuditLogEntry } from "../types/audit";
import { AdminTable, type Column } from "./shared/AdminTable";
import { FilterBar, type FilterField } from "./shared/FilterBar";
import { useDraftFilters } from "../hooks/useDraftFilters";

const PAGE_SIZE = 100;
const CHANGE_DETAILS_REGION_ID = "audit-change-details-region";

const filterFields: FilterField[] = [
  { name: "table", label: "Table", type: "text", placeholder: "Table name" },
  {
    name: "operation",
    label: "Operation",
    type: "select",
    options: [
      { value: "", label: "All" },
      { value: "INSERT", label: "INSERT" },
      { value: "UPDATE", label: "UPDATE" },
      { value: "DELETE", label: "DELETE" },
    ],
  },
  { name: "from", label: "From", type: "date" },
  { name: "to", label: "To", type: "date" },
];

const columns: Column<AuditLogEntry>[] = [
  { key: "operation", header: "Operation" },
  { key: "table_name", header: "Table" },
  {
    key: "timestamp",
    header: "Timestamp",
    render: (row) => new Date(row.timestamp).toLocaleString(),
  },
  {
    key: "user_id",
    header: "User",
    render: (row) => row.user_id ?? "-",
  },
  {
    key: "ip_address",
    header: "IP",
    render: (row) => row.ip_address ?? "-",
  },
];

export function AuditLogs() {
  const [entries, setEntries] = useState<AuditLogEntry[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const { draftValues, appliedValues, setDraftValue, applyValues, resetValues } =
    useDraftFilters({ table: "", operation: "", from: "", to: "" });

  const fetchEntries = useCallback(
    async (offset: number) => {
      setLoading(true);
      setError(null);
      try {
        const res = await listAuditLogs({
          table: appliedValues.table || undefined,
          operation: appliedValues.operation || undefined,
          from: appliedValues.from || undefined,
          to: appliedValues.to || undefined,
          limit: PAGE_SIZE,
          offset,
        });
        setEntries(res.items);
        setTotalCount(res.count);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load");
      } finally {
        setLoading(false);
      }
    },
    [appliedValues],
  );

  useEffect(() => {
    fetchEntries(page * PAGE_SIZE);
  }, [fetchEntries, page]);

  const totalPages = Math.max(1, Math.ceil(totalCount / PAGE_SIZE));
  const expandedEntry = entries.find((entry) => entry.id === expandedId);

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Audit Logs
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
        Audit Logs
      </h2>

      <FilterBar
        fields={filterFields}
        values={draftValues}
        onChange={(name, value) => setDraftValue(name, value)}
        onApply={(values) => {
          setPage(0);
          applyValues(values);
        }}
        onReset={() => {
          setPage(0);
          resetValues();
        }}
      />

      {loading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-4">
          Loading...
        </p>
      ) : (
        <>
          <AdminTable
            columns={columns}
            rows={entries}
            rowKey="id"
            page={page + 1}
            totalPages={totalPages}
            onPageChange={(p) => setPage(p - 1)}
            emptyMessage="No audit log entries found"
          />

          {expandedEntry && (
            <div
              id={CHANGE_DETAILS_REGION_ID}
              role="region"
              aria-label="Audit change details"
              className="mt-2 p-3 bg-gray-100 dark:bg-gray-800 rounded text-xs font-mono whitespace-pre-wrap"
            >
              <p className="font-semibold mb-1">Old Values:</p>
              <p>{JSON.stringify(expandedEntry.old_values, null, 2) ?? "null"}</p>
              <p className="font-semibold mt-2 mb-1">New Values:</p>
              <p>{JSON.stringify(expandedEntry.new_values, null, 2) ?? "null"}</p>
            </div>
          )}

          {entries.length > 0 && (
            <div className="mt-2">
              {entries.map((entry) => (
                <button
                  key={entry.id}
                  onClick={() =>
                    setExpandedId(expandedId === entry.id ? null : entry.id)
                  }
                  aria-expanded={expandedId === entry.id}
                  aria-controls={CHANGE_DETAILS_REGION_ID}
                  className="text-xs text-blue-500 hover:text-blue-600 mr-3"
                >
                  {expandedId === entry.id ? "Hide" : "Show"} changes for{" "}
                  {entry.id.slice(0, 8)}
                </button>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}
