import { test, expect, execSQL } from "../fixtures";

/**
 * SMOKE TEST: Collections - Create Record
 *
 * Critical Path: Navigate to table → Create new record → Verify appears in list
 */

test.describe("Smoke: Collections Create", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded record renders in table view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = "smoke_test_records";

    // Register cleanup early so afterEach runs it even on failure
    pendingCleanup.push(`DELETE FROM ${tableName} WHERE name = 'Seed User ${runId}';`);

    // Arrange: create table and seed a record via SQL
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE IF NOT EXISTS ${tableName} (id SERIAL PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL, created_at TIMESTAMPTZ DEFAULT NOW());`,
    );
    await execSQL(
      request,
      adminToken,
      `INSERT INTO ${tableName} (name, email) VALUES ('Seed User ${runId}', 'seed-${runId}@test.com');`,
    );

    // Act: navigate to the table
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    const sidebar = page.locator("aside");
    const tableLink = sidebar.getByText(tableName, { exact: true });
    await expect(tableLink).toBeVisible({ timeout: 5000 });
    await tableLink.click();

    // Assert: seeded record appears in the table
    await expect(page.getByRole("cell", { name: `Seed User ${runId}` })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("cell", { name: `seed-${runId}@test.com` })).toBeVisible();

    // Cleanup handled by afterEach
  });

  test("create test table and add record via UI", async ({ page }) => {
    const runId = Date.now();

    // Register cleanup early so afterEach runs it even on failure
    pendingCleanup.push(`DELETE FROM smoke_test_records WHERE name = 'Smoke User ${runId}';`);

    // Step 1: Navigate to admin dashboard
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    // Step 2: Navigate to SQL Editor via sidebar
    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    // Step 3: Create a simple test table via SQL
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    const createTableSQL = `CREATE TABLE IF NOT EXISTS smoke_test_records (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  email TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);`;

    await sqlInput.fill(createTableSQL);
    const runButton = page.getByRole("button", { name: /run|execute/i });
    await runButton.click();

    // Step 4: Reload to see new table in sidebar
    await page.reload();
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    // Step 5: Click on the new table in sidebar
    const tableLink = sidebar.getByText("smoke_test_records", { exact: true });
    await expect(tableLink).toBeVisible({ timeout: 5000 });
    await tableLink.click();

    // Step 6: Click "New Row" button to create record
    const newButton = page.getByRole("button", { name: "New Row" });
    await expect(newButton).toBeVisible({ timeout: 5000 });
    await newButton.click();

    // Step 7: Verify form opened
    await expect(page.getByText("New Record")).toBeVisible();

    // Step 8: Fill in form fields
    await page.getByLabel("name").fill(`Smoke User ${runId}`);
    await page.getByLabel("email").fill(`smoke-${runId}@test.com`);

    // Step 9: Submit form
    await page.getByRole("button", { name: "Create" }).click();

    // Step 10: Verify record appears in table
    await expect(page.getByRole("cell", { name: `Smoke User ${runId}` })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("cell", { name: `smoke-${runId}@test.com` })).toBeVisible();
  });
});
