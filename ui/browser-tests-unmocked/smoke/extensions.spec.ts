import {
  test,
  expect,
  probeEndpoint,
  enableExtension,
  disableExtension,
  execSQL,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Extensions
 *
 * Critical Path: Navigate to Extensions → Verify page heading, table structure,
 * and at least one extension row renders with name/status/version in the page body.
 */

test.describe("Smoke: Extensions", () => {
  const extensionsToDisableAfterTest: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (extensionsToDisableAfterTest.length > 0) {
      const extensionName = extensionsToDisableAfterTest.pop();
      if (!extensionName) continue;
      await disableExtension(request, adminToken, extensionName).catch(() => {});
    }
  });

  test("enabled extension renders as installed in extensions table", async ({ page, request, adminToken }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/extensions");
    test.skip(
      status === 501 || status === 404,
      `Extensions endpoint not available (status ${status})`,
    );

    const extensionName = "pgcrypto";
    const extensionAvailability = await execSQL(
      request,
      adminToken,
      `SELECT EXISTS (
         SELECT 1
         FROM pg_available_extensions
         WHERE name = '${extensionName}'
       )`,
    );
    const isAvailable = extensionAvailability.rows[0]?.[0] === true;
    test.skip(!isAvailable, `${extensionName} is not available in this Postgres environment`);

    const installedState = await execSQL(
      request,
      adminToken,
      `SELECT EXISTS (
         SELECT 1
         FROM pg_extension
         WHERE extname = '${extensionName}'
       )`,
    );
    const wasInstalledBeforeTest = installedState.rows[0]?.[0] === true;

    await enableExtension(request, adminToken, extensionName);
    if (!wasInstalledBeforeTest) {
      extensionsToDisableAfterTest.push(extensionName);
    }

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Extensions/i }).click();
    await expect(page.getByRole("heading", { name: /Extensions/i })).toBeVisible({ timeout: 15_000 });

    const extensionRow = page.locator("tr").filter({ hasText: extensionName }).first();
    await expect(extensionRow).toBeVisible({ timeout: 5000 });
    await expect(extensionRow.getByText("installed")).toBeVisible();
    await expect(extensionRow.getByRole("button", { name: /^Disable$/i })).toBeVisible();
  });
});
