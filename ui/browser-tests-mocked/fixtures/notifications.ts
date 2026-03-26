/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/notifications.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface NotificationsMockOptions {
  sendResponder?: (body: Record<string, unknown>) => MockApiResponse;
}

/**
 * TODO: Document mockAdminNotificationApis.
 */
export async function mockAdminNotificationApis(
  page: Page,
  options: NotificationsMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (method === "POST" && path === "/api/admin/notifications") {
      const body = request.postDataJSON() as Record<string, unknown>;
      if (options.sendResponder) {
        const resp = options.sendResponder(body);
        return json(route, resp.status, resp.body);
      }
      return json(route, 201, {
        id: "notif-001",
        userId: String(body.user_id ?? ""),
        title: String(body.title ?? ""),
        body: String(body.body ?? ""),
        channel: String(body.channel ?? ""),
        createdAt: "2026-02-28T12:00:00Z",
      });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}
