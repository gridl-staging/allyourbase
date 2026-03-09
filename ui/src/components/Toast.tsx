import { useState, useEffect, useCallback } from "react";
import { X, CheckCircle2, AlertCircle } from "lucide-react";
import { cn } from "../lib/utils";

export interface ToastMessage {
  id: number;
  type: "success" | "error";
  text: string;
}

let nextId = 0;

export function useToast() {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const addToast = useCallback((type: "success" | "error", text: string) => {
    const id = nextId++;
    setToasts((prev) => [...prev, { id, type, text }]);
  }, []);

  const removeToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  return { toasts, addToast, removeToast };
}

interface ToastContainerProps {
  toasts: ToastMessage[];
  onRemove: (id: number) => void;
}

export function ToastContainer({ toasts, onRemove }: ToastContainerProps) {
  if (toasts.length === 0) return null;

  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
      {toasts.map((toast) => (
        <ToastItem key={toast.id} toast={toast} onRemove={onRemove} />
      ))}
    </div>
  );
}

function ToastItem({
  toast,
  onRemove,
}: {
  toast: ToastMessage;
  onRemove: (id: number) => void;
}) {
  useEffect(() => {
    const timer = setTimeout(() => onRemove(toast.id), 4000);
    return () => clearTimeout(timer);
  }, [toast.id, onRemove]);

  return (
    <div
      data-testid="toast"
      className={cn(
        "flex items-center gap-2 px-4 py-2.5 rounded-lg shadow-lg text-sm min-w-[280px] max-w-[400px]",
        toast.type === "success"
          ? "bg-green-50 text-green-800 border border-green-200"
          : "bg-red-50 text-red-800 border border-red-200",
      )}
    >
      {toast.type === "success" ? (
        <CheckCircle2 className="w-4 h-4 shrink-0" />
      ) : (
        <AlertCircle className="w-4 h-4 shrink-0" />
      )}
      <span className="flex-1">{toast.text}</span>
      <button
        onClick={() => onRemove(toast.id)}
        className="shrink-0 p-0.5 hover:bg-black/5 rounded"
      >
        <X className="w-3.5 h-3.5" />
      </button>
    </div>
  );
}
