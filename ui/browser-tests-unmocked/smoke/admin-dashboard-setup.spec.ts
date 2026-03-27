import { test, expect, execSQL, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Admin Dashboard Setup
 *
 * Critical Path:
 * 1. Open dashboard (admin password already set via auth.setup.ts)
 * 2. Verify dashboard UI loads with sidebar sections
 * 3. Create a table via SQL Editor
 * 4. Verify table appears in sidebar
 */

test.describe("Smoke: Admin Dashboard Setup", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("dashboard loads with all sidebar sections", async ({ page }) => {
    // Act: Navigate to admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    // Assert: Dashboard heading visible
    await waitForDashboard(page);

    // Assert: Sidebar sections are present
    const sidebar = page.locator("aside");

    // DATABASE section
    await expect(sidebar.getByRole("button", { name: /^SQL Editor$/i })).toBeVisible();

    // SERVICES section (Storage, Webhooks, etc.)
    // Note: Using flexible matching since exact labels may vary
    await expect(
      sidebar.getByText(/Storage|Webhooks/i).first()
    ).toBeVisible({ timeout: 5000 });

    // ADMIN section
    await expect(
      sidebar.getByText(/Users|API Keys/i).first()
    ).toBeVisible({ timeout: 5000 });
  });

  test("first-run journey creates first table and verifies first row in table data view", async ({
    page,
  }) => {
    const runId = Date.now();
    const tableName = `posts_smoke_${runId}`;
    const rowTitle = `First Post ${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Step 1: Navigate to admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    // Step 2: Click SQL Editor in sidebar
    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    // Step 3: Verify SQL Editor opened
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // Step 4: Create table (single statement per execution)
    const createTableSQL = `CREATE TABLE ${tableName} (
      id SERIAL PRIMARY KEY,
      title TEXT NOT NULL
    )`;

    await sqlInput.fill(createTableSQL);

    // Step 5: Execute CREATE TABLE
    let runButton = page.getByRole("button", { name: /^Execute$/i });
    await expect(runButton).toBeVisible();
    await runButton.click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // Step 6: Sidebar should refresh automatically after CREATE TABLE
    const tableLink = sidebar.getByText(tableName, { exact: true });
    await expect(tableLink).toBeVisible({ timeout: 15000 });

    // Step 7: Insert first row
    const insertSQL = `INSERT INTO ${tableName} (title) VALUES ('${rowTitle}');`;

    await sqlInput.clear();
    await sqlInput.fill(insertSQL);

    runButton = page.getByRole("button", { name: /^Execute$/i });
    await runButton.click();
    await expect(page.getByText(/rows? affected/i).first()).toBeVisible({ timeout: 10000 });

    // Step 8: Click table and verify existing table data view renders the new row
    await tableLink.click();
    await expect(page.getByRole("button", { name: /^Data$/i })).toBeVisible();
    await expect(page.getByRole("cell", { name: rowTitle })).toBeVisible({ timeout: 10000 });

    // Cleanup handled by afterEach
  });

  test("SQL Editor shows query results and duration", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `test_query_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Arrange: Create a simple table via API
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (id SERIAL PRIMARY KEY, name TEXT);
       INSERT INTO ${tableName} (name) VALUES ('Test 1'), ('Test 2');`
    );

    // Act: Navigate to SQL Editor
    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    // Execute a SELECT query
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });
    await sqlInput.fill(`SELECT * FROM ${tableName};`);

    const runButton = page.getByRole("button", { name: /^Execute$/i });
    await runButton.click();

    // Assert: Results should appear
    await expect(page.getByText("Test 1")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("Test 2")).toBeVisible();

    // Assert: Duration should be displayed (in ms or similar)
    await expect(page.getByText(/\d+\s*ms/i).or(page.getByText(/duration/i))).toBeVisible({
      timeout: 5000,
    });

    // Cleanup handled by afterEach
  });
});
