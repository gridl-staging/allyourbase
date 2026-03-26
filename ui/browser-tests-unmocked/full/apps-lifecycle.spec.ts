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
 * FULL E2E TEST: Apps Lifecycle
 *
 * Critical Path: Load seeded app row → create app via UI → delete app via UI
 */

test.describe("Apps Lifecycle (Full E2E)", () => {
  const appNames: string[] = [];
  const userEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (appNames.length > 0) {
      const appName = appNames.pop();
      if (!appName) continue;
      await cleanupAdminAppByName(request, adminToken, appName).catch(() => {});
    }
    while (userEmails.length > 0) {
      const email = userEmails.pop();
      if (!email) continue;
      await cleanupUserByEmail(request, adminToken, email).catch(() => {});
    }
  });

  test("load-and-verify seeded app, then create and delete app", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/apps");
    test.skip(
      probeStatus === 404 || probeStatus === 501,
      `Apps service not configured (status ${probeStatus})`,
    );

    const runId = Date.now();
    const ownerEmail = `apps-full-owner-${runId}@test.com`;
    const seededName = `apps-full-seeded-${runId}`;
    const seededDescription = `Seeded app for full lifecycle ${runId}`;
    const createdName = `apps-full-created-${runId}`;
    const createdDescription = `Created app from full lifecycle ${runId}`;

    const owner = await ensureUserByEmail(request, adminToken, ownerEmail);
    userEmails.push(ownerEmail);

    await seedAdminApp(request, adminToken, {
      name: seededName,
      description: seededDescription,
      ownerUserId: owner.id,
      rateLimitRps: 60,
      rateLimitWindowSeconds: 60,
    });
    appNames.push(seededName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Apps$/i }).click();
    await expect(page.getByRole("heading", { name: /Applications/i })).toBeVisible({ timeout: 5000 });

    const seededRow = page.getByRole("row", { name: new RegExp(seededName) }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });
    await expect(seededRow).toContainText(seededDescription);
    await expect(seededRow.getByRole("button", { name: /Delete app/i })).toBeVisible();

    await page.getByRole("button", { name: /^Create App$/i }).click();
    await expect(page.getByRole("heading", { name: /Create Application/i })).toBeVisible({ timeout: 5000 });

    await page.getByLabel("App name").fill(createdName);
    await page.getByLabel("Description").fill(createdDescription);
    await page.getByLabel("Owner").selectOption(owner.id);
    await page.getByRole("button", { name: /^Create$/i }).click();

    appNames.push(createdName);

    const createdRow = page.getByRole("row", { name: new RegExp(createdName) }).first();
    await expect(createdRow).toBeVisible({ timeout: 5000 });
    await expect(createdRow).toContainText(createdDescription);

    await createdRow.getByRole("button", { name: /Delete app/i }).click();
    await expect(page.getByRole("heading", { name: /Delete Application/i })).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Delete$/i }).click();

    await expect(page.getByRole("row", { name: new RegExp(createdName) })).toHaveCount(0, { timeout: 5000 });
  });
});
