import { test, expect, bootstrapMockedAdminApp } from "./fixtures";

test.describe("Realtime Inspector (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);

    await page.route("**/api/**", async (route) => {
      const url = new URL(route.request().url());
      const path = url.pathname;
      const method = route.request().method();

      if (method === "GET" && path === "/api/admin/status") {
        return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ auth: true }) });
      }
      if (method === "GET" && path === "/api/schema") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ tables: {}, schemas: ["public"], builtAt: "2026-02-28T00:00:00Z" }),
        });
      }
      if (method === "GET" && path === "/api/admin/realtime/stats") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            version: "v1",
            timestamp: "2026-03-15T00:00:00Z",
            connections: { sse: 2, ws: 3, total: 5 },
            subscriptions: {
              tables: { public_posts: 3 },
              channels: {
                broadcast: { "room:lobby": 2 },
                presence: {},
              },
            },
            counters: { dropped_messages: 1, heartbeat_failures: 0 },
          }),
        });
      }

      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({}) });
    });
  });

  test("opens panel and renders live realtime telemetry", async ({ page }) => {
    await page.goto("/admin/");

    await page.getByRole("button", { name: /Realtime Inspector/i }).click();
    await expect(page.getByRole("heading", { name: /Realtime Inspector/i })).toBeVisible();

    // Connection metric cards
    await expect(page.getByTestId("realtime-total-metric-value")).toHaveText("5");
    await expect(page.getByTestId("realtime-sse-metric-value")).toHaveText("2");
    await expect(page.getByTestId("realtime-ws-metric-value")).toHaveText("3");

    // Counter cards
    await expect(page.getByTestId("realtime-dropped-metric-value")).toHaveText("1");
    await expect(page.getByTestId("realtime-heartbeat-failures-metric-value")).toHaveText("0");

    // Subscription table rows
    await expect(page.getByText("public_posts")).toBeVisible();
    await expect(page.getByRole("cell", { name: "3" })).toBeVisible();
    await expect(page.getByText("room:lobby")).toBeVisible();
  });
});
