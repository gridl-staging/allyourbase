/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/fdw.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface FDWMockOptions {
  listServersResponder?: () => MockApiResponse;
  listTablesResponder?: () => MockApiResponse;
  dropServerResponder?: (name: string) => MockApiResponse;
  importTablesResponder?: (serverName: string) => MockApiResponse;
}

const defaultServers = [
  { name: "remote_pg", fdw_type: "postgres_fdw", options: { host: "remote.example.com", port: "5432", dbname: "analytics" }, created_at: "2026-02-28T00:00:00Z" },
];

const defaultTables = [
  { schema: "public", name: "remote_metrics", server_name: "remote_pg", columns: [{ name: "id", type: "integer" }, { name: "value", type: "numeric" }, { name: "ts", type: "timestamptz" }], options: {} },
];

/**
 * TODO: Document mockAdminFDWApis.
 */
export async function mockAdminFDWApis(
  page: Page,
  options: FDWMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (method === "GET" && path === "/api/admin/fdw/servers") {
      if (options.listServersResponder) {
        const resp = options.listServersResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, { servers: defaultServers });
    }

    if (method === "GET" && path === "/api/admin/fdw/tables") {
      if (options.listTablesResponder) {
        const resp = options.listTablesResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, { tables: defaultTables });
    }

    if (method === "DELETE" && path.startsWith("/api/admin/fdw/servers/")) {
      const name = path.replace("/api/admin/fdw/servers/", "");
      if (options.dropServerResponder) {
        const resp = options.dropServerResponder(name);
        return json(route, resp.status, resp.body);
      }
      return route.fulfill({ status: 204, body: "" });
    }

    if (method === "POST" && path.endsWith("/import")) {
      const serverName = path.replace("/api/admin/fdw/servers/", "").replace("/import", "");
      if (options.importTablesResponder) {
        const resp = options.importTablesResponder(serverName);
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, { tables: [] });
    }

    if (method === "DELETE" && path.startsWith("/api/admin/fdw/tables/")) {
      return route.fulfill({ status: 204, body: "" });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}
