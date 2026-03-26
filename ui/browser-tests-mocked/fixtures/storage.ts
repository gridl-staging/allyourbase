/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/storage.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface StorageMockOptions {
  listResponder?: (bucket: string) => MockApiResponse;
  deleteResponder?: (bucket: string, name: string) => MockApiResponse;
  uploadResponder?: (bucket: string) => MockApiResponse;
}

const defaultFiles = [
  {
    id: "file-001",
    bucket: "default",
    name: "report.pdf",
    contentType: "application/pdf",
    size: 1048576,
    createdAt: "2026-02-28T00:00:00Z",
  },
];

/**
 * TODO: Document mockAdminStorageApis.
 */
export async function mockAdminStorageApis(
  page: Page,
  options: StorageMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (path.startsWith("/api/storage/")) {
      const rest = path.replace("/api/storage/", "");
      const parts = rest.split("/");
      const bucket = parts[0];

      if (method === "GET" && parts.length === 1) {
        if (options.listResponder) {
          const resp = options.listResponder(bucket);
          return json(route, resp.status, resp.body);
        }
        return json(route, 200, { items: defaultFiles, totalItems: defaultFiles.length });
      }

      if (method === "POST" && parts.length === 1) {
        if (options.uploadResponder) {
          const resp = options.uploadResponder(bucket);
          return json(route, resp.status, resp.body);
        }
        return json(route, 201, {
          id: "file-002",
          bucket,
          name: "uploaded.txt",
          contentType: "text/plain",
          size: 256,
          createdAt: "2026-02-28T12:00:00Z",
        });
      }

      if (method === "DELETE" && parts.length >= 2) {
        const fileName = parts.slice(1).join("/");
        if (options.deleteResponder) {
          const resp = options.deleteResponder(bucket, fileName);
          return json(route, resp.status, resp.body);
        }
        return route.fulfill({ status: 204, body: "" });
      }
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}
