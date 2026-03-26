import { ApiError } from "../../api";

export function toPanelError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 401) return "Unauthorized. Please sign in again.";
    if (err.status >= 500) return "Server error while loading telemetry.";
    return err.message || "Request failed.";
  }
  if (err instanceof Error) {
    return err.message || "Network error.";
  }
  return "Unknown error.";
}
