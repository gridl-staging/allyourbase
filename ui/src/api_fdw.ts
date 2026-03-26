import { request, requestNoBody } from "./api_client";
import type {
  ForeignServer,
  ForeignTable,
  CreateServerRequest,
  ImportTablesRequest,
} from "./types/fdw";

export function listServers(): Promise<ForeignServer[]> {
  return request<{ servers: ForeignServer[] }>("/api/admin/fdw/servers").then(
    (r) => r.servers,
  );
}

export function createServer(req: CreateServerRequest): Promise<void> {
  return requestNoBody("/api/admin/fdw/servers", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function dropServer(name: string, cascade?: boolean): Promise<void> {
  const params = cascade ? "?cascade=true" : "";
  return requestNoBody(
    `/api/admin/fdw/servers/${encodeURIComponent(name)}${params}`,
    { method: "DELETE" },
  );
}

export function listTables(): Promise<ForeignTable[]> {
  return request<{ tables: ForeignTable[] }>("/api/admin/fdw/tables").then(
    (r) => r.tables,
  );
}

export function importTables(
  serverName: string,
  req: ImportTablesRequest,
): Promise<ForeignTable[]> {
  return request<{ tables: ForeignTable[] }>(
    `/api/admin/fdw/servers/${encodeURIComponent(serverName)}/import`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  ).then((r) => r.tables);
}

export function dropTable(schema: string, table: string): Promise<void> {
  return requestNoBody(
    `/api/admin/fdw/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}`,
    { method: "DELETE" },
  );
}
