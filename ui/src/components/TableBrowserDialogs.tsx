import { useState, useEffect, useRef } from "react";
import { Pencil, Trash2, X } from "lucide-react";
import type { ExpandColumn } from "./TableBrowserGrid";

export function RowDetail({
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
  expandColumns: ExpandColumn[];
  isWritable: boolean;
  onClose: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  // Close on Escape key — matches RecordForm, DeleteConfirm, and BatchDeleteConfirm behavior
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/20" onClick={onClose} />
      <div className="relative w-full max-w-lg bg-white dark:bg-gray-900 shadow-lg overflow-y-auto">
        <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between sticky top-0 bg-white dark:bg-gray-900">
          <h2 className="font-semibold">Row Detail</h2>
          <div className="flex items-center gap-1">
            {isWritable && (
              <>
                <button
                  onClick={onEdit}
                  className="p-1.5 hover:bg-gray-100 dark:hover:bg-gray-800 rounded text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
                  title="Edit"
                >
                  <Pencil className="w-4 h-4" />
                </button>
                <button
                  onClick={onDelete}
                  className="p-1.5 hover:bg-red-50 dark:hover:bg-red-900/40 rounded text-gray-500 dark:text-gray-400 hover:text-red-600 dark:hover:text-red-300"
                  title="Delete"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </>
            )}
            <button
              onClick={onClose}
              className="p-1.5 hover:bg-gray-100 dark:hover:bg-gray-800 rounded"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>
        <div className="p-6 space-y-3">
          {columns.map((col) => (
            <div key={col.name}>
              <label className="text-xs font-medium text-gray-500 dark:text-gray-400 block mb-0.5">
                {col.name}{" "}
                <span className="text-gray-300 dark:text-gray-500 font-normal">{col.type}</span>
              </label>
              <div className="text-sm bg-gray-50 dark:bg-gray-800 rounded px-3 py-2 font-mono break-all">
                {row[col.name] === null || row[col.name] === undefined ? (
                  <span className="text-gray-300 dark:text-gray-500 italic">null</span>
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
              <div className="border-t border-gray-200 dark:border-gray-700 pt-3 mt-3">
                <h3 className="text-xs font-semibold text-purple-600 dark:text-purple-300 uppercase tracking-wide mb-2">
                  Expanded Relations
                </h3>
              </div>
              {expandColumns.map((ec) => {
                const expandData = row.expand as Record<string, unknown> | undefined;
                const related = expandData?.[ec.label] as Record<string, unknown> | undefined;
                return (
                  <div key={ec.label}>
                    <label className="text-xs font-medium text-purple-500 dark:text-purple-300 block mb-0.5">
                      {ec.label}{" "}
                      <span className="text-gray-300 dark:text-gray-500 font-normal">→ {ec.relation.toTable}</span>
                    </label>
                    <div className="text-sm bg-purple-50 dark:bg-purple-900/30 rounded px-3 py-2 font-mono break-all">
                      {related ? (
                        JSON.stringify(related, null, 2)
                      ) : (
                        <span className="text-gray-300 dark:text-gray-500 italic">null</span>
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

export function DeleteConfirm({
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
      <div className="relative bg-white dark:bg-gray-900 rounded-lg shadow-lg p-6 max-w-sm w-full mx-4">
        <h3 className="font-semibold text-gray-900 dark:text-gray-100 mb-2">Delete record?</h3>
        <p className="text-sm text-gray-600 dark:text-gray-300 mb-1">
          This will permanently delete the record from <strong>{tableName}</strong>.
        </p>
        <p className="text-xs text-gray-400 dark:text-gray-500 font-mono mb-4">{pkDisplay}</p>

        {error && (
          <div className="bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900/60 text-red-700 dark:text-red-300 rounded px-3 py-2 text-sm mb-4">
            {error}
          </div>
        )}

        <div className="flex gap-2 justify-end">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm font-medium text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 rounded"
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

export function BatchDeleteConfirm({
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
      <div className="relative bg-white dark:bg-gray-900 rounded-lg shadow-lg p-6 max-w-sm w-full mx-4">
        <h3 className="font-semibold text-gray-900 dark:text-gray-100 mb-2">Delete {count} records?</h3>
        <p className="text-sm text-gray-600 dark:text-gray-300 mb-4">
          This will permanently delete {count} record{count !== 1 ? "s" : ""} from{" "}
          <strong>{tableName}</strong>. This action cannot be undone.
        </p>

        {error && (
          <div className="bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900/60 text-red-700 dark:text-red-300 rounded px-3 py-2 text-sm mb-4">
            {error}
          </div>
        )}

        <div className="flex gap-2 justify-end">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm font-medium text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 rounded"
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
