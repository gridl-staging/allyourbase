import { useState, useEffect, useRef } from "react";
import type { Relationship } from "../types";
import {
  FileSearch,
  Search,
  X,
  Plus,
  Trash2,
  Download,
  Link,
} from "lucide-react";

interface TableBrowserToolbarProps {
  search: string;
  setSearch: (v: string) => void;
  handleSearchSubmit: () => void;
  setAppliedSearch: (v: string) => void;
  filter: string;
  setFilter: (v: string) => void;
  handleFilterSubmit: () => void;
  setAppliedFilter: (v: string) => void;
  setPage: (fn: (p: number) => number) => void;
  expandableRelations: Relationship[];
  expandedRelations: Set<string>;
  toggleExpand: (fieldName: string) => void;
  hasData: boolean;
  hasItems: boolean;
  onExport: (format: "csv" | "json") => void;
  selectedCount: number;
  onBatchDelete: () => void;
  isWritable: boolean;
  onCreateNew: () => void;
}

export function TableBrowserToolbar({
  search,
  setSearch,
  handleSearchSubmit,
  setAppliedSearch,
  filter,
  setFilter,
  handleFilterSubmit,
  setAppliedFilter,
  setPage,
  expandableRelations,
  expandedRelations,
  toggleExpand,
  hasData,
  hasItems,
  onExport,
  selectedCount,
  onBatchDelete,
  isWritable,
  onCreateNew,
}: TableBrowserToolbarProps) {
  return (
    <div className="px-4 py-2 border-b border-gray-200 dark:border-gray-700 flex items-center gap-2 bg-gray-50 dark:bg-gray-900">
      <FileSearch className="w-4 h-4 text-gray-400 dark:text-gray-500" />
      <input
        type="text"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && handleSearchSubmit()}
        placeholder="Search..."
        className="w-40 bg-transparent text-sm outline-none placeholder-gray-400 dark:placeholder-gray-500"
        aria-label="Full-text search"
      />
      {search && (
        <button
          onClick={() => {
            setSearch("");
            setAppliedSearch("");
            setPage(() => 1);
          }}
          className="text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300"
          aria-label="Clear search"
        >
          <X className="w-4 h-4" />
        </button>
      )}
      <div className="w-px h-5 bg-gray-300 dark:bg-gray-700" />
      <Search className="w-4 h-4 text-gray-400 dark:text-gray-500" />
      <input
        type="text"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && handleFilterSubmit()}
        placeholder="Filter... e.g. status='active' && age>25"
        className="flex-1 bg-transparent text-sm outline-none placeholder-gray-400 dark:placeholder-gray-500"
      />
      {filter && (
        <button
          onClick={() => {
            setFilter("");
            setAppliedFilter("");
            setPage(() => 1);
          }}
          className="text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300"
        >
          <X className="w-4 h-4" />
        </button>
      )}
      <button
        onClick={() => { handleFilterSubmit(); handleSearchSubmit(); }}
        className="px-3 py-1 text-xs bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-100 hover:bg-gray-300 dark:hover:bg-gray-600 rounded font-medium"
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
      {hasData && hasItems && (
        <ExportMenu onExport={onExport} />
      )}
      {selectedCount > 0 && (
        <button
          onClick={onBatchDelete}
          className="px-3 py-1 text-xs bg-red-600 text-white hover:bg-red-700 rounded font-medium inline-flex items-center gap-1"
          aria-label="Delete selected"
        >
          <Trash2 className="w-3.5 h-3.5" />
          Delete ({selectedCount})
        </button>
      )}
      {isWritable && (
        <button
          onClick={onCreateNew}
          className="ml-2 px-3 py-1 text-xs bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 hover:bg-gray-800 dark:hover:bg-gray-200 rounded font-medium inline-flex items-center gap-1"
        >
          <Plus className="w-3.5 h-3.5" />
          New Row
        </button>
      )}
    </div>
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
            ? "bg-purple-100 dark:bg-purple-900/40 text-purple-700 dark:text-purple-300 hover:bg-purple-200 dark:hover:bg-purple-900/60"
            : "bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-100 hover:bg-gray-300 dark:hover:bg-gray-600"
        }`}
        aria-label="Expand"
      >
        <Link className="w-3.5 h-3.5" />
        Expand{expanded.size > 0 ? ` (${expanded.size})` : ""}
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded shadow-lg z-10 py-1 min-w-[180px]">
          {relations.map((rel) => (
            <label
              key={rel.fieldName}
              className="flex items-center gap-2 px-3 py-1.5 text-sm text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 cursor-pointer"
            >
              <input
                type="checkbox"
                checked={expanded.has(rel.fieldName)}
                onChange={() => onToggle(rel.fieldName)}
              />
              <span>{rel.fieldName}</span>
              <span className="text-gray-400 dark:text-gray-500 text-xs ml-auto">{rel.toTable}</span>
            </label>
          ))}
        </div>
      )}
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
        className="px-3 py-1 text-xs bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-100 hover:bg-gray-300 dark:hover:bg-gray-600 rounded font-medium inline-flex items-center gap-1"
        aria-label="Export"
      >
        <Download className="w-3.5 h-3.5" />
        Export
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded shadow-lg z-10 py-1 min-w-[100px]">
          <button
            onClick={() => { onExport("csv"); setOpen(false); }}
            className="w-full text-left px-3 py-1.5 text-sm text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800"
          >
            CSV
          </button>
          <button
            onClick={() => { onExport("json"); setOpen(false); }}
            className="w-full text-left px-3 py-1.5 text-sm text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800"
          >
            JSON
          </button>
        </div>
      )}
    </div>
  );
}
