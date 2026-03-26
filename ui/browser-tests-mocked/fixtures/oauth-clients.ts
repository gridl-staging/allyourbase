/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/oauth-clients.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface OAuthClientsMockOptions {
  listResponder?: () => MockApiResponse;
  createResponder?: (body: Record<string, unknown>) => MockApiResponse;
  revokeResponder?: (clientId: string) => MockApiResponse;
  rotateResponder?: (clientId: string) => MockApiResponse;
}

const defaultClients = [
  {
    clientId: "oauth-001",
    name: "Portal Client",
    appId: "app-001",
    clientType: "confidential",
    redirectUris: ["https://portal.example.com/callback"],
    scopes: ["readonly"],
    createdAt: "2026-02-28T00:00:00Z",
  },
];

const defaultApps = [
  { id: "app-001", name: "Portal App", description: "", ownerUserId: "user-001", rateLimitRps: 10, rateLimitWindowSeconds: 60, createdAt: "2026-02-28T00:00:00Z", updatedAt: "2026-02-28T00:00:00Z" },
];

/**
 * TODO: Document mockAdminOAuthClientApis.
 */
export async function mockAdminOAuthClientApis(
  page: Page,
  options: OAuthClientsMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (method === "GET" && path === "/api/admin/oauth/clients") {
      if (options.listResponder) {
        const resp = options.listResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        items: defaultClients,
        page: 1,
        perPage: 20,
        totalItems: defaultClients.length,
        totalPages: 1,
      });
    }

    if (method === "POST" && path === "/api/admin/oauth/clients") {
      const body = request.postDataJSON() as Record<string, unknown>;
      if (options.createResponder) {
        const resp = options.createResponder(body);
        return json(route, resp.status, resp.body);
      }
      return json(route, 201, {
        clientSecret: "mock_client_secret_value",
        client: {
          clientId: "oauth-002",
          name: String(body.name ?? ""),
          appId: String(body.appId ?? ""),
          clientType: String(body.clientType ?? "confidential"),
          redirectUris: body.redirectUris ?? [],
          scopes: body.scopes ?? ["readonly"],
          createdAt: "2026-02-28T12:00:00Z",
        },
      });
    }

    if (method === "POST" && path.endsWith("/rotate-secret")) {
      const clientId = path.replace("/api/admin/oauth/clients/", "").replace("/rotate-secret", "");
      if (options.rotateResponder) {
        const resp = options.rotateResponder(clientId);
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, { clientSecret: "mock_rotated_secret_value" });
    }

    if (method === "DELETE" && path.startsWith("/api/admin/oauth/clients/")) {
      const clientId = path.replace("/api/admin/oauth/clients/", "");
      if (options.revokeResponder) {
        const resp = options.revokeResponder(clientId);
        return json(route, resp.status, resp.body);
      }
      return route.fulfill({ status: 204, body: "" });
    }

    if (method === "GET" && path === "/api/admin/apps") {
      return json(route, 200, {
        items: defaultApps,
        page: 1,
        perPage: 100,
        totalItems: defaultApps.length,
        totalPages: 0,
      });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}
