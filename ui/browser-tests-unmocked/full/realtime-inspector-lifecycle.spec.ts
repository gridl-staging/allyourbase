import {
  test,
  expect,
  probeEndpoint,
  fetchRealtimeStats,
  waitForDashboard,
} from "../fixtures";

test.describe("Realtime Inspector Lifecycle (Full E2E)", () => {
  test("renders metrics from live snapshot and supports manual refresh", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/realtime/stats");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Realtime stats endpoint unavailable (status ${probeStatus})`,
    );

    const stats = await fetchRealtimeStats(request, adminToken);

    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Realtime Inspector$/i }).click();
    await expect(page.getByRole("heading", { name: /Realtime Inspector/i })).toBeVisible({ timeout: 5000 });

    const panel = page.getByTestId("realtime-inspector-panel");
    await expect(panel).toBeVisible({ timeout: 5000 });

    // Assert all five metric cards render values matching the live fixture snapshot.
    await expect(panel.getByTestId("realtime-total-metric-value")).toHaveText(
      String(stats.connections.total),
    );
    await expect(panel.getByTestId("realtime-sse-metric-value")).toHaveText(
      String(stats.connections.sse),
    );
    await expect(panel.getByTestId("realtime-ws-metric-value")).toHaveText(
      String(stats.connections.ws),
    );
    await expect(panel.getByTestId("realtime-dropped-metric-value")).toHaveText(
      String(stats.counters.dropped_messages),
    );
    await expect(panel.getByTestId("realtime-heartbeat-failures-metric-value")).toHaveText(
      String(stats.counters.heartbeat_failures),
    );

    await expect(panel.getByRole("heading", { name: /^Subscriptions$/i })).toBeVisible();
    const subscriptionsTableOrEmpty = panel
      .getByRole("columnheader", { name: /^Name$/i })
      .or(panel.getByText(/No active subscriptions/i));
    await expect(subscriptionsTableOrEmpty).toBeVisible({ timeout: 5000 });

    await Promise.all([
      page.waitForResponse((response) =>
        response.url().includes("/api/admin/realtime/stats") && response.status() === 200,
      ),
      panel.getByRole("button", { name: "Refresh" }).click(),
    ]);

    await expect(panel.getByTestId("realtime-total-metric-value")).toBeVisible();
  });
});
