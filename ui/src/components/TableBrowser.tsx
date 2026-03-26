import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import type { Table, ListResponse } from "../types";
import { getRows, createRecord, updateRecord, deleteRecord, batchRecords, ApiError } from "../api";
import { RecordForm } from "./RecordForm";
import { TableBrowserToolbar } from "./TableBrowserToolbar";
import { TableBrowserGrid } from "./TableBrowserGrid";
import type { ExpandColumn } from "./TableBrowserGrid";
import { RowDetail, DeleteConfirm, BatchDeleteConfirm } from "./TableBrowserDialogs";

const PER_PAGE = 20;
const RATE_LIMIT_RETRY_BUDGET_MS = 30000;
const RATE_LIMIT_RETRY_DELAYS_MS = [200, 500, 1000, 2000];

type Modal =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "edit"; row: Record<string, unknown> }
  | { kind: "detail"; row: Record<string, unknown> }
  | { kind: "delete"; row: Record<string, unknown> }
  | { kind: "batch-delete" };

interface TableBrowserProps {
  table: Table;
}

export function TableBrowser({ table }: TableBrowserProps) {
  const [data, setData] = useState<ListResponse | null>(null);
  const [page, setPage] = useState(1);
  const [sort, setSort] = useState<string | null>(null);
  const [filter, setFilter] = useState("");
  const [appliedFilter, setAppliedFilter] = useState("");
  const [search, setSearch] = useState("");
  const [appliedSearch, setAppliedSearch] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [modal, setModal] = useState<Modal>({ kind: "none" });
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [expandedRelations, setExpandedRelations] = useState<Set<string>>(new Set());
  const prevTableRef = useRef(table.name);
  const fetchRunRef = useRef(0);

  const isWritable = table.kind === "table" || table.kind === "partitioned_table";
  const hasPK = (table.primaryKey?.length ?? 0) > 0;

  // Compute available many-to-one FK relationships for expand.
  const expandableRelations = useMemo(() => {
    if (!table.relationships) return [];
    return table.relationships.filter((r) => r.type === "many-to-one");
  }, [table.relationships]);

  // Build expand query param from selected relations.
  const expandParam = useMemo(() => {
    if (expandedRelations.size === 0) return undefined;
    return Array.from(expandedRelations).join(",");
  }, [expandedRelations]);

  // Reset state when table changes.
  useEffect(() => {
    if (prevTableRef.current !== table.name) {
      setPage(1);
      setSort(null);
      setFilter("");
      setAppliedFilter("");
      setSearch("");
      setAppliedSearch("");
      setModal({ kind: "none" });
      setSelectedIds(new Set());
      setExpandedRelations(new Set());
      prevTableRef.current = table.name;
    }
  }, [table.name]);

  const fetchData = useCallback(async () => {
    const runID = ++fetchRunRef.current;
    const isCurrentRun = () => fetchRunRef.current === runID;
    setLoading(true);
    setError(null);
    const retryDeadline = Date.now() + RATE_LIMIT_RETRY_BUDGET_MS;
    let retryDelayIndex = 0;
    for (;;) {
      try {
        const result = await getRows(table.name, {
          page,
          perPage: PER_PAGE,
          sort: sort || undefined,
          filter: appliedFilter || undefined,
          search: appliedSearch || undefined,
          expand: expandParam,
        });
        if (!isCurrentRun()) {
          return;
        }
        setData(result);
        break;
      } catch (e) {
        if (!isCurrentRun()) {
          return;
        }
        const remainingRetryBudgetMs = retryDeadline - Date.now();
        const shouldRetryRateLimit =
          e instanceof ApiError &&
          e.status === 429 &&
          remainingRetryBudgetMs > 0;
        if (shouldRetryRateLimit) {
          const backoffDelayMs =
            RATE_LIMIT_RETRY_DELAYS_MS[
              Math.min(retryDelayIndex, RATE_LIMIT_RETRY_DELAYS_MS.length - 1)
            ];
          const retryAfterDelayMs =
            e.retryAfterSeconds !== undefined ? e.retryAfterSeconds * 1000 : 0;
          const delayMs = Math.min(
            Math.max(backoffDelayMs, retryAfterDelayMs),
            remainingRetryBudgetMs,
          );
          retryDelayIndex += 1;
          if (delayMs <= 0) {
            setError(e.message);
            setData(null);
            break;
          }
          await new Promise((resolve) => setTimeout(resolve, delayMs));
          continue;
        }
        setError(e instanceof ApiError ? e.message : "Failed to load data");
        setData(null);
        break;
      }
    }
    if (isCurrentRun()) {
      setLoading(false);
    }
  }, [table.name, page, sort, appliedFilter, appliedSearch, expandParam]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const toggleSort = useCallback(
    (col: string) => {
      setSort((prev) => {
        if (prev === `+${col}` || prev === col) return `-${col}`;
        return `+${col}`;
      });
      setPage(1);
    },
    [],
  );

  const handleFilterSubmit = useCallback(() => {
    setAppliedFilter(filter);
    setPage(1);
  }, [filter]);

  const handleSearchSubmit = useCallback(() => {
    setAppliedSearch(search);
    setPage(1);
  }, [search]);

  const pkId = useCallback(
    (row: Record<string, unknown>): string => {
      return table.primaryKey.map((k) => String(row[k])).join(",");
    },
    [table.primaryKey],
  );

  const handleCreate = useCallback(
    async (formData: Record<string, unknown>) => {
      await createRecord(table.name, formData);
      setModal({ kind: "none" });
      fetchData();
    },
    [table.name, fetchData],
  );

  const handleUpdate = useCallback(
    async (formData: Record<string, unknown>) => {
      if (modal.kind !== "edit") return;
      const id = pkId(modal.row);
      await updateRecord(table.name, id, formData);
      setModal({ kind: "none" });
      fetchData();
    },
    [table.name, modal, pkId, fetchData],
  );

  const handleDelete = useCallback(async () => {
    if (modal.kind !== "delete") return;
    const id = pkId(modal.row);
    await deleteRecord(table.name, id);
    setModal({ kind: "none" });
    fetchData();
  }, [table.name, modal, pkId, fetchData]);

  const handleBatchDelete = useCallback(async () => {
    if (selectedIds.size === 0) return;
    const ops = Array.from(selectedIds).map((id) => ({
      method: "delete" as const,
      id,
    }));
    await batchRecords(table.name, ops);
    setSelectedIds(new Set());
    setModal({ kind: "none" });
    fetchData();
  }, [table.name, selectedIds, fetchData]);

  // Selection helpers.
  const toggleSelect = useCallback(
    (id: string) => {
      setSelectedIds((prev) => {
        const next = new Set(prev);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        return next;
      });
    },
    [],
  );

  const toggleSelectAll = useCallback(() => {
    if (!data) return;
    const allIds = data.items.map((row) => pkId(row));
    const allSelected = allIds.every((id) => selectedIds.has(id));
    if (allSelected) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(allIds));
    }
  }, [data, pkId, selectedIds]);

  // Expand toggle.
  const toggleExpand = useCallback((fieldName: string) => {
    setExpandedRelations((prev) => {
      const next = new Set(prev);
      if (next.has(fieldName)) next.delete(fieldName);
      else next.add(fieldName);
      return next;
    });
  }, []);

  const columns = table.columns;

  // Extra columns from expanded relations.
  const expandColumns: ExpandColumn[] = useMemo(() => {
    const cols: ExpandColumn[] = [];
    for (const rel of expandableRelations) {
      if (expandedRelations.has(rel.fieldName)) {
        cols.push({ relation: rel, label: rel.fieldName });
      }
    }
    return cols;
  }, [expandableRelations, expandedRelations]);

  const handleExport = useCallback(
    (format: "csv" | "json") => {
      if (!data || data.items.length === 0) return;
      const colNames = columns.map((c) => c.name);

      let content: string;
      let mimeType: string;
      let ext: string;

      if (format === "csv") {
        const escapeCsv = (val: unknown): string => {
          if (val === null || val === undefined) return "";
          const s = typeof val === "object" ? JSON.stringify(val) : String(val);
          if (s.includes(",") || s.includes('"') || s.includes("\n")) {
            return `"${s.replace(/"/g, '""')}"`;
          }
          return s;
        };
        const header = colNames.map(escapeCsv).join(",");
        const rows = data.items.map((row) =>
          colNames.map((c) => escapeCsv(row[c])).join(","),
        );
        content = [header, ...rows].join("\n");
        mimeType = "text/csv";
        ext = "csv";
      } else {
        content = JSON.stringify(data.items, null, 2);
        mimeType = "application/json";
        ext = "json";
      }

      const blob = new Blob([content], { type: mimeType });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${table.name}_page${page}.${ext}`;
      a.click();
      URL.revokeObjectURL(url);
    },
    [data, columns, table.name, page],
  );

  const showCheckboxes = isWritable && hasPK;

  return (
    <div className="flex flex-col h-full text-gray-900 dark:text-gray-100">
      {/* Toolbar */}
      <TableBrowserToolbar
        search={search}
        setSearch={setSearch}
        handleSearchSubmit={handleSearchSubmit}
        setAppliedSearch={setAppliedSearch}
        filter={filter}
        setFilter={setFilter}
        handleFilterSubmit={handleFilterSubmit}
        setAppliedFilter={setAppliedFilter}
        setPage={setPage as (fn: (p: number) => number) => void}
        expandableRelations={expandableRelations}
        expandedRelations={expandedRelations}
        toggleExpand={toggleExpand}
        hasData={!!data}
        hasItems={!!data && data.items.length > 0}
        onExport={handleExport}
        selectedCount={selectedIds.size}
        onBatchDelete={() => setModal({ kind: "batch-delete" })}
        isWritable={isWritable}
        onCreateNew={() => setModal({ kind: "create" })}
      />

      {/* Error */}
      {error && (
        <div className="px-4 py-2 bg-red-50 dark:bg-red-950/30 text-red-600 dark:text-red-300 text-sm border-b border-red-200 dark:border-red-900/60">
          {error}
        </div>
      )}

      {/* Grid + Pagination */}
      <TableBrowserGrid
        data={data}
        loading={loading}
        columns={columns}
        expandColumns={expandColumns}
        sort={sort}
        toggleSort={toggleSort}
        showCheckboxes={showCheckboxes}
        isWritable={isWritable}
        hasPK={hasPK}
        selectedIds={selectedIds}
        toggleSelectAll={toggleSelectAll}
        toggleSelect={toggleSelect}
        pkId={pkId}
        onRowClick={(row) => setModal({ kind: "detail", row })}
        onEdit={(row) => setModal({ kind: "edit", row })}
        onDelete={(row) => setModal({ kind: "delete", row })}
        page={page}
        setPage={setPage}
      />

      {/* Create form */}
      {modal.kind === "create" && (
        <RecordForm
          columns={columns}
          primaryKey={table.primaryKey}
          onSubmit={handleCreate}
          onClose={() => setModal({ kind: "none" })}
          mode="create"
        />
      )}

      {/* Edit form */}
      {modal.kind === "edit" && (
        <RecordForm
          columns={columns}
          primaryKey={table.primaryKey}
          initialData={modal.row}
          onSubmit={handleUpdate}
          onClose={() => setModal({ kind: "none" })}
          mode="edit"
        />
      )}

      {/* Row detail drawer */}
      {modal.kind === "detail" && (
        <RowDetail
          row={modal.row}
          columns={columns}
          expandColumns={expandColumns}
          isWritable={isWritable && hasPK}
          onClose={() => setModal({ kind: "none" })}
          onEdit={() => setModal({ kind: "edit", row: modal.row })}
          onDelete={() => setModal({ kind: "delete", row: modal.row })}
        />
      )}

      {/* Delete confirmation */}
      {modal.kind === "delete" && (
        <DeleteConfirm
          row={modal.row}
          primaryKey={table.primaryKey}
          tableName={table.name}
          onConfirm={handleDelete}
          onCancel={() => setModal({ kind: "none" })}
        />
      )}

      {/* Batch delete confirmation */}
      {modal.kind === "batch-delete" && (
        <BatchDeleteConfirm
          count={selectedIds.size}
          tableName={table.name}
          onConfirm={handleBatchDelete}
          onCancel={() => setModal({ kind: "none" })}
        />
      )}
    </div>
  );
}
