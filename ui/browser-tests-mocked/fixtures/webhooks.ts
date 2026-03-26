/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/webhooks.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface WebhooksMockOptions {
  listResponder?: () => MockApiResponse;
  deleteResponder?: (id: string) => MockApiResponse;
  testResponder?: (id: string) => MockApiResponse;
}

const defaultWebhooks = [
  {
    id: "wh-001",
    url: "https://hooks.example.com/events",
    events: ["create", "update"],
    tables: [],
    enabled: true,
    hasSecret: true,
    createdAt: "2026-02-28T00:00:00Z",
    updatedAt: "2026-02-28T00:00:00Z",
  },
];

/**
 * TODO: Document mockAdminWebhookApis.
 */
export async function mockAdminWebhookApis(
  page: Page,
  options: WebhooksMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (method === "GET" && path === "/api/webhooks") {
      if (options.listResponder) {
        const resp = options.listResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, { items: defaultWebhooks });
    }

    if (method === "POST" && path.endsWith("/test")) {
      const id = path.replace("/api/webhooks/", "").replace("/test", "");
      if (options.testResponder) {
        const resp = options.testResponder(id);
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, { success: true, statusCode: 200, durationMs: 42, error: null });
    }

    if (method === "DELETE" && path.startsWith("/api/webhooks/")) {
      const id = path.replace("/api/webhooks/", "");
      if (options.deleteResponder) {
        const resp = options.deleteResponder(id);
        return json(route, resp.status, resp.body);
      }
      return route.fulfill({ status: 204, body: "" });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}
