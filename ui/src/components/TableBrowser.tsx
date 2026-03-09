import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import type { Table, ListResponse, Relationship } from "../types";
import { getRows, createRecord, updateRecord, deleteRecord, batchRecords, ApiError } from "../api";
import { RecordForm } from "./RecordForm";
import {
  ChevronUp,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Search,
  FileSearch,
  Plus,
  Pencil,
  Trash2,
  X,
  Download,
  Link,
} from "lucide-react";

const PER_PAGE = 20;

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

  const isWritable = table.kind === "table" || table.kind === "partitioned_table";
  const hasPK = table.primaryKey.length > 0;

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
    setLoading(true);
    setError(null);
    try {
      const result = await getRows(table.name, {
        page,
        perPage: PER_PAGE,
        sort: sort || undefined,
        filter: appliedFilter || undefined,
        search: appliedSearch || undefined,
        expand: expandParam,
      });
      setData(result);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Failed to load data");
      setData(null);
    } finally {
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
  const expandColumns = useMemo(() => {
    const cols: { relation: Relationship; label: string }[] = [];
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
  const extraColCount =
    (showCheckboxes ? 1 : 0) +
    (isWritable && hasPK ? 1 : 0) +
    expandColumns.length;

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="px-4 py-2 border-b flex items-center gap-2 bg-gray-50">
        <FileSearch className="w-4 h-4 text-gray-400" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleSearchSubmit()}
          placeholder="Search..."
          className="w-40 bg-transparent text-sm outline-none placeholder-gray-400"
          aria-label="Full-text search"
        />
        {search && (
          <button
            onClick={() => {
              setSearch("");
              setAppliedSearch("");
              setPage(1);
            }}
            className="text-gray-400 hover:text-gray-600"
            aria-label="Clear search"
          >
            <X className="w-4 h-4" />
          </button>
        )}
        <div className="w-px h-5 bg-gray-300" />
        <Search className="w-4 h-4 text-gray-400" />
        <input
          type="text"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleFilterSubmit()}
          placeholder="Filter... e.g. status='active' && age>25"
          className="flex-1 bg-transparent text-sm outline-none placeholder-gray-400"
        />
        {filter && (
          <button
            onClick={() => {
              setFilter("");
              setAppliedFilter("");
              setPage(1);
            }}
            className="text-gray-400 hover:text-gray-600"
          >
            <X className="w-4 h-4" />
          </button>
        )}
        <button
          onClick={() => { handleFilterSubmit(); handleSearchSubmit(); }}
          className="px-3 py-1 text-xs bg-gray-200 hover:bg-gray-300 rounded font-medium"
        >
          Apply
        </button>
        {expandableRelations.length > 0 && (
          <ExpandMenu
            relations={expandableRelations}
            expanded={expandedRelations}
            onToggle={toggleExpand}
          />
        )}
        {data && data.items.length > 0 && (
          <ExportMenu onExport={handleExport} />
        )}
        {selectedIds.size > 0 && (
          <button
            onClick={() => setModal({ kind: "batch-delete" })}
            className="px-3 py-1 text-xs bg-red-600 text-white hover:bg-red-700 rounded font-medium inline-flex items-center gap-1"
            aria-label="Delete selected"
          >
            <Trash2 className="w-3.5 h-3.5" />
            Delete ({selectedIds.size})
          </button>
        )}
        {isWritable && (
          <button
            onClick={() => setModal({ kind: "create" })}
            className="ml-2 px-3 py-1 text-xs bg-gray-900 text-white hover:bg-gray-800 rounded font-medium inline-flex items-center gap-1"
          >
            <Plus className="w-3.5 h-3.5" />
            New Row
          </button>
        )}
      </div>

      {/* Error */}
      {error && (
        <div className="px-4 py-2 bg-red-50 text-red-600 text-sm border-b">
          {error}
        </div>
      )}

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 sticky top-0">
            <tr>
              {showCheckboxes && (
                <th className="px-2 py-2 border-b w-10">
                  <input
                    type="checkbox"
                    checked={
                      !!data &&
                      data.items.length > 0 &&
                      data.items.every((row) => selectedIds.has(pkId(row)))
                    }
                    onChange={toggleSelectAll}
                    aria-label="Select all"
                  />
                </th>
              )}
              {columns.map((col) => (
                <th
                  key={col.name}
                  onClick={() => toggleSort(col.name)}
                  className="px-4 py-2 text-left font-medium text-gray-600 border-b cursor-pointer hover:bg-gray-100 whitespace-nowrap select-none"
                >
                  <span className="inline-flex items-center gap-1">
                    {col.name}
                    {col.isPrimaryKey && (
                      <span className="text-blue-500 text-xs">PK</span>
                    )}
                    <SortIcon sort={sort} col={col.name} />
                  </span>
                </th>
              ))}
              {expandColumns.map((ec) => (
                <th
                  key={`expand-${ec.label}`}
                  className="px-4 py-2 text-left font-medium text-purple-600 border-b whitespace-nowrap select-none"
                >
                  <span className="inline-flex items-center gap-1">
                    <Link className="w-3 h-3" />
                    {ec.label}
                  </span>
                </th>
              ))}
              {isWritable && hasPK && (
                <th className="px-4 py-2 border-b w-20" />
              )}
            </tr>
          </thead>
          <tbody>
            {loading && !data && (
              <tr>
                <td
                  colSpan={columns.length + extraColCount}
                  className="px-4 py-8 text-center text-gray-400"
                >
                  Loading...
                </td>
              </tr>
            )}
            {data?.items.length === 0 && (
              <tr>
                <td
                  colSpan={columns.length + extraColCount}
                  className="px-4 py-8 text-center text-gray-400"
                >
                  No rows found
                </td>
              </tr>
            )}
            {data?.items.map((row, i) => {
              const rowId = hasPK ? pkId(row) : String(i);
              const isSelected = selectedIds.has(rowId);
              return (
                <tr
                  key={i}
                  onClick={() => setModal({ kind: "detail", row })}
                  className={`border-b hover:bg-blue-50 cursor-pointer group ${isSelected ? "bg-blue-50" : ""}`}
                >
                  {showCheckboxes && (
                    <td className="px-2 py-2 whitespace-nowrap">
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={(e) => {
                          e.stopPropagation();
                          toggleSelect(rowId);
                        }}
                        onClick={(e) => e.stopPropagation()}
                        aria-label={`Select row ${rowId}`}
                      />
                    </td>
                  )}
                  {columns.map((col) => (
                    <td
                      key={col.name}
                      className="px-4 py-2 whitespace-nowrap max-w-xs truncate"
                    >
                      <CellValue value={row[col.name]} />
                    </td>
                  ))}
                  {expandColumns.map((ec) => (
                    <td
                      key={`expand-${ec.label}`}
                      className="px-4 py-2 whitespace-nowrap max-w-xs truncate"
                    >
                      <ExpandedCell row={row} fieldName={ec.label} />
                    </td>
                  ))}
                  {isWritable && hasPK && (
                    <td className="px-2 py-2 whitespace-nowrap">
                      <span className="opacity-0 group-hover:opacity-100 inline-flex gap-1">
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            setModal({ kind: "edit", row });
                          }}
                          className="p-1 hover:bg-gray-200 rounded text-gray-500 hover:text-gray-700"
                          title="Edit"
                        >
                          <Pencil className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            setModal({ kind: "delete", row });
                          }}
                          className="p-1 hover:bg-red-100 rounded text-gray-500 hover:text-red-600"
                          title="Delete"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </span>
                    </td>
                  )}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {data && (
        <div className="px-4 py-2 border-t bg-gray-50 flex items-center justify-between text-sm text-gray-500">
          <span>
            {data.totalItems} row{data.totalItems !== 1 ? "s" : ""}
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1}
              className="p-1 rounded hover:bg-gray-200 disabled:opacity-30"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <span>
              {page} / {data.totalPages || 1}
            </span>
            <button
              onClick={() => setPage((p) => Math.min(data.totalPages, p + 1))}
              disabled={page >= data.totalPages}
              className="p-1 rounded hover:bg-gray-200 disabled:opacity-30"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}

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

function SortIcon({ sort, col }: { sort: string | null; col: string }) {
  if (sort === `+${col}` || sort === col)
    return <ChevronUp className="w-3 h-3 text-blue-500" />;
  if (sort === `-${col}`)
    return <ChevronDown className="w-3 h-3 text-blue-500" />;
  return <ChevronUp className="w-3 h-3 text-transparent" />;
}

function CellValue({ value }: { value: unknown }) {
  if (value === null || value === undefined)
    return <span className="text-gray-300 italic">null</span>;
  if (typeof value === "boolean")
    return <span className={value ? "text-green-600" : "text-gray-400"}>{String(value)}</span>;
  if (typeof value === "object")
    return (
      <span className="text-gray-500 font-mono text-xs">
        {JSON.stringify(value)}
      </span>
    );
  return <>{String(value)}</>;
}

function ExpandedCell({ row, fieldName }: { row: Record<string, unknown>; fieldName: string }) {
  const expandData = row.expand as Record<string, unknown> | undefined;
  if (!expandData || !expandData[fieldName]) {
    return <span className="text-gray-300 italic">null</span>;
  }
  const related = expandData[fieldName] as Record<string, unknown>;
  // Show a summary of the related record — first non-id text field, or the whole thing.
  const keys = Object.keys(related).filter((k) => k !== "id" && k !== "expand");
  const display = keys.length > 0 ? String(related[keys[0]]) : JSON.stringify(related);
  return (
    <span className="text-purple-600 text-xs" title={JSON.stringify(related, null, 2)}>
      {display}
    </span>
  );
}

function ExpandMenu({
  relations,
  expanded,
  onToggle,
}: {
  relations: Relationship[];
  expanded: Set<string>;
  onToggle: (fieldName: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open]);

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen((v) => !v)}
        className={`px-3 py-1 text-xs rounded font-medium inline-flex items-center gap-1 ${
          expanded.size > 0
            ? "bg-purple-100 text-purple-700 hover:bg-purple-200"
            : "bg-gray-200 hover:bg-gray-300"
        }`}
        aria-label="Expand"
      >
        <Link className="w-3.5 h-3.5" />
        Expand{expanded.size > 0 ? ` (${expanded.size})` : ""}
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 bg-white border rounded shadow-lg z-10 py-1 min-w-[180px]">
          {relations.map((rel) => (
            <label
              key={rel.fieldName}
              className="flex items-center gap-2 px-3 py-1.5 text-sm hover:bg-gray-100 cursor-pointer"
            >
              <input
                type="checkbox"
                checked={expanded.has(rel.fieldName)}
                onChange={() => onToggle(rel.fieldName)}
              />
              <span>{rel.fieldName}</span>
              <span className="text-gray-400 text-xs ml-auto">{rel.toTable}</span>
            </label>
          ))}
        </div>
      )}
    </div>
  );
}

