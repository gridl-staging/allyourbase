/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/secrets.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface SecretsMockOptions {
  listResponder?: () => MockApiResponse;
  createResponder?: (body: Record<string, unknown>) => MockApiResponse;
  rotateResponder?: () => MockApiResponse;
}

const defaultSecrets = [
  { name: "DATABASE_URL", createdAt: "2026-02-28T00:00:00Z", updatedAt: "2026-02-28T00:00:00Z" },
  { name: "API_TOKEN", createdAt: "2026-02-28T00:00:00Z", updatedAt: "2026-02-28T00:00:00Z" },
];

/**
 * TODO: Document mockAdminSecretApis.
 */
export async function mockAdminSecretApis(
  page: Page,
  options: SecretsMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (method === "GET" && path === "/api/admin/secrets") {
      if (options.listResponder) {
        const resp = options.listResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, defaultSecrets);
    }

    if (method === "POST" && path === "/api/admin/secrets/rotate") {
      if (options.rotateResponder) {
        const resp = options.rotateResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, { status: "rotated" });
    }

    if (method === "POST" && path === "/api/admin/secrets") {
      const body = request.postDataJSON() as Record<string, unknown>;
      if (options.createResponder) {
        const resp = options.createResponder(body);
        return json(route, resp.status, resp.body);
      }
      return route.fulfill({ status: 204, body: "" });
    }

    if (method === "DELETE" && path.startsWith("/api/admin/secrets/")) {
      return route.fulfill({ status: 204, body: "" });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}
