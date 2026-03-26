import type { Column, ListResponse, Relationship } from "../types";
import {
  ChevronUp,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Pencil,
  Trash2,
  Link,
} from "lucide-react";

export interface ExpandColumn {
  relation: Relationship;
  label: string;
}

interface TableBrowserGridProps {
  data: ListResponse | null;
  loading: boolean;
  columns: Column[];
  expandColumns: ExpandColumn[];
  sort: string | null;
  toggleSort: (col: string) => void;
  showCheckboxes: boolean;
  isWritable: boolean;
  hasPK: boolean;
  selectedIds: Set<string>;
  toggleSelectAll: () => void;
  toggleSelect: (id: string) => void;
  pkId: (row: Record<string, unknown>) => string;
  onRowClick: (row: Record<string, unknown>) => void;
  onEdit: (row: Record<string, unknown>) => void;
  onDelete: (row: Record<string, unknown>) => void;
  page: number;
  setPage: React.Dispatch<React.SetStateAction<number>>;
}

export function TableBrowserGrid({
  data,
  loading,
  columns,
  expandColumns,
  sort,
  toggleSort,
  showCheckboxes,
  isWritable,
  hasPK,
  selectedIds,
  toggleSelectAll,
  toggleSelect,
  pkId,
  onRowClick,
  onEdit,
  onDelete,
  page,
  setPage,
}: TableBrowserGridProps) {
  const extraColCount =
    (showCheckboxes ? 1 : 0) +
    (isWritable && hasPK ? 1 : 0) +
    expandColumns.length;

  return (
    <>
      {/* Table */}
      <div className="flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-900 sticky top-0">
            <tr>
              {showCheckboxes && (
                <th className="px-2 py-2 border-b border-gray-200 dark:border-gray-700 w-10">
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
                  className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300 border-b border-gray-200 dark:border-gray-700 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-800 whitespace-nowrap select-none"
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
                  className="px-4 py-2 text-left font-medium text-purple-600 dark:text-purple-300 border-b border-gray-200 dark:border-gray-700 whitespace-nowrap select-none"
                >
                  <span className="inline-flex items-center gap-1">
                    <Link className="w-3 h-3" />
                    {ec.label}
                  </span>
                </th>
              ))}
              {isWritable && hasPK && (
                <th className="px-4 py-2 border-b border-gray-200 dark:border-gray-700 w-20" />
              )}
            </tr>
          </thead>
          <tbody>
            {loading && !data && (
              <tr>
                <td
                  colSpan={columns.length + extraColCount}
                  className="px-4 py-8 text-center text-gray-400 dark:text-gray-500"
                >
                  Loading...
                </td>
              </tr>
            )}
            {data?.items.length === 0 && (
              <tr>
                <td
                  colSpan={columns.length + extraColCount}
                  className="px-4 py-8 text-center text-gray-400 dark:text-gray-500"
                >
                  <div className="space-y-1">
                    <p className="text-sm font-medium text-gray-600 dark:text-gray-300">
                      No rows in this table yet
                    </p>
                    <p className="text-sm">
                      Insert data using the SQL editor, REST API, or SDK.
                    </p>
                  </div>
                </td>
              </tr>
            )}
            {data?.items.map((row, i) => {
              const rowId = hasPK ? pkId(row) : String(i);
              const isSelected = selectedIds.has(rowId);
              return (
                <tr
                  key={i}
                  onClick={() => onRowClick(row)}
                  className={`border-b border-gray-200 dark:border-gray-800 hover:bg-blue-50 dark:hover:bg-blue-950/40 cursor-pointer group ${isSelected ? "bg-blue-50 dark:bg-blue-950/40" : ""}`}
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
                            onEdit(row);
                          }}
                          className="p-1 hover:bg-gray-200 dark:hover:bg-gray-700 rounded text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
                          title="Edit"
                        >
                          <Pencil className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            onDelete(row);
                          }}
                          className="p-1 hover:bg-red-100 dark:hover:bg-red-900/40 rounded text-gray-500 dark:text-gray-400 hover:text-red-600 dark:hover:text-red-300"
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
        <div className="px-4 py-2 border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 flex items-center justify-between text-sm text-gray-500 dark:text-gray-400">
          <span>
            {data.totalItems} row{data.totalItems !== 1 ? "s" : ""}
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1}
              className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-700 disabled:opacity-30"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <span>
              {page} / {data.totalPages || 1}
            </span>
            <button
              onClick={() => setPage((p) => Math.min(data.totalPages, p + 1))}
              disabled={page >= data.totalPages}
              className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-700 disabled:opacity-30"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}
    </>
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
    return <span className="text-gray-300 dark:text-gray-500 italic">null</span>;
  if (typeof value === "boolean")
    return <span className={value ? "text-green-600 dark:text-green-400" : "text-gray-400 dark:text-gray-500"}>{String(value)}</span>;
  if (typeof value === "object")
    return (
      <span className="text-gray-500 dark:text-gray-400 font-mono text-xs">
        {JSON.stringify(value)}
      </span>
    );
  return <>{String(value)}</>;
}

function ExpandedCell({ row, fieldName }: { row: Record<string, unknown>; fieldName: string }) {
  const expandData = row.expand as Record<string, unknown> | undefined;
  if (!expandData || !expandData[fieldName]) {
    return <span className="text-gray-300 dark:text-gray-500 italic">null</span>;
  }
  const related = expandData[fieldName] as Record<string, unknown>;
  // Show a summary of the related record — first non-id text field, or the whole thing.
  const keys = Object.keys(related).filter((k) => k !== "id" && k !== "expand");
  const display = keys.length > 0 ? String(related[keys[0]]) : JSON.stringify(related);
  return (
    <span className="text-purple-600 dark:text-purple-300 text-xs" title={JSON.stringify(related, null, 2)}>
      {display}
    </span>
  );
}
