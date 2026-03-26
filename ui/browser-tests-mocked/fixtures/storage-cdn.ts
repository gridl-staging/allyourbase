/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/storage-cdn.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute } from "./core";

interface CDNPurgeResponderInput {
  urls?: string[];
  purge_all?: boolean;
}

interface CDNPurgeResponse {
  status: number;
  body: unknown;
}

export interface StorageCDNPurgeMockOptions {
  purgeResponder?: (req: CDNPurgeResponderInput) => CDNPurgeResponse;
}

export interface StorageCDNPurgeMockState {
  purgeCalls: number;
  lastPurgeBody: CDNPurgeResponderInput | null;
}

/**
 * TODO: Document mockStorageCDNApis.
 */
export async function mockStorageCDNApis(
  page: Page,
  options: StorageCDNPurgeMockOptions = {},
): Promise<StorageCDNPurgeMockState> {
  const state: StorageCDNPurgeMockState = {
    purgeCalls: 0,
    lastPurgeBody: null,
  };

  await page.route("**/api/**", async (route) => {
    const method = route.request().method();
    const url = new URL(route.request().url());
    const path = url.pathname;

    // Storage file list (StorageBrowser loads this on mount)
    if (method === "GET" && path.match(/^\/api\/storage\/[^/]+$/)) {
      return json(route, 200, { items: [], totalItems: 0 });
    }

    // CDN purge endpoint
    if (method === "POST" && path === "/api/admin/storage/cdn/purge") {
      state.purgeCalls += 1;
      const body = route.request().postDataJSON() as CDNPurgeResponderInput;
      state.lastPurgeBody = body;

      if (options.purgeResponder) {
        const resp = options.purgeResponder(body);
        return json(route, resp.status, resp.body);
      }

      // Default: 202 success
      const operation = body.purge_all ? "purge_all" : "purge_urls";
      const submitted = body.purge_all ? 0 : (body.urls?.length ?? 0);
      return json(route, 202, { operation, submitted, provider: "cloudflare" });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });

  return state;
}
