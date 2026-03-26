import type { Page } from "@playwright/test";
import {
  createEdgeFunctionMockState,
  createEdgeFunctionMockStore,
  type EdgeFunctionMockOptions,
  type EdgeFunctionMockState,
} from "./edge-function-mock-state";
import { handleEdgeFunctionApiRoute } from "./edge-function-routes";

export type { EdgeFunctionMockOptions, EdgeFunctionMockState } from "./edge-function-mock-state";

/**
 * Comprehensive mock for edge function management: CRUD operations, invocation, execution logs, and three trigger types (database, cron, storage) with enable/disable/delete support. Generates sequential IDs and maintains state across operations. @param page - Playwright page @param options - optional custom responders for deploy, update, invoke operations @returns promise resolving to EdgeFunctionMockState with extensive tracking of all operations and trigger management
 */
export async function mockAdminEdgeFunctionApis(
  page: Page,
  options?: EdgeFunctionMockOptions,
): Promise<EdgeFunctionMockState> {
  const state = createEdgeFunctionMockState();
  const store = createEdgeFunctionMockStore();

  await page.route("**/api/**", async (route) =>
    handleEdgeFunctionApiRoute(route, options, state, store),
  );

  return state;
}
