import { test, expect, execSQL } from "../fixtures";

/**
 * SMOKE TEST: Creating a table via SQL Editor auto-updates sidebar nav
 *
 * Critical Path: Admin creates table via SQL Editor → table appears in sidebar without refresh
 */

test.describe("Smoke: Create Table Nav Update", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("new table appears in sidebar after CREATE TABLE in SQL editor", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const tableName = `nav_auto_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Navigate to admin dashboard
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    const sidebar = page.locator("aside");

    // Verify the table does NOT exist in sidebar yet
    await expect(sidebar.getByText(tableName, { exact: true })).not.toBeVisible();

    // Navigate to SQL Editor via sidebar
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // Create table via SQL
    await sqlInput.fill(`CREATE TABLE ${tableName} (id SERIAL PRIMARY KEY, name TEXT NOT NULL);`);
    await page.getByRole("button", { name: /run|execute/i }).click();

    // Wait for success message
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // Assert: table should now appear in sidebar WITHOUT clicking refresh
    await expect(sidebar.getByText(tableName, { exact: true })).toBeVisible({ timeout: 10000 });
  });
});
