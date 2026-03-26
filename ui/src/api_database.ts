/**
 * @module Database and RPC API client providing CRUD, batch, SQL, and remote procedure call functions.
 */
import type { ListResponse, SqlResult } from "./types";
import {
  request,
  requestNoBody,
  fetchAdmin,
  throwApiError,
} from "./api_client";

/**
 * Fetches rows from a table with optional pagination, sorting, filtering, and searching.
 * @param table - Table name to query.
 * @param params - Optional query parameters (page, perPage, sort, filter, search, expand).
 * @returns Promise resolving to a ListResponse.
 */
export async function getRows(
  table: string,
  params: {
    page?: number;
    perPage?: number;
    sort?: string;
    filter?: string;
    search?: string;
    expand?: string;
  } = {},
): Promise<ListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  if (params.sort) qs.set("sort", params.sort);
  if (params.filter) qs.set("filter", params.filter);
  if (params.search) qs.set("search", params.search);
  if (params.expand) qs.set("expand", params.expand);
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/collections/${table}${suffix}`);
}

export async function createRecord(
  table: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  return request(`/api/collections/${table}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function updateRecord(
  table: string,
  id: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  return request(`/api/collections/${table}/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function executeSQL(query: string): Promise<SqlResult> {
  return request("/api/admin/sql/", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query }),
  });
}

export async function deleteRecord(
  table: string,
  id: string,
): Promise<void> {
  return requestNoBody(`/api/collections/${table}/${id}`, {
    method: "DELETE",
  });
}

// --- Batch ---

export interface BatchOperation {
  method: "create" | "update" | "delete";
  id?: string;
  body?: Record<string, unknown>;
}

export interface BatchResult {
  index: number;
  status: number;
  body?: Record<string, unknown>;
}

export async function batchRecords(
  table: string,
  operations: BatchOperation[],
): Promise<BatchResult[]> {
  return request(`/api/collections/${table}/batch`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ operations }),
  });
}

// --- RPC ---

/**
 * Invokes a remote procedure call.
 * @param functionName - Name of the RPC function to invoke.
 * @param args - Optional arguments object passed to the function.
 * @returns Promise resolving to status code and parsed response data; null for 204 No Content.
 * @throws When the HTTP request fails.
 */
export async function callRpc(
  functionName: string,
  args: Record<string, unknown> = {},
): Promise<{ status: number; data: unknown }> {
  const res = await fetchAdmin(`/api/rpc/${functionName}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(args),
  });
  if (res.status === 204) {
    return { status: 204, data: null };
  }
  if (!res.ok) {
    await throwApiError(res);
  }
  const data = await res.json();
  return { status: res.status, data };
}
