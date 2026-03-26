import {
  test,
  expect,
  probeEndpoint,
  cleanupPushTestData,
  seedPushDeviceToken,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Push Notifications - View
 *
 * Critical Path: Navigate to Push Notifications → verify tabs, device table headers, and register action
 */

test.describe("Smoke: Push Notifications", () => {
  const seededTokenPatterns: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededTokenPatterns.length > 0) {
      const tokenPattern = seededTokenPatterns.pop();
      if (!tokenPattern) continue;
      await cleanupPushTestData(request, adminToken, tokenPattern).catch(() => {});
    }
  });

  test("seeded push device token renders in devices tab", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/push/devices");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Push notifications service unavailable (status ${probeStatus})`,
    );

    const tokenValue = `smk${Date.now()}`;
    await seedPushDeviceToken(request, adminToken, { tokenValue });
    seededTokenPatterns.push(tokenValue);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Push Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: /Push Notifications/i })).toBeVisible({ timeout: 15_000 });

    await expect(page.getByRole("button", { name: /^Devices$/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /^Token$/i })).toBeVisible({ timeout: 5000 });

    const seededDeviceRow = page.locator("tr").filter({ hasText: tokenValue }).first();
    await expect(seededDeviceRow).toBeVisible({ timeout: 5000 });
  });
});
