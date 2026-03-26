/**
 * @module API client for managing database schemas and Row Level Security policies.
 */
import type {
  RlsPolicy,
  RlsTableStatus,
  SchemaCache,
} from "./types";
import {
  request,
  requestNoBody,
} from "./api_client";

export async function getSchema(): Promise<SchemaCache> {
  return request("/api/schema");
}

export async function getSchemaDesignerSchema(): Promise<SchemaCache> {
  return request("/api/schema");
}

// --- RLS Policies ---

export type {
  RlsPolicy,
  RlsTableStatus,
} from "./types";

export async function listRlsPolicies(table?: string): Promise<RlsPolicy[]> {
  const path = table ? `/api/admin/rls/${encodeURIComponent(table)}` : "/api/admin/rls";
  return request(path);
}

export async function getRlsStatus(table: string): Promise<RlsTableStatus> {
  return request(`/api/admin/rls/${encodeURIComponent(table)}/status`);
}

/**
 * Creates a new Row Level Security policy for a database table. @param data - Policy configuration object with required fields table, name, command, and optional fields schema, permissive, roles, using, withCheck. @returns Promise resolving to an object with a server message confirming the policy was created.
 */
export async function createRlsPolicy(data: {
  table: string;
  schema?: string;
  name: string;
  command: string;
  permissive?: boolean;
  roles?: string[];
  using?: string;
  withCheck?: string;
}): Promise<{ message: string }> {
  return request("/api/admin/rls", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteRlsPolicy(
  table: string,
  policy: string,
): Promise<void> {
  return requestNoBody(
    `/api/admin/rls/${encodeURIComponent(table)}/${encodeURIComponent(policy)}`,
    { method: "DELETE" },
  );
}

export async function enableRls(table: string): Promise<{ message: string }> {
  return request(`/api/admin/rls/${encodeURIComponent(table)}/enable`, {
    method: "POST",
  });
}

export async function disableRls(table: string): Promise<{ message: string }> {
  return request(`/api/admin/rls/${encodeURIComponent(table)}/disable`, {
    method: "POST",
  });
}
