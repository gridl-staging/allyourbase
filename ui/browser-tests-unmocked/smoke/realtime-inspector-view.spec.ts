import { test, expect, probeEndpoint, fetchRealtimeStats, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Realtime Inspector — Content-Verified
 *
 * Fetches the live realtime stats snapshot from the backend, then asserts
 * the rendered metric cards show the actual connection count. The frontend
 * adapter maps backend `/admin/realtime/stats` to the UI snapshot shape.
 */

test.describe("Smoke: Realtime Inspector", () => {
  test("realtime inspector renders metrics matching live API data", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/realtime/stats");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Realtime service unavailable (status ${probeStatus})`,
    );

    // Arrange: fetch live realtime stats via fixture helper
    const stats = await fetchRealtimeStats(request, adminToken);

    // Act: navigate to Realtime Inspector
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Realtime Inspector$/i }).click();
    await expect(page.getByRole("heading", { name: /Realtime Inspector/i })).toBeVisible({ timeout: 15_000 });

    const panel = page.getByTestId("realtime-inspector-panel");
    await expect(panel).toBeVisible({ timeout: 5000 });

    // Assert: Total metric card displays the actual connection count
    const totalCard = panel.getByTestId("realtime-total-metric");
    await expect(totalCard).toBeVisible({ timeout: 5000 });
    await expect(panel.getByTestId("realtime-total-metric-value")).toContainText(String(stats.connections.total));

    // Assert: Subscriptions section with table or empty state
    await expect(panel.getByRole("heading", { name: /^Subscriptions$/i })).toBeVisible();
    const nameHeader = panel.getByRole("columnheader", { name: /^Name$/i });
    const noSubscriptions = panel.getByText(/No active subscriptions/i);
    await expect(nameHeader.or(noSubscriptions)).toBeVisible({ timeout: 5000 });
  });
});
