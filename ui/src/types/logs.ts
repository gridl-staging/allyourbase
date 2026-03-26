export type AdminLogLevel = "debug" | "info" | "warn" | "error" | "unknown";

export interface AdminLogEntry {
  id: string;
  time: string;
  parsedTimeMs: number | null;
  level: AdminLogLevel;
  levelLabel: string;
  message: string;
  attrs: Record<string, unknown>;
  attrsText: string;
  searchText: string;
}

export interface AdminLogsResult {
  entries: AdminLogEntry[];
  message?: string;
  bufferingEnabled: boolean;
}
