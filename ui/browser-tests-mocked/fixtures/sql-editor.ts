/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/sql-editor.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface SqlEditorMockOptions {
  executeResponder?: (query: string) => MockApiResponse;
}

/**
 * TODO: Document mockAdminSqlEditorApis.
 */
export async function mockAdminSqlEditorApis(
  page: Page,
  options: SqlEditorMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (method === "POST" && path === "/api/admin/sql/") {
      const body = request.postDataJSON() as Record<string, unknown>;
      const query = String(body.query ?? "");
      if (options.executeResponder) {
        const resp = options.executeResponder(query);
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        columns: ["result"],
        rows: [[1]],
        rowCount: 1,
        durationMs: 2,
      });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}
