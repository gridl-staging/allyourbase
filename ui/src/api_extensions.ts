import { request, requestNoBody } from "./api_client";
import type { ExtensionListResponse } from "./types/extensions";

export function listExtensions(): Promise<ExtensionListResponse> {
  return request<ExtensionListResponse>("/api/admin/extensions");
}

export function enableExtension(name: string): Promise<void> {
  return requestNoBody("/api/admin/extensions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
}

export function disableExtension(name: string): Promise<void> {
  return requestNoBody(`/api/admin/extensions/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}
