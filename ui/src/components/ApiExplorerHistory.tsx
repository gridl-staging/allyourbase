import type { ApiExplorerHistoryEntry } from "../types";
import { Clock, Trash2 } from "lucide-react";
import { cn } from "../lib/utils";
import { METHOD_COLORS, statusColor, type Method } from "./api-explorer-helpers";

interface ApiExplorerHistoryProps {
  history: ApiExplorerHistoryEntry[];
  onClear: () => void;
  onSelect: (entry: ApiExplorerHistoryEntry) => void;
}

export function ApiExplorerHistory({ history, onClear, onSelect }: ApiExplorerHistoryProps) {
  if (history.length === 0) {
    return null;
  }

  return (
    <div className="border-b bg-gray-50 dark:bg-gray-800 px-6 py-3 max-h-48 overflow-y-auto">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-medium text-gray-500 dark:text-gray-400">
          Recent Requests
        </span>
        <button
          onClick={onClear}
          className="text-xs text-gray-400 dark:text-gray-500 hover:text-red-500 flex items-center gap-1"
        >
          <Trash2 className="w-3 h-3" />
          Clear
        </button>
      </div>
      {history.map((entry, index) => (
        <button
          key={index}
          onClick={() => onSelect(entry)}
          className="w-full text-left px-2 py-1 text-xs hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700 rounded flex items-center gap-2 font-mono"
        >
          <span
            className={cn(
              "px-1.5 py-0.5 rounded text-[10px] font-bold",
              METHOD_COLORS[entry.method as Method] || "bg-gray-100 dark:bg-gray-700",
            )}
          >
            {entry.method}
          </span>
          <span className="truncate flex-1">{entry.path}</span>
          <span className={cn("shrink-0", statusColor(entry.status))}>{entry.status}</span>
          <span className="text-gray-400 dark:text-gray-500 shrink-0">
            <Clock className="w-3 h-3 inline mr-1" />
            {entry.durationMs}ms
          </span>
        </button>
      ))}
    </div>
  );
}
