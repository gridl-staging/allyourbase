export interface ApiExplorerRequest {
  method: string;
  path: string;
  body?: string;
}

export interface ApiExplorerResponse {
  status: number;
  statusText: string;
  headers: Record<string, string>;
  body: string;
  durationMs: number;
}

export interface ApiExplorerHistoryEntry {
  method: string;
  path: string;
  body?: string;
  status: number;
  durationMs: number;
  timestamp: string;
}
