/**
 * @module Admin API client for push notification management, including device registration, delivery tracking, and sending operations.
 */
import type {
  PushDelivery,
  PushDeviceToken,
  PushDeviceListResponse,
  PushDeliveryListResponse,
  PushSendResponse,
} from "./types";
import {
  request,
  requestNoBody,
} from "./api_client";

export async function listAdminPushDevices(params: {
  app_id?: string;
  user_id?: string;
  include_inactive?: boolean;
} = {}): Promise<PushDeviceListResponse> {
  const qs = new URLSearchParams();
  if (params.app_id) qs.set("app_id", params.app_id);
  if (params.user_id) qs.set("user_id", params.user_id);
  if (params.include_inactive) qs.set("include_inactive", "true");
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/push/devices${suffix}`);
}

export async function registerAdminPushDevice(data: {
  app_id: string;
  user_id: string;
  provider: string;
  platform: string;
  token: string;
  device_name?: string;
}): Promise<PushDeviceToken> {
  return request("/api/admin/push/devices", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function revokeAdminPushDevice(id: string): Promise<void> {
  return requestNoBody(`/api/admin/push/devices/${id}`, {
    method: "DELETE",
  });
}

/**
 * Fetches paginated push delivery records, optionally filtered by app, user, or status.
 * @param params - Optional filters (app_id, user_id, status) and pagination (limit, offset).
 * @returns The push delivery list with pagination metadata.
 */
export async function listAdminPushDeliveries(params: {
  app_id?: string;
  user_id?: string;
  status?: string;
  limit?: number;
  offset?: number;
} = {}): Promise<PushDeliveryListResponse> {
  const qs = new URLSearchParams();
  if (params.app_id) qs.set("app_id", params.app_id);
  if (params.user_id) qs.set("user_id", params.user_id);
  if (params.status) qs.set("status", params.status);
  if (params.limit) qs.set("limit", String(params.limit));
  if (params.offset) qs.set("offset", String(params.offset));
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/admin/push/deliveries${suffix}`);
}

export async function getAdminPushDelivery(id: string): Promise<PushDelivery> {
  return request(`/api/admin/push/deliveries/${id}`);
}

export async function adminSendPush(data: {
  app_id: string;
  user_id: string;
  title: string;
  body: string;
  data?: Record<string, string>;
}): Promise<PushSendResponse> {
  return request("/api/admin/push/send", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function adminSendPushToToken(data: {
  token_id: string;
  title: string;
  body: string;
  data?: Record<string, string>;
}): Promise<PushDelivery> {
  return request("/api/admin/push/send-to-token", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}