function RowDetail({
  row,
  columns,
  expandColumns,
  isWritable,
  onClose,
  onEdit,
  onDelete,
}: {
  row: Record<string, unknown>;
  columns: { name: string; type: string }[];
  expandColumns: { relation: Relationship; label: string }[];
  isWritable: boolean;
  onClose: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/20" onClick={onClose} />
      <div className="relative w-full max-w-lg bg-white shadow-lg overflow-y-auto">
        <div className="px-6 py-4 border-b flex items-center justify-between sticky top-0 bg-white">
          <h2 className="font-semibold">Row Detail</h2>
          <div className="flex items-center gap-1">
            {isWritable && (
              <>
                <button
                  onClick={onEdit}
                  className="p-1.5 hover:bg-gray-100 rounded text-gray-500 hover:text-gray-700"
                  title="Edit"
                >
                  <Pencil className="w-4 h-4" />
                </button>
                <button
                  onClick={onDelete}
                  className="p-1.5 hover:bg-red-50 rounded text-gray-500 hover:text-red-600"
                  title="Delete"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </>
            )}
            <button
              onClick={onClose}
              className="p-1.5 hover:bg-gray-100 rounded"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>
        <div className="p-6 space-y-3">
          {columns.map((col) => (
            <div key={col.name}>
              <label className="text-xs font-medium text-gray-500 block mb-0.5">
                {col.name}{" "}
                <span className="text-gray-300 font-normal">{col.type}</span>
              </label>
              <div className="text-sm bg-gray-50 rounded px-3 py-2 font-mono break-all">
                {row[col.name] === null || row[col.name] === undefined ? (
                  <span className="text-gray-300 italic">null</span>
                ) : typeof row[col.name] === "object" ? (
                  JSON.stringify(row[col.name], null, 2)
                ) : (
                  String(row[col.name])
                )}
              </div>
            </div>
          ))}
          {expandColumns.length > 0 && (
            <>
              <div className="border-t pt-3 mt-3">
                <h3 className="text-xs font-semibold text-purple-600 uppercase tracking-wide mb-2">
                  Expanded Relations
                </h3>
              </div>
              {expandColumns.map((ec) => {
                const expandData = row.expand as Record<string, unknown> | undefined;
                const related = expandData?.[ec.label] as Record<string, unknown> | undefined;
                return (
                  <div key={ec.label}>
                    <label className="text-xs font-medium text-purple-500 block mb-0.5">
                      {ec.label}{" "}
                      <span className="text-gray-300 font-normal">→ {ec.relation.toTable}</span>
                    </label>
                    <div className="text-sm bg-purple-50 rounded px-3 py-2 font-mono break-all">
                      {related ? (
                        JSON.stringify(related, null, 2)
                      ) : (
                        <span className="text-gray-300 italic">null</span>
                      )}
                    </div>
                  </div>
                );
              })}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function ExportMenu({ onExport }: { onExport: (format: "csv" | "json") => void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open]);

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen((v) => !v)}
        className="px-3 py-1 text-xs bg-gray-200 hover:bg-gray-300 rounded font-medium inline-flex items-center gap-1"
        aria-label="Export"
      >
        <Download className="w-3.5 h-3.5" />
        Export
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 bg-white border rounded shadow-lg z-10 py-1 min-w-[100px]">
          <button
            onClick={() => { onExport("csv"); setOpen(false); }}
            className="w-full text-left px-3 py-1.5 text-sm hover:bg-gray-100"
          >
            CSV
          </button>
          <button
            onClick={() => { onExport("json"); setOpen(false); }}
            className="w-full text-left px-3 py-1.5 text-sm hover:bg-gray-100"
          >
            JSON
          </button>
        </div>
      )}
    </div>
  );
}

