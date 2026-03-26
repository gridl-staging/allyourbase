/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/admin-logs.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute } from "./core";

export interface MockAdminLogsResponse {
  entries: Array<{
    time: string;
    level: string;
    message: string;
    attrs?: Record<string, unknown>;
  }>;
  message?: string;
}

export interface AdminLogsMockOptions {
  responses?: MockAdminLogsResponse[];
}

export interface AdminLogsMockState {
  logsCalls: number;
}

/**
 * TODO: Document mockAdminLogsApis.
 */
export async function mockAdminLogsApis(
  page: Page,
  options: AdminLogsMockOptions = {},
): Promise<AdminLogsMockState> {
  const responses =
    options.responses && options.responses.length > 0
      ? options.responses
      : [{ entries: [] }];

  const state: AdminLogsMockState = {
    logsCalls: 0,
  };

  await page.route("**/api/**", async (route) => {
    const method = route.request().method();
    const url = new URL(route.request().url());
    const path = url.pathname;

    if (method === "GET" && path === "/api/admin/logs") {
      const responseIndex = Math.min(state.logsCalls, responses.length - 1);
      const response = responses[responseIndex];
      state.logsCalls += 1;
      return json(route, 200, response);
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });

  return state;
}
