import { test, expect, execSQL } from "../fixtures";

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

    // Assert: Dashboard heading visible
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

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

  test("create table via SQL Editor and verify in sidebar", async ({ page }) => {
    const runId = Date.now();
    const tableName = `posts_smoke_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Step 1: Navigate to admin dashboard
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    // Step 2: Click SQL Editor in sidebar
    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    // Step 3: Verify SQL Editor opened
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // Step 4: Create posts table (UI doesn't support multi-statement SQL)
    const createTableSQL = `
      CREATE TABLE ${tableName} (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        title TEXT NOT NULL,
        body TEXT,
        author_id UUID,
        status TEXT DEFAULT 'draft',
        created_at TIMESTAMPTZ DEFAULT NOW()
      )`;

    await sqlInput.fill(createTableSQL);

    // Step 5: Execute CREATE TABLE
    let runButton = page.getByRole("button", { name: /run|execute/i });
    await expect(runButton).toBeVisible();
    await runButton.click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // Step 7: Insert sample data (separate statement)
    const insertSQL = `
      INSERT INTO ${tableName} (title, body, status) VALUES
        ('First Post', 'Hello World', 'published'),
        ('Second Post', 'Testing', 'draft'),
        ('Third Post', 'More content', 'published')`;

    await sqlInput.clear();
    await sqlInput.fill(insertSQL);

    runButton = page.getByRole("button", { name: /run|execute/i });
    await runButton.click();
    await expect(page.getByText(/rows? affected|statement executed successfully/i).first()).toBeVisible({
      timeout: 10000,
    });

    const refreshButton = page.getByRole("button", { name: /refresh schema/i });
    await expect(refreshButton).toBeVisible({ timeout: 5000 });
    await refreshButton.click({ timeout: 15000 });

    // Step 10: Wait for table to appear in sidebar (more reliable than button state)
    const tableLink = sidebar.getByText(tableName, { exact: true });
    await expect(tableLink).toBeVisible({ timeout: 15000 });

    // Step 11: Click table to verify it's navigable
    await tableLink.click();

    // Step 12: Verify we're on the table view page
    // Should see the table data we inserted
    await expect(page.getByText("First Post")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("Second Post")).toBeVisible();
    await expect(page.getByText("Third Post")).toBeVisible();

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
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    // Execute a SELECT query
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });
    await sqlInput.fill(`SELECT * FROM ${tableName};`);

    const runButton = page.getByRole("button", { name: /run|execute/i });
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
