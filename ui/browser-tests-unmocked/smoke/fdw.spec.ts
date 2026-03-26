import {
  test,
  expect,
  probeEndpoint,
  seedFDWServer,
  cleanupFDWServer,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: FDW Management
 *
 * Critical Path: Seed a file_fdw server → Navigate to FDW → Verify the seeded
 * server renders in the Foreign Servers table with name and type.
 */

test.describe("Smoke: FDW Management", () => {
  const seededServerNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededServerNames.length > 0) {
      const name = seededServerNames.pop();
      if (!name) continue;
      await cleanupFDWServer(request, adminToken, name).catch(() => {});
    }
  });

  test("seeded FDW server renders in the foreign servers table", async ({ page, request, adminToken }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/fdw/servers");
    test.skip(
      status === 501 || status === 404,
      `FDW service not configured (status ${status})`,
    );

    const runId = Date.now();
    const serverName = `smoke_fdw_${runId}`;
    try {
      await seedFDWServer(request, adminToken, {
        name: serverName,
        fdwType: "file_fdw",
      });
    } catch (err) {
      // file_fdw extension may not be available in all Postgres configurations.
      const msg = err instanceof Error ? err.message : String(err);
      test.skip(msg.includes("invalid option") || msg.includes("file_fdw"), `file_fdw not available: ${msg}`);
      return;
    }
    seededServerNames.push(serverName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /FDW/i }).click();
    await expect(page.getByRole("heading", { name: /FDW Management/i })).toBeVisible({ timeout: 15_000 });

    // Verify dual-section headings
    await expect(page.getByText("Foreign Servers")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("Foreign Tables")).toBeVisible();

    // Verify seeded server renders in the Foreign Servers table
    const serverRow = page.locator("tr").filter({ hasText: serverName }).first();
    await expect(serverRow).toBeVisible({ timeout: 5000 });
    await expect(serverRow.getByText("file_fdw")).toBeVisible();

    // Verify action controls
    await expect(page.getByRole("button", { name: /Add Server/i })).toBeVisible();
  });
});