function DeleteConfirm({
  row,
  primaryKey,
  tableName,
  onConfirm,
  onCancel,
}: {
  row: Record<string, unknown>;
  primaryKey: string[];
  tableName: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const confirmRef = useRef<() => Promise<void>>();

  const pkDisplay = primaryKey.map((k) => `${k}=${row[k]}`).join(", ");

  const handleConfirm = async () => {
    setDeleting(true);
    setError(null);
    try {
      await onConfirm();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to delete");
      setDeleting(false);
    }
  };

  confirmRef.current = handleConfirm;

  // Keyboard: Enter confirms, Cmd+Delete/Backspace or Escape cancels
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Enter") {
        e.preventDefault();
        confirmRef.current?.();
      } else if (e.key === "Escape" || ((e.metaKey || e.ctrlKey) && (e.key === "Delete" || e.key === "Backspace"))) {
        e.preventDefault();
        onCancel();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onCancel]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/20" onClick={onCancel} />
      <div className="relative bg-white rounded-lg shadow-lg p-6 max-w-sm w-full mx-4">
        <h3 className="font-semibold text-gray-900 mb-2">Delete record?</h3>
        <p className="text-sm text-gray-600 mb-1">
          This will permanently delete the record from <strong>{tableName}</strong>.
        </p>
        <p className="text-xs text-gray-400 font-mono mb-4">{pkDisplay}</p>

        {error && (
          <div className="bg-red-50 border border-red-200 text-red-700 rounded px-3 py-2 text-sm mb-4">
            {error}
          </div>
        )}

        <div className="flex gap-2 justify-end">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-100 rounded"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            disabled={deleting}
            className="px-4 py-2 text-sm font-medium bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
          >
            {deleting ? "Deleting..." : "Delete"}
          </button>
        </div>
      </div>
    </div>
  );
}

