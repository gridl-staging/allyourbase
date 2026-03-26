/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/api-keys.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface ApiKeysMockOptions {
  listResponder?: () => MockApiResponse;
  createResponder?: (body: Record<string, unknown>) => MockApiResponse;
  revokeResponder?: (id: string) => MockApiResponse;
}

const defaultKeys = [
  {
    id: "key-001",
    name: "Service Key",
    userId: "user-001",
    scope: "*",
    prefix: "ayb_sk_",
    createdAt: "2026-02-28T00:00:00Z",
  },
];

const defaultUsers = [
  { id: "user-001", email: "admin@example.com", emailVerified: true, createdAt: "2026-02-28T00:00:00Z", updatedAt: "2026-02-28T00:00:00Z" },
];

/**
 * TODO: Document mockAdminApiKeyApis.
 */
export async function mockAdminApiKeyApis(
  page: Page,
  options: ApiKeysMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (method === "GET" && path === "/api/admin/api-keys") {
      if (options.listResponder) {
        const resp = options.listResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        items: defaultKeys,
        page: 1,
        perPage: 20,
        totalItems: defaultKeys.length,
        totalPages: 1,
      });
    }

    if (method === "POST" && path === "/api/admin/api-keys") {
      const body = request.postDataJSON() as Record<string, unknown>;
      if (options.createResponder) {
        const resp = options.createResponder(body);
        return json(route, resp.status, resp.body);
      }
      return json(route, 201, {
        key: "ayb_sk_mock_secret_key_value",
        apiKey: {
          id: "key-002",
          name: String(body.name ?? ""),
          userId: String(body.userId ?? ""),
          scope: String(body.scope ?? "*"),
          prefix: "ayb_sk_",
          createdAt: "2026-02-28T12:00:00Z",
        },
      });
    }

    if (method === "DELETE" && path.startsWith("/api/admin/api-keys/")) {
      const id = path.replace("/api/admin/api-keys/", "");
      if (options.revokeResponder) {
        const resp = options.revokeResponder(id);
        return json(route, resp.status, resp.body);
      }
      return route.fulfill({ status: 204, body: "" });
    }

    if (method === "GET" && path === "/api/admin/users") {
      return json(route, 200, {
        items: defaultUsers,
        page: 1,
        perPage: 100,
        totalItems: defaultUsers.length,
        totalPages: 1,
      });
    }

    if (method === "GET" && path === "/api/admin/apps") {
      return json(route, 200, { items: [], page: 1, perPage: 100, totalItems: 0, totalPages: 0 });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}
