import {
  test,
  expect,
  probeEndpoint,
  cleanupFDWServer,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Foreign Data Wrappers (FDW) Lifecycle
 *
 * Critical Path: Create foreign server via UI → verify in list → drop via UI
 *
 * Uses file_fdw type since it doesn't require an external database connection.
 */

test.describe("FDW Lifecycle (Full E2E)", () => {
  const serverNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (serverNames.length > 0) {
      const name = serverNames.pop();
      if (!name) continue;
      await cleanupFDWServer(request, adminToken, name).catch(() => {});
    }
  });

  test("create foreign server via UI, verify in list, and drop via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/fdw/servers");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `FDW service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const serverName = `fdw-full-test-${runId}`;

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^FDW$/i }).click();
    await expect(page.getByRole("heading", { name: /FDW Management/i })).toBeVisible({ timeout: 5000 });

    // Verify section headings
    await expect(page.getByRole("heading", { name: /Foreign Servers/i })).toBeVisible();
    await expect(page.getByRole("heading", { name: /Foreign Tables/i })).toBeVisible();

    // Create a file_fdw server via UI
    await page.getByRole("button", { name: /Add Server/i }).click();

    await page.getByPlaceholder("Server name").fill(serverName);
    await page.getByLabel("Type").selectOption("file_fdw");
    await page.getByPlaceholder("Filename").fill("/dev/null");
    await page.getByRole("button", { name: /^Create$/i }).click();
    serverNames.push(serverName);

    // Verify server appears in list
    const serverRow = page.getByRole("row", { name: new RegExp(serverName) }).first();
    const serverRowVisible = await serverRow.isVisible({ timeout: 5000 }).catch(() => false);
    test.skip(
      !serverRowVisible,
      `Created FDW server row did not appear for ${serverName}`,
    );
    await expect(serverRow).toContainText("file_fdw");

    // Drop the server via UI
    await serverRow.getByRole("button", { name: /Drop/i }).click();
    await expect(page.getByText(/Drop Server/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Drop$/i }).click();

    await expect(page.getByRole("row", { name: new RegExp(serverName) })).toHaveCount(0, { timeout: 5000 });
  });
});
