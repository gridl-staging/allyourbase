import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
} from "react";
import type { ReactNode } from "react";
import {
  X,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Info,
} from "lucide-react";
import { cn } from "../lib/utils";

export type ToastType = "success" | "error" | "warning" | "info";

const DEFAULT_DURATION = 4000;

interface Toast {
  id: number;
  type: ToastType;
  text: string;
  duration: number;
}

interface ToastContextValue {
  addToast: (type: ToastType, text: string, duration?: number) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

let nextId = 0;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const addToast = useCallback(
    (type: ToastType, text: string, duration?: number) => {
      const id = nextId++;
      setToasts((prev) => [
        ...prev,
        { id, type, text, duration: duration ?? DEFAULT_DURATION },
      ]);
    },
    [],
  );

  const removeToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  return (
    <ToastContext.Provider value={{ addToast }}>
      {children}
      {toasts.length > 0 && (
        <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
          {toasts.map((toast) => (
            <ToastItem key={toast.id} toast={toast} onRemove={removeToast} />
          ))}
        </div>
      )}
    </ToastContext.Provider>
  );
}

export function useAppToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error("useAppToast must be used within a ToastProvider");
  }
  return ctx;
}

const VARIANT_STYLES: Record<ToastType, string> = {
  success:
    "bg-green-50 text-green-800 border border-green-200 dark:bg-green-900/30 dark:text-green-300 dark:border-green-700",
  error:
    "bg-red-50 text-red-800 border border-red-200 dark:bg-red-900/30 dark:text-red-300 dark:border-red-700",
  warning:
    "bg-amber-50 text-amber-800 border border-amber-200 dark:bg-amber-900/30 dark:text-amber-300 dark:border-amber-700",
  info: "bg-blue-50 text-blue-800 border border-blue-200 dark:bg-blue-900/30 dark:text-blue-300 dark:border-blue-700",
};

const VARIANT_ICONS: Record<ToastType, typeof CheckCircle2> = {
  success: CheckCircle2,
  error: AlertCircle,
  warning: AlertTriangle,
  info: Info,
};

function ToastItem({
  toast,
  onRemove,
}: {
  toast: Toast;
  onRemove: (id: number) => void;
}) {
  useEffect(() => {
    const timer = setTimeout(() => onRemove(toast.id), toast.duration);
    return () => clearTimeout(timer);
  }, [toast.id, toast.duration, onRemove]);

  const Icon = VARIANT_ICONS[toast.type];

  return (
    <div
      data-testid="toast"
      className={cn(
        "flex items-center gap-2 px-4 py-2.5 rounded-lg shadow-lg text-sm min-w-[280px] max-w-[400px]",
        VARIANT_STYLES[toast.type],
      )}
    >
      <Icon className="w-4 h-4 shrink-0" />
      <span className="flex-1">{toast.text}</span>
      <button
        onClick={() => onRemove(toast.id)}
        className="shrink-0 p-0.5 hover:bg-black/5 dark:hover:bg-white/10 rounded"
      >
        <X className="w-3.5 h-3.5" />
      </button>
    </div>
  );
}
