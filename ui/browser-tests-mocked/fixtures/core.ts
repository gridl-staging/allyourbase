/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/core.ts.
 */
import type { Page, Route } from "@playwright/test";

export interface MockApiResponse {
  status: number;
  body: unknown;
}

export function json(route: Route, status: number, body: unknown): Promise<void> {
  return route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

export function unhandledMockedApiRoute(
  route: Route,
  method: string,
  path: string,
): Promise<void> {
  return json(route, 500, {
    message: `Unhandled mocked API route: ${method} ${path}`,
  });
}

const defaultSchemaBody = { tables: {}, schemas: ["public"], builtAt: "2026-02-28T00:00:00Z" };
const defaultStatusBody = { auth: true };

/**
 * TODO: Document handleCommonAdminRoutes.
 */
export async function handleCommonAdminRoutes(
  route: Route,
  method: string,
  path: string,
  options?: { schemaBody?: unknown; statusBody?: unknown },
): Promise<boolean> {
  if (method === "GET" && path === "/api/admin/status") {
    await json(route, 200, options?.statusBody ?? defaultStatusBody);
    return true;
  }

  if (method === "GET" && path === "/api/schema") {
    await json(route, 200, options?.schemaBody ?? defaultSchemaBody);
    return true;
  }

  return false;
}

export async function bootstrapMockedAdminApp(page: Page): Promise<void> {
  await page.addInitScript(() => {
    window.localStorage.setItem("ayb_admin_token", "mock-admin-token");
  });
}

export interface RealtimeInspectorMockOptions {
  statsResponder?: () => MockApiResponse;
}

/**
 * TODO: Document mockRealtimeInspectorApis.
 */
export async function mockRealtimeInspectorApis(
  page: Page,
  options: RealtimeInspectorMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const method = route.request().method();

    if (method === "GET" && path === "/api/admin/realtime/stats") {
      if (options.statsResponder) {
        const resp = options.statsResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        version: "v1",
        timestamp: "2026-02-28T00:00:00Z",
        connections: {
          sse: 1,
          ws: 4,
          total: 5,
        },
        subscriptions: {
          tables: { "public.posts": 3 },
          channels: {
            broadcast: { "public:posts": 3 },
            presence: {},
          },
        },
        counters: {
          dropped_messages: 0,
          heartbeat_failures: 0,
        },
      });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}

export interface PerformanceAdvisorMockOptions {
  reportResponder?: () => MockApiResponse;
}

/**
 * TODO: Document mockPerformanceAdvisorApis.
 */
export async function mockPerformanceAdvisorApis(
  page: Page,
  options: PerformanceAdvisorMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const method = route.request().method();

    if (method === "GET" && path === "/api/admin/advisors/performance") {
      if (options.reportResponder) {
        const resp = options.reportResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        generatedAt: "2026-02-28T00:00:00Z",
        stale: false,
        range: "1h",
        queries: [
          {
            fingerprint: "fp1",
            normalizedQuery: "select * from posts where author_id = $1",
            meanMs: 44.2,
            totalMs: 1200,
            calls: 27,
            rows: 442,
            endpoints: ["GET /api/collections/posts"],
            trend: "up",
          },
        ],
      });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}

export interface SecurityAdvisorMockOptions {
  reportResponder?: () => MockApiResponse;
}

/**
 * TODO: Document mockSecurityAdvisorApis.
 */
export async function mockSecurityAdvisorApis(
  page: Page,
  options: SecurityAdvisorMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const method = route.request().method();

    if (method === "GET" && path === "/api/admin/advisors/security") {
      if (options.reportResponder) {
        const resp = options.reportResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        evaluatedAt: "2026-02-28T00:00:00Z",
        stale: false,
        findings: [
          {
            id: "f1",
            severity: "critical",
            category: "rls",
            status: "open",
            title: "RLS disabled on public.posts",
            description: "Too broad table exposure",
            remediation: "Enable RLS and add restrictive policy",
          },
        ],
      });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}

export interface SchemaDesignerMockOptions {
  schemaResponder?: () => MockApiResponse;
}

/**
 * TODO: Document mockSchemaDesignerApis.
 */
export async function mockSchemaDesignerApis(
  page: Page,
  options: SchemaDesignerMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const method = route.request().method();

    if (method === "GET" && path === "/api/schema") {
      if (options.schemaResponder) {
        const resp = options.schemaResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        schemas: ["public"],
        builtAt: "2026-02-28T00:00:00Z",
        tables: {
          "public.users": {
            schema: "public",
            name: "users",
            kind: "table",
            columns: [{ name: "id", position: 1, type: "uuid", nullable: false, isPrimaryKey: true, jsonType: "string" }],
            primaryKey: ["id"],
          },
          "public.posts": {
            schema: "public",
            name: "posts",
            kind: "table",
            columns: [
              { name: "id", position: 1, type: "uuid", nullable: false, isPrimaryKey: true, jsonType: "string" },
              { name: "author_id", position: 2, type: "uuid", nullable: false, isPrimaryKey: false, jsonType: "string" },
            ],
            primaryKey: ["id"],
            foreignKeys: [
              {
                constraintName: "posts_author_id_fkey",
                columns: ["author_id"],
                referencedSchema: "public",
                referencedTable: "users",
                referencedColumns: ["id"],
              },
            ],
          },
        },
      });
    }

    if (method === "GET" && path.startsWith("/api/collections/")) {
      return json(route, 200, {
        items: [],
        count: 0,
        page: 1,
        perPage: 20,
        totalPages: 0,
      });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}

export async function mockThemePersistenceApis(page: Page): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const method = route.request().method();

    if (await handleCommonAdminRoutes(route, method, path, {
      schemaBody: { tables: {}, schemas: ["public"], builtAt: "2026-02-25T12:00:00Z" },
    })) return;

    return unhandledMockedApiRoute(route, method, path);
  });
}
