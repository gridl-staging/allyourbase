/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/apps.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface AppsMockState {
  listAppsCalls: number;
  listUsersCalls: number;
  createAppCalls: number;
  lastCreateBody: Record<string, unknown> | null;
}

export interface AppsMockOptions {
  createResponder?: (body: Record<string, unknown>) => MockApiResponse;
  listAppsResponder?: () => MockApiResponse;
  listUsersResponder?: () => MockApiResponse;
}

const defaultApps = [
  {
    id: "app-001",
    name: "Starter App",
    description: "Seeded mocked app",
    ownerUserId: "user-001",
    rateLimitRps: 10,
    rateLimitWindowSeconds: 60,
    createdAt: "2026-02-24T12:00:00Z",
    updatedAt: "2026-02-24T12:00:00Z",
  },
];

const defaultUsers = [
  {
    id: "user-001",
    email: "owner@example.com",
    emailVerified: true,
    createdAt: "2026-02-24T10:00:00Z",
    updatedAt: "2026-02-24T10:00:00Z",
  },
];

/**
 * TODO: Document mockAdminAppsApis.
 */
export async function mockAdminAppsApis(
  page: Page,
  options: AppsMockOptions = {},
): Promise<AppsMockState> {
  const state: AppsMockState = {
    listAppsCalls: 0,
    listUsersCalls: 0,
    createAppCalls: 0,
    lastCreateBody: null,
  };

  const apps = defaultApps.map((app) => ({ ...app }));
  let nextAppNumber =
    apps.reduce((max, app) => {
      const match = app.id.match(/^app-(\d+)$/);
      if (!match) return max;
      return Math.max(max, Number(match[1]));
    }, 0) + 1;

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (await handleCommonAdminRoutes(route, method, path)) return;

    if (method === "GET" && path === "/api/admin/users") {
      state.listUsersCalls += 1;
      if (options.listUsersResponder) {
        const resp = options.listUsersResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        items: defaultUsers,
        page: 1,
        perPage: 100,
        totalItems: defaultUsers.length,
        totalPages: 1,
      });
    }

    if (method === "GET" && path === "/api/admin/apps") {
      state.listAppsCalls += 1;
      if (options.listAppsResponder) {
        const resp = options.listAppsResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        items: apps,
        page: 1,
        perPage: 20,
        totalItems: apps.length,
        totalPages: 1,
      });
    }

    if (method === "POST" && path === "/api/admin/apps") {
      state.createAppCalls += 1;
      const body = request.postDataJSON() as Record<string, unknown>;
      state.lastCreateBody = body;

      if (options.createResponder) {
        const resp = options.createResponder(body);
        return json(route, resp.status, resp.body);
      }

      const now = "2026-02-25T12:00:00Z";
      const created = {
        id: `app-${String(nextAppNumber).padStart(3, "0")}`,
        name: String(body.name ?? ""),
        description: String(body.description ?? ""),
        ownerUserId: String(body.ownerUserId ?? ""),
        rateLimitRps: 0,
        rateLimitWindowSeconds: 0,
        createdAt: now,
        updatedAt: now,
      };
      nextAppNumber += 1;
      apps.unshift(created);

      return json(route, 201, created);
    }

    return unhandledMockedApiRoute(route, method, path);
  });

  return state;
}
