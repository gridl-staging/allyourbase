/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/incidents.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface IncidentsMockOptions {
  listResponder?: () => MockApiResponse;
  createResponder?: (body: Record<string, unknown>) => MockApiResponse;
  updateResponder?: (id: string, body: Record<string, unknown>) => MockApiResponse;
  addUpdateResponder?: (id: string, body: Record<string, unknown>) => MockApiResponse;
}

/**
 * TODO: Document mockAdminIncidentApis.
 */
export async function mockAdminIncidentApis(
  page: Page,
  options: IncidentsMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    // POST /api/admin/incidents/:id/updates
    const updatesMatch = path.match(/^\/api\/admin\/incidents\/([^/]+)\/updates$/);
    if (method === "POST" && updatesMatch) {
      const id = updatesMatch[1];
      const body = request.postDataJSON() as Record<string, unknown>;
      if (options.addUpdateResponder) {
        const resp = options.addUpdateResponder(id, body);
        return json(route, resp.status, resp.body);
      }
      return json(route, 201, {
        id: "update-001",
        incident_id: id,
        message: String(body.message ?? ""),
        status: String(body.status ?? "investigating"),
        created_at: "2026-02-28T12:01:00Z",
      });
    }

    // PUT /api/admin/incidents/:id
    const incidentIdMatch = path.match(/^\/api\/admin\/incidents\/([^/]+)$/);
    if (method === "PUT" && incidentIdMatch) {
      const id = incidentIdMatch[1];
      const body = request.postDataJSON() as Record<string, unknown>;
      if (options.updateResponder) {
        const resp = options.updateResponder(id, body);
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        id,
        title: String(body.title ?? "Test incident"),
        status: String(body.status ?? "investigating"),
        affectedServices: [],
        createdAt: "2026-02-28T12:00:00Z",
        updatedAt: "2026-02-28T12:01:00Z",
      });
    }

    // POST /api/admin/incidents (create)
    if (method === "POST" && path === "/api/admin/incidents") {
      const body = request.postDataJSON() as Record<string, unknown>;
      if (options.createResponder) {
        const resp = options.createResponder(body);
        return json(route, resp.status, resp.body);
      }
      return json(route, 201, {
        id: "inc-001",
        title: String(body.title ?? ""),
        status: String(body.status ?? "investigating"),
        affectedServices: body.affected_services ?? [],
        createdAt: "2026-02-28T12:00:00Z",
        updatedAt: "2026-02-28T12:00:00Z",
      });
    }

    // GET /api/admin/incidents
    if (method === "GET" && path === "/api/admin/incidents") {
      if (options.listResponder) {
        const resp = options.listResponder();
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, []);
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });
}
