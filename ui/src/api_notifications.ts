import { request } from "./api_client";
import type { Notification, CreateNotificationRequest } from "./types/notifications";

export function createNotification(
  req: CreateNotificationRequest,
): Promise<Notification> {
  return request<Notification>("/api/admin/notifications", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}
