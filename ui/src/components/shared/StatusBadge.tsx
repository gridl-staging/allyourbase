import { cn } from "../../lib/utils";

type BadgeVariant = "success" | "error" | "warning" | "info" | "default";

const VARIANT_CLASSES: Record<BadgeVariant, string> = {
  success: "bg-green-100 text-green-700",
  error: "bg-red-100 text-red-700",
  warning: "bg-yellow-100 text-yellow-700",
  info: "bg-blue-100 text-blue-700",
  default: "bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200",
};

const DEFAULT_VARIANT_MAP: Record<string, BadgeVariant> = {
  completed: "success",
  success: "success",
  active: "success",
  healthy: "success",
  failed: "error",
  error: "error",
  revoked: "error",
  running: "warning",
  pending: "warning",
  in_progress: "warning",
  queued: "info",
  canceled: "default",
  unknown: "default",
};

interface StatusBadgeProps {
  status: string;
  variantMap?: Record<string, BadgeVariant>;
  className?: string;
}

export function StatusBadge({
  status,
  variantMap,
  className,
}: StatusBadgeProps) {
  const merged = variantMap
    ? { ...DEFAULT_VARIANT_MAP, ...variantMap }
    : DEFAULT_VARIANT_MAP;
  const variant = merged[status] ?? "default";

  return (
    <span
      className={cn(
        "inline-block px-2 py-0.5 rounded-full text-[10px] font-medium",
        VARIANT_CLASSES[variant],
        className,
      )}
    >
      {status}
    </span>
  );
}