function BatchDeleteConfirm({
  count,
  tableName,
  onConfirm,
  onCancel,
}: {
  count: number;
  tableName: string;
  onConfirm: () => Promise<void>;
  onCancel: () => void;
}) {
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const confirmRef = useRef<() => Promise<void>>();

  const handleConfirm = async () => {
    setDeleting(true);
    setError(null);
    try {
      await onConfirm();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to delete");
      setDeleting(false);
    }
  };

  confirmRef.current = handleConfirm;

  // Keyboard: Enter confirms, Cmd+Delete/Backspace or Escape cancels
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Enter") {
        e.preventDefault();
        confirmRef.current?.();
      } else if (e.key === "Escape" || ((e.metaKey || e.ctrlKey) && (e.key === "Delete" || e.key === "Backspace"))) {
        e.preventDefault();
        onCancel();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onCancel]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/20" onClick={onCancel} />
      <div className="relative bg-white rounded-lg shadow-lg p-6 max-w-sm w-full mx-4">
        <h3 className="font-semibold text-gray-900 mb-2">Delete {count} records?</h3>
        <p className="text-sm text-gray-600 mb-4">
          This will permanently delete {count} record{count !== 1 ? "s" : ""} from{" "}
          <strong>{tableName}</strong>. This action cannot be undone.
        </p>

        {error && (
          <div className="bg-red-50 border border-red-200 text-red-700 rounded px-3 py-2 text-sm mb-4">
            {error}
          </div>
        )}

        <div className="flex gap-2 justify-end">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-100 rounded"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            disabled={deleting}
            className="px-4 py-2 text-sm font-medium bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
          >
            {deleting ? "Deleting..." : `Delete ${count}`}
          </button>
        </div>
      </div>
    </div>
  );
}
