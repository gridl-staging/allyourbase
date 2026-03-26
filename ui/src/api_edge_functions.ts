/**
 * @module API client functions for managing edge functions and their triggers. Provides operations to list, create, deploy, invoke, and manage database, cron, and storage triggers for edge functions.
 */
import type {
  EdgeFunctionResponse,
  EdgeFunctionLogEntry,
  EdgeFunctionDeployRequest,
  EdgeFunctionUpdateRequest,
  EdgeFunctionInvokeRequest,
  EdgeFunctionInvokeResponse,
  DBTriggerResponse,
  CreateDBTriggerRequest,
  CronTriggerResponse,
  CreateCronTriggerRequest,
  StorageTriggerResponse,
  CreateStorageTriggerRequest,
  ManualRunResponse,
} from "./types";
import {
  request,
  requestNoBody,
} from "./api_client";

// --- Edge Functions ---

export async function listEdgeFunctions(): Promise<EdgeFunctionResponse[]> {
  return request("/api/admin/functions");
}

export async function getEdgeFunction(id: string): Promise<EdgeFunctionResponse> {
  return request(`/api/admin/functions/${id}`);
}

export async function deployEdgeFunction(
  data: EdgeFunctionDeployRequest,
): Promise<EdgeFunctionResponse> {
  return request("/api/admin/functions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function updateEdgeFunction(
  id: string,
  data: EdgeFunctionUpdateRequest,
): Promise<EdgeFunctionResponse> {
  return request(`/api/admin/functions/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteEdgeFunction(id: string): Promise<void> {
  return requestNoBody(`/api/admin/functions/${id}`, {
    method: "DELETE",
  });
}

/**
 * Fetches execution logs for an edge function, with optional filtering and pagination.
 * @param id - The edge function identifier
 * @param params - Query parameters for filtering and pagination
 * @param params.page - Page number for pagination
 * @param params.perPage - Number of entries per page
 * @param params.status - Filter by execution status
 * @param params.trigger_type - Filter by trigger type that invoked the function
 * @param params.since - Start of date range (ISO 8601 format)
 * @param params.until - End of date range (ISO 8601 format)
 * @param params.limit - Maximum number of entries to return
 * @returns Array of log entries for the function
 */
export async function listEdgeFunctionLogs(
  id: string,
  params: {
    page?: number;
    perPage?: number;
    status?: "success" | "error";
    trigger_type?: string;
    since?: string;
    until?: string;
    limit?: number;
  } = {},
): Promise<EdgeFunctionLogEntry[]> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  if (params.status) qs.set("status", params.status);
  if (params.trigger_type) qs.set("trigger_type", params.trigger_type);
  if (params.since) qs.set("since", params.since);
  if (params.until) qs.set("until", params.until);
  if (params.limit) qs.set("limit", String(params.limit));
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/functions/${id}/logs${suffix}`);
}

export async function invokeEdgeFunction(
  id: string,
  data: EdgeFunctionInvokeRequest,
): Promise<EdgeFunctionInvokeResponse> {
  return request(`/api/admin/functions/${id}/invoke`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

// --- DB Triggers ---

export async function listDBTriggers(functionId: string): Promise<DBTriggerResponse[]> {
  return request(`/api/admin/functions/${functionId}/triggers/db`);
}

export async function createDBTrigger(functionId: string, data: CreateDBTriggerRequest): Promise<DBTriggerResponse> {
  return request(`/api/admin/functions/${functionId}/triggers/db`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteDBTrigger(functionId: string, triggerId: string): Promise<void> {
  await requestNoBody(`/api/admin/functions/${functionId}/triggers/db/${triggerId}`, { method: "DELETE" });
}

export async function enableDBTrigger(functionId: string, triggerId: string): Promise<DBTriggerResponse> {
  return request(`/api/admin/functions/${functionId}/triggers/db/${triggerId}/enable`, { method: "POST" });
}

export async function disableDBTrigger(functionId: string, triggerId: string): Promise<DBTriggerResponse> {
  return request(`/api/admin/functions/${functionId}/triggers/db/${triggerId}/disable`, { method: "POST" });
}

// --- Cron Triggers ---

export async function listCronTriggers(functionId: string): Promise<CronTriggerResponse[]> {
  return request(`/api/admin/functions/${functionId}/triggers/cron`);
}

export async function createCronTrigger(functionId: string, data: CreateCronTriggerRequest): Promise<CronTriggerResponse> {
  return request(`/api/admin/functions/${functionId}/triggers/cron`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteCronTrigger(functionId: string, triggerId: string): Promise<void> {
  await requestNoBody(`/api/admin/functions/${functionId}/triggers/cron/${triggerId}`, { method: "DELETE" });
}

export async function enableCronTrigger(functionId: string, triggerId: string): Promise<CronTriggerResponse> {
  return request(`/api/admin/functions/${functionId}/triggers/cron/${triggerId}/enable`, { method: "POST" });
}

export async function disableCronTrigger(functionId: string, triggerId: string): Promise<CronTriggerResponse> {
  return request(`/api/admin/functions/${functionId}/triggers/cron/${triggerId}/disable`, { method: "POST" });
}

export async function manualRunCronTrigger(functionId: string, triggerId: string): Promise<ManualRunResponse> {
  return request(`/api/admin/functions/${functionId}/triggers/cron/${triggerId}/run`, { method: "POST" });
}

// --- Storage Triggers ---

export async function listStorageTriggers(functionId: string): Promise<StorageTriggerResponse[]> {
  return request(`/api/admin/functions/${functionId}/triggers/storage`);
}

export async function createStorageTrigger(functionId: string, data: CreateStorageTriggerRequest): Promise<StorageTriggerResponse> {
  return request(`/api/admin/functions/${functionId}/triggers/storage`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteStorageTrigger(functionId: string, triggerId: string): Promise<void> {
  await requestNoBody(`/api/admin/functions/${functionId}/triggers/storage/${triggerId}`, { method: "DELETE" });
}

export async function enableStorageTrigger(functionId: string, triggerId: string): Promise<StorageTriggerResponse> {
  return request(`/api/admin/functions/${functionId}/triggers/storage/${triggerId}/enable`, { method: "POST" });
}

export async function disableStorageTrigger(functionId: string, triggerId: string): Promise<StorageTriggerResponse> {
  return request(`/api/admin/functions/${functionId}/triggers/storage/${triggerId}/disable`, { method: "POST" });
}
