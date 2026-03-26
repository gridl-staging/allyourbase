/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/support-tickets.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface SupportTicketsMockOptions {
  listResponder?: () => MockApiResponse;
  getResponder?: (id: string) => MockApiResponse;
  updateResponder?: (id: string, body: Record<string, unknown>) => MockApiResponse;
  replyResponder?: (id: string, body: Record<string, unknown>) => MockApiResponse;
}

/**
 * TODO: Document mockAdminSupportTicketApis.
 */
export async function mockAdminSupportTicketApis(
  page: Page,
  options: SupportTicketsMockOptions = {},
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    // POST /api/admin/support/tickets/:id/messages
    const messagesMatch = path.match(/^\/api\/admin\/support\/tickets\/([^/]+)\/messages$/);
    if (method === "POST" && messagesMatch) {
      const id = messagesMatch[1];
      const body = request.postDataJSON() as Record<string, unknown>;
      if (options.replyResponder) {
        const resp = options.replyResponder(id, body);
        return json(route, resp.status, resp.body);
      }
      return json(route, 201, {
        id: "msg-001",
        ticket_id: id,
        sender_type: "support",
        body: String(body.body ?? ""),
        created_at: "2026-02-28T12:00:00Z",
      });
    }

    // PUT /api/admin/support/tickets/:id
    const ticketIdMatch = path.match(/^\/api\/admin\/support\/tickets\/([^/]+)$/);
    if (method === "PUT" && ticketIdMatch) {
      const id = ticketIdMatch[1];
      const body = request.postDataJSON() as Record<string, unknown>;
      if (options.updateResponder) {
        const resp = options.updateResponder(id, body);
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        id,
        tenant_id: "tenant-001",
        user_id: "user-001",
        subject: "Test ticket",
        status: String(body.status ?? "open"),
        priority: String(body.priority ?? "normal"),
        created_at: "2026-02-28T12:00:00Z",
        updated_at: "2026-02-28T12:01:00Z",
      });
    }

    // GET /api/admin/support/tickets/:id
    if (method === "GET" && ticketIdMatch) {
      const id = ticketIdMatch[1];
      if (options.getResponder) {
        const resp = options.getResponder(id);
        return json(route, resp.status, resp.body);
      }
      return json(route, 200, {
        ticket: {
          id,
          tenant_id: "tenant-001",
          user_id: "user-001",
          subject: "Test ticket",
          status: "open",
          priority: "normal",
          created_at: "2026-02-28T12:00:00Z",
          updated_at: "2026-02-28T12:00:00Z",
        },
        messages: [],
      });
    }

    // GET /api/admin/support/tickets
    if (method === "GET" && path === "/api/admin/support/tickets") {
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
