import type { Table } from "../types";
import {
  Play,
  Clock,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { cn } from "../lib/utils";
import {
  METHOD_COLORS,
  METHODS,
  type Method,
} from "./api-explorer-helpers";

interface ApiExplorerRequestProps {
  tables: Table[];
  method: Method;
  path: string;
  body: string;
  loading: boolean;
  historyCount: number;
  showParams: boolean;
  filter: string;
  sort: string;
  page: string;
  perPage: string;
  fields: string;
  expand: string;
  search: string;
  showBodyEditor: boolean;
  selectedTable: Table | null;
  onMethodChange: (method: Method) => void;
  onPathChange: (path: string) => void;
  onBodyChange: (body: string) => void;
  onExecute: () => void;
  onToggleHistory: () => void;
  onToggleParams: () => void;
  onCollectionSelect: (table: Table) => void;
  onFilterChange: (value: string) => void;
  onSortChange: (value: string) => void;
  onPageChange: (value: string) => void;
  onPerPageChange: (value: string) => void;
  onFieldsChange: (value: string) => void;
  onExpandChange: (value: string) => void;
  onSearchChange: (value: string) => void;
}

export function ApiExplorerRequest({
  tables,
  method,
  path,
  body,
  loading,
  historyCount,
  showParams,
  filter,
  sort,
  page,
  perPage,
  fields,
  expand,
  search,
  showBodyEditor,
  selectedTable,
  onMethodChange,
  onPathChange,
  onBodyChange,
  onExecute,
  onToggleHistory,
  onToggleParams,
  onCollectionSelect,
  onFilterChange,
  onSortChange,
  onPageChange,
  onPerPageChange,
  onFieldsChange,
  onExpandChange,
  onSearchChange,
}: ApiExplorerRequestProps) {
  return (
    <div className="border-b px-6 py-4">
      <div className="flex items-center gap-3 mb-3">
        <h1 className="font-semibold text-lg">API Explorer</h1>
        <button
          onClick={onToggleHistory}
          className="ml-auto text-xs text-gray-500 dark:text-gray-200 hover:text-gray-700 dark:hover:text-gray-200 flex items-center gap-1"
        >
          <Clock className="w-3 h-3" />
          History ({historyCount})
        </button>
      </div>

      <div className="flex gap-2 items-stretch">
        <select
          aria-label="HTTP method"
          value={method}
          onChange={(event) => onMethodChange(event.target.value as Method)}
          className={cn(
            "px-3 py-2 rounded-lg font-mono text-sm font-bold border",
            METHOD_COLORS[method],
          )}
        >
          {METHODS.map((methodOption) => (
            <option key={methodOption} value={methodOption}>
              {methodOption}
            </option>
          ))}
        </select>

        <input
          aria-label="Request path"
          type="text"
          value={path}
          onChange={(event) => onPathChange(event.target.value)}
          className="flex-1 px-3 py-2 border rounded-lg font-mono text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          placeholder="/api/collections/table_name"
        />

        <button
          onClick={onExecute}
          disabled={loading || !path.trim()}
          className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
        >
          <Play className="w-3.5 h-3.5" />
          {loading ? "Sending..." : "Send"}
        </button>
      </div>

      <div className="mt-2 flex flex-wrap gap-1">
        {tables.slice(0, 12).map((table) => {
          const label = table.schema !== "public" ? `${table.schema}.${table.name}` : table.name;
          return (
            <button
              key={`${table.schema}.${table.name}`}
              onClick={() => onCollectionSelect(table)}
              className="px-2 py-0.5 text-xs bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-700 rounded text-gray-600 dark:text-gray-300"
            >
              {label}
            </button>
          );
        })}
        {tables.length > 12 && (
          <span className="px-2 py-0.5 text-xs text-gray-400 dark:text-gray-500">
            +{tables.length - 12} more
          </span>
        )}
      </div>

      <button
        onClick={onToggleParams}
        className="mt-2 text-xs text-gray-500 dark:text-gray-200 hover:text-gray-700 dark:hover:text-gray-200 flex items-center gap-1"
      >
        {showParams ? (
          <ChevronDown className="w-3 h-3" />
        ) : (
          <ChevronRight className="w-3 h-3" />
        )}
        Query Parameters
      </button>

      {showParams && (
        <div className="mt-2 grid grid-cols-2 gap-2">
          <div>
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">filter</label>
            <input
              aria-label="filter"
              type="text"
              value={filter}
              onChange={(event) => onFilterChange(event.target.value)}
              className="w-full px-2 py-1 text-xs border rounded font-mono"
              placeholder="status='active' AND age>21"
            />
          </div>
          <div>
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">sort</label>
            <input
              aria-label="sort"
              type="text"
              value={sort}
              onChange={(event) => onSortChange(event.target.value)}
              className="w-full px-2 py-1 text-xs border rounded font-mono"
              placeholder="-created_at,+title"
            />
          </div>
          <div>
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">page</label>
            <input
              aria-label="page"
              type="text"
              value={page}
              onChange={(event) => onPageChange(event.target.value)}
              className="w-full px-2 py-1 text-xs border rounded font-mono"
              placeholder="1"
            />
          </div>
          <div>
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">perPage</label>
            <input
              aria-label="perPage"
              type="text"
              value={perPage}
              onChange={(event) => onPerPageChange(event.target.value)}
              className="w-full px-2 py-1 text-xs border rounded font-mono"
              placeholder="20"
            />
          </div>
          <div>
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">fields</label>
            <input
              aria-label="fields"
              type="text"
              value={fields}
              onChange={(event) => onFieldsChange(event.target.value)}
              className="w-full px-2 py-1 text-xs border rounded font-mono"
              placeholder="id,name,email"
            />
          </div>
          <div>
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">expand</label>
            <input
              aria-label="expand"
              type="text"
              value={expand}
              onChange={(event) => onExpandChange(event.target.value)}
              className="w-full px-2 py-1 text-xs border rounded font-mono"
              placeholder={
                selectedTable?.foreignKeys?.map((fk) => fk.referencedTable).join(",") ||
                "author,category"
              }
            />
          </div>
          <div className="col-span-2">
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">search</label>
            <input
              aria-label="search"
              type="text"
              value={search}
              onChange={(event) => onSearchChange(event.target.value)}
              className="w-full px-2 py-1 text-xs border rounded font-mono"
              placeholder="full text search query"
            />
          </div>
        </div>
      )}

      {showBodyEditor && (
        <div className="mt-3">
          <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">
            Request Body (JSON)
          </label>
          <textarea
            aria-label="Request body"
            value={body}
            onChange={(event) => onBodyChange(event.target.value)}
            className="w-full h-24 px-3 py-2 font-mono text-xs border rounded-lg resize-y bg-gray-50 dark:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder='{"key": "value"}'
            spellCheck={false}
          />
        </div>
      )}

      <div className="mt-2 text-xs text-gray-400 dark:text-gray-500">
        {navigator.platform?.includes("Mac") ? "\u2318" : "Ctrl"}+Enter to send
      </div>
    </div>
  );
}
