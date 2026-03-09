import { test, expect, execSQL, seedRecord } from "../fixtures";

/**
 * FULL E2E TEST: Table Browser Advanced Features
 *
 * Tests advanced data browsing features:
 * - Setup: Create table with sample data via SQL
 * - Filter records (advanced filter syntax)
 * - Sort by column header
 * - Edit row via row action button
 * - Cleanup: Drop test table
 */

test.describe("Table Browser Advanced (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded records render in table view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `adv_seed_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Arrange: create table and seed records
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY, name TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'active', score INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );`,
    );
    await seedRecord(request, adminToken, tableName, { name: `Seed Alpha ${runId}`, status: "active", score: 90 });
    await seedRecord(request, adminToken, tableName, { name: `Seed Beta ${runId}`, status: "inactive", score: 30 });

    // Act: navigate to the table
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    const sidebar = page.locator("aside");
    const tableLink = sidebar.getByText(tableName, { exact: true });
    await expect(tableLink).toBeVisible({ timeout: 10000 });
    await tableLink.click();

    // Assert: seeded records appear in the table
    await expect(page.getByText(`Seed Alpha ${runId}`)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(`Seed Beta ${runId}`)).toBeVisible();

    // Cleanup handled by afterEach
  });

  test("filter, sort, and edit row via table browser", async ({ page }) => {
    // This test has many steps (setup, filter, sort, export, detail, batch delete, cleanup).
    // Increase timeout to avoid flaky timeouts during cleanup.
    test.setTimeout(60_000);
    const runId = Date.now();
    const tableName = `adv_test_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // ============================================================
    // Setup: Create table with sample data via SQL
    // ============================================================
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    const sidebar = page.locator("aside");

    // Navigate to SQL Editor via sidebar
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // Create table first (separate execution to avoid multi-statement issues)
    await sqlInput.fill(`
      CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY,
        name TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'active',
        score INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);
    await page.getByRole("button", { name: /run|execute/i }).click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // Insert 5 records (separate SQL execution)
    await sqlInput.fill(`
      INSERT INTO ${tableName} (name, status, score) VALUES
        ('Alpha Product', 'active', 95),
        ('Beta Service', 'inactive', 42),
        ('Gamma Tool', 'active', 78),
        ('Delta Widget', 'active', 88),
        ('Epsilon Device', 'inactive', 15);
    `);
    await page.getByRole("button", { name: /run|execute/i }).click();
    await expect(page.getByText(/rows? affected/i)).toBeVisible({ timeout: 10000 });

    // Reload to see new table
    await page.reload();
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    // Navigate to the test table
    const tableLink = sidebar.getByText(tableName, { exact: true });
    await expect(tableLink).toBeVisible({ timeout: 10000 });
    await tableLink.click();

    // Verify Data tab is active and records are visible
    await expect(page.getByText("Alpha Product")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("Beta Service")).toBeVisible();

    // ============================================================
    // FILTER: Apply advanced filter
    // ============================================================
    const filterInput = page.getByPlaceholder(/filter/i);
    await expect(filterInput).toBeVisible({ timeout: 3000 });
    await filterInput.fill("status='active'");

    const applyButton = page.getByRole("button", { name: /apply/i });
    await expect(applyButton).toBeVisible();
    await applyButton.click();

    // Verify only active records visible
    await expect(page.getByText("Alpha Product")).toBeVisible({ timeout: 3000 });
    await expect(page.getByText("Gamma Tool")).toBeVisible();
    // Inactive records should not be visible
    await expect(page.getByText("Beta Service")).not.toBeVisible({ timeout: 2000 });

    // Clear filter
    await filterInput.clear();
    await applyButton.click();

    // All records visible again
    await expect(page.getByText("Beta Service")).toBeVisible({ timeout: 3000 });

    // ============================================================
    // SORT: Click column header to sort ascending
    // ============================================================
    const nameHeader = page.getByRole("columnheader", { name: "name" });
    await expect(nameHeader).toBeVisible({ timeout: 2000 });
    await nameHeader.click();

    // Verify sort ORDER — first data row must be "Alpha Product" (alphabetically first)
    // Alpha < Beta < Delta < Epsilon < Gamma
    const rows = page.locator("tr").filter({ has: page.getByRole("cell") });
    await expect(rows.first()).toContainText("Alpha Product", { timeout: 3000 });
    await expect(rows.nth(4)).toContainText("Gamma Tool");

    // Click again to reverse sort (descending)
    await nameHeader.click();
    // Now Gamma should be first and Alpha last
    await expect(rows.first()).toContainText("Gamma Tool", { timeout: 3000 });
    await expect(rows.nth(4)).toContainText("Alpha Product");

    // ============================================================
    // ROW INTERACTION: Click edit button on a row
    // ============================================================
    const alphaRow = page.locator("tr").filter({ hasText: "Alpha Product" });
    await expect(alphaRow).toBeVisible({ timeout: 2000 });

    // Click the edit button — must exist on every data row
    const editBtn = alphaRow.getByRole("button", { name: /edit/i });
    await expect(editBtn).toBeVisible({ timeout: 2000 });
    await editBtn.click();

    // Verify edit form/drawer opened — check for a form label that only appears in the edit drawer
    await expect(page.getByLabel("name")).toBeVisible({ timeout: 2000 });

    // Dismiss the drawer before cleanup navigation
    await page.keyboard.press("Escape");

    // ============================================================
    // Cleanup: Drop test table via SQL
    // ============================================================
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();
    const cleanupSql = page.getByLabel("SQL query");
    await expect(cleanupSql).toBeVisible({ timeout: 5000 });
    await cleanupSql.fill(`DROP TABLE IF EXISTS ${tableName};`);
    await page.getByRole("button", { name: /run|execute/i }).click();

  });
});
