import { Loader2 } from "lucide-react";
import { cn } from "../../lib/utils";

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  message: string;
  confirmLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
  destructive?: boolean;
  loading?: boolean;
  confirmDisabled?: boolean;
  children?: React.ReactNode;
}

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel,
  onConfirm,
  onCancel,
  destructive = false,
  loading = false,
  confirmDisabled = false,
  children,
}: ConfirmDialogProps) {
  if (!open) return null;

  return (
    <div role="dialog" aria-label={title} className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
        <h3 className="font-semibold mb-2">{title}</h3>
        <p className={`text-sm text-gray-600 dark:text-gray-300 ${children ? "mb-3" : "mb-4"}`}>
          {message}
        </p>
        {children ? <div className="mb-4">{children}</div> : null}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            disabled={loading}
            className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded border disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={loading || confirmDisabled}
            className={cn(
              "px-3 py-1.5 text-sm text-white rounded disabled:opacity-50 inline-flex items-center gap-1.5",
              destructive
                ? "bg-red-600 hover:bg-red-700"
                : "bg-blue-600 hover:bg-blue-700",
            )}
          >
            {loading && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
