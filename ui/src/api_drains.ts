import { request, requestNoBody } from "./api_client";
import type { DrainInfo, LogDrainConfig } from "./types/drains";

export function listDrains(): Promise<DrainInfo[]> {
  return request<DrainInfo[]>("/api/admin/logging/drains");
}

export function createDrain(config: LogDrainConfig): Promise<DrainInfo> {
  return request<DrainInfo>("/api/admin/logging/drains", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  });
}

export function deleteDrain(id: string): Promise<void> {
  return requestNoBody(`/api/admin/logging/drains/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}
