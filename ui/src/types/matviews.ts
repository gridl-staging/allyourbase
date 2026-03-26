export type MatviewRefreshMode = "standard" | "concurrent";
export type MatviewRefreshStatus = "success" | "error";

export interface MatviewRegistration {
  id: string;
  schemaName: string;
  viewName: string;
  refreshMode: MatviewRefreshMode;
  lastRefreshAt: string | null;
  lastRefreshDurationMs: number | null;
  lastRefreshStatus: MatviewRefreshStatus | null;
  lastRefreshError: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface MatviewListResponse {
  items: MatviewRegistration[];
  count: number;
}

export interface MatviewRefreshResult {
  registration: MatviewRegistration;
  durationMs: number;
}
