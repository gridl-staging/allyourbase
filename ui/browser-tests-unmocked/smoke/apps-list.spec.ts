import {
  test,
  expect,
  probeEndpoint,
  ensureUserByEmail,
  cleanupUserByEmail,
  seedAdminApp,
  cleanupAdminAppByName,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Apps - List View
 *
 * Critical Path: Navigate to Apps → Verify seeded app row renders with expected details
 */

test.describe("Smoke: Apps List", () => {
  const appNames: string[] = [];
  const userEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (appNames.length > 0) {
      const name = appNames.pop();
      if (!name) continue;
      await cleanupAdminAppByName(request, adminToken, name).catch(() => {});
    }
    while (userEmails.length > 0) {
      const email = userEmails.pop();
      if (!email) continue;
      await cleanupUserByEmail(request, adminToken, email).catch(() => {});
    }
  });

  test("seeded app renders in applications table", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/apps");
    test.skip(
      probeStatus === 404 || probeStatus === 501,
      `Apps service not configured (status ${probeStatus})`,
    );

    const runId = Date.now();
    const ownerEmail = `apps-smoke-owner-${runId}@test.com`;
    const appName = `smoke-app-${runId}`;
    const appDescription = `Seeded smoke app ${runId}`;

    const owner = await ensureUserByEmail(request, adminToken, ownerEmail);
    userEmails.push(ownerEmail);

    await seedAdminApp(request, adminToken, {
      name: appName,
      description: appDescription,
      ownerUserId: owner.id,
      rateLimitRps: 120,
      rateLimitWindowSeconds: 60,
    });
    appNames.push(appName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Apps$/i }).click();
    await expect(page.getByRole("heading", { name: /Applications/i })).toBeVisible({ timeout: 15_000 });

    const row = page.getByRole("row", { name: new RegExp(appName) }).first();
    await expect(row).toBeVisible({ timeout: 5000 });
    await expect(row.getByText(appDescription)).toBeVisible();
    await expect(row.getByRole("button", { name: /Delete app/i })).toBeVisible();
  });
});
