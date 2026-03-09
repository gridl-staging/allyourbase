import { test, expect, execSQL } from "../fixtures";
import type { Page } from "@playwright/test";

/**
 * SMOKE TEST: Table Browser CRUD Operations
 *
 * Critical Path:
 * 1. Setup: Create table with 3 records via SQL
 * 2. Read: View records in table browser
 * 3. Search: Filter records by text search
 * 4. Create: Add new record via UI
 * 5. Update: Edit record via UI
 * 6. Delete: Remove record via UI
 * 7. Filter: Apply filter expression
 */

test.describe("Smoke: Table Browser CRUD", () => {
  const pendingCleanup: string[] = [];
  async function openTableFromSidebar(page: Page, tableName: string): Promise<void> {
    const sidebar = page.locator("aside");
    const refreshButton = page.getByRole("button", { name: /refresh schema/i });
    const tableLink = sidebar.getByText(tableName, { exact: true });

    await expect(refreshButton).toBeVisible({ timeout: 5000 });
    await expect
      .poll(
        async () => {
          await refreshButton.click();
          return tableLink.isVisible();
        },
        { timeout: 15_000 }
      )
      .toBe(true);

    await tableLink.click();
  }

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded records render in table browser", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `posts_crud_seed_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Arrange: create table and seed records via SQL
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY,
        title TEXT NOT NULL,
        body TEXT,
        status TEXT DEFAULT 'draft'
      );

      INSERT INTO ${tableName} (title, body, status) VALUES
        ('First Post ${runId}', 'Hello World', 'published'),
        ('Second Post ${runId}', 'Testing', 'draft'),
        ('Third Post ${runId}', 'More content', 'published');`
    );

    // Act: navigate to the table
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await openTableFromSidebar(page, tableName);

    // Assert: all 3 seeded records appear
    await expect(page.getByText(`First Post ${runId}`)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(`Second Post ${runId}`)).toBeVisible();
    await expect(page.getByText(`Third Post ${runId}`)).toBeVisible();

    // Cleanup handled by afterEach
  });

  test("search filters records in table browser", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `posts_search_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Arrange: create table with distinct searchable records
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY,
        title TEXT NOT NULL
      );

      INSERT INTO ${tableName} (title) VALUES
        ('Unique Search Term ${runId}'),
        ('Different Content ${runId}'),
        ('Another Entry ${runId}');`
    );

    // Act: navigate to table
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await openTableFromSidebar(page, tableName);

    // Verify all records visible initially
    await expect(page.getByText(`Unique Search Term ${runId}`)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(`Different Content ${runId}`)).toBeVisible();

    // Search for specific term
    const searchBox = page.getByPlaceholder(/search/i).or(page.getByRole("searchbox"));
    await expect(searchBox).toBeVisible({ timeout: 5000 });
    await searchBox.fill("Unique Search");
    await searchBox.press("Enter");

    // Assert: only matching record visible
    await expect(page.getByText(`Unique Search Term ${runId}`)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(`Different Content ${runId}`)).not.toBeVisible();

    // Clear search
    await searchBox.clear();
    await searchBox.press("Enter");

    // Assert: all records visible again
    await expect(page.getByText(`Different Content ${runId}`)).toBeVisible({ timeout: 5000 });

    // Cleanup handled by afterEach
  });

  test("complete CRUD flow: create, update, delete", async ({ page, request, adminToken }) => {
    test.setTimeout(90_000); // Extended timeout for multi-step test with schema refresh

    const runId = Date.now();
    const tableName = `posts_crud_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Arrange: create table via API
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY,
        title TEXT NOT NULL,
        body TEXT,
        status TEXT DEFAULT 'draft'
      );

      INSERT INTO ${tableName} (title, body, status) VALUES
        ('Existing Post ${runId}', 'Initial content', 'published');`
    );

    // Navigate to table
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await openTableFromSidebar(page, tableName);

    // ============================================================
    // CREATE: Add new record via UI
    // ============================================================

    // Click "New Row" button
    const newButton = page.getByRole("button", { name: /New Row|New Record|Create/i });
    await expect(newButton).toBeVisible({ timeout: 5000 });
    await newButton.click();

    // Wait for form modal/drawer to open
    await expect(page.getByRole("heading", { name: /New Record/i })).toBeVisible({
      timeout: 5000,
    });

    // Fill in form fields - use more flexible selectors
    const titleInput = page.getByLabel(/title/i);
    const bodyInput = page.getByLabel(/body/i);
    const statusInput = page.getByLabel(/status/i);

    await expect(titleInput).toBeVisible({ timeout: 5000 });
    await titleInput.fill(`Fourth Post ${runId}`);
    await bodyInput.fill("Created via dashboard");
    await statusInput.fill("published");

    // Submit form
    const createButton = page.getByRole("button", { name: /^Create$/i });
    await createButton.click();

    // Verify record appears in table
    await expect(page.getByText(`Fourth Post ${runId}`)).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Created via dashboard")).toBeVisible();

    // ============================================================
    // UPDATE: Edit record via UI
    // ============================================================

    // Find the row containing our new record
    const targetRow = page.locator("tr").filter({ hasText: `Fourth Post ${runId}` }).first();
    await expect(targetRow).toBeVisible({ timeout: 5000 });

    // Click the Edit button within the row
    const editButton = targetRow.getByRole("button", { name: /edit/i }).first();
    await editButton.click();

    // Wait for edit form modal to appear
    await expect(page.getByRole("heading", { name: /edit/i })).toBeVisible({ timeout: 5000 });

    // Find and modify fields in the edit modal
    const editTitleInput = page.getByLabel(/title/i);
    const editBodyInput = page.getByLabel(/body/i);

    await expect(editTitleInput).toBeVisible({ timeout: 5000 });
    await editTitleInput.clear();
    await editTitleInput.fill(`Fourth Post (Edited) ${runId}`);
    await editBodyInput.clear();
    await editBodyInput.fill("Updated content");

    // Save changes
    const saveButton = page.getByRole("button", { name: /Save|Update/i });
    await saveButton.click();

    // Verify updated content appears
    await expect(page.getByText(`Fourth Post (Edited) ${runId}`)).toBeVisible({
      timeout: 10000,
    });
    await expect(page.getByText("Updated content")).toBeVisible();

    // ============================================================
    // DELETE: Remove record via UI
    // ============================================================

    // Find the edited row and click delete button directly (like webhooks/storage tests)
    const editedRow = page.locator("tr").filter({ hasText: `Fourth Post (Edited) ${runId}` }).first();
    await expect(editedRow).toBeVisible({ timeout: 5000 });

    // Click delete button within the row (don't click row first - causes modal overlay issues)
    const deleteButton = editedRow.getByRole("button", { name: /delete/i });
    await expect(deleteButton).toBeVisible({ timeout: 5000 });
    await deleteButton.click();

    // Check if confirmation modal appears (some UIs may delete immediately)
    const confirmButton = page.getByRole("button", { name: /^(delete|confirm|yes)$/i }).last();
    const isConfirmVisible = await confirmButton.isVisible({ timeout: 2000 }).catch(() => false);

    if (isConfirmVisible) {
      // Confirmation modal exists, click it
      await confirmButton.click();
    }

    // Verify record is gone from table
    await expect(page.getByText(`Fourth Post (Edited) ${runId}`)).not.toBeVisible({
      timeout: 10000,
    });

    // Verify original record still exists
    await expect(page.getByText(`Existing Post ${runId}`)).toBeVisible();

    // Cleanup handled by afterEach
  });

  test("filter records by status field", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `posts_filter_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Arrange: create table with different statuses
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY,
        title TEXT NOT NULL,
        status TEXT NOT NULL
      );

      INSERT INTO ${tableName} (title, status) VALUES
        ('Published Post 1 ${runId}', 'published'),
        ('Draft Post 1 ${runId}', 'draft'),
        ('Published Post 2 ${runId}', 'published');`
    );

    // Navigate to table
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await openTableFromSidebar(page, tableName);

    // Verify all records visible initially
    await expect(page.getByText(`Published Post 1 ${runId}`)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(`Draft Post 1 ${runId}`)).toBeVisible();
    await expect(page.getByText(`Published Post 2 ${runId}`)).toBeVisible();

    // Apply filter
    const filterButton = page.getByRole("button", { name: /filter/i });
    await expect(filterButton).toBeVisible({ timeout: 5000 });
    await filterButton.click();

    // Wait for filter input to appear
    const filterInput = page.getByPlaceholder(/filter|expression/i).or(
      page.getByLabel(/filter/i)
    );
    await expect(filterInput).toBeVisible({ timeout: 5000 });

    // Enter filter expression
    await filterInput.fill("status='published'");

    // Apply filter
    const applyButton = page.getByRole("button", { name: /apply/i });
    await applyButton.click();

    // Assert: only published posts visible
    await expect(page.getByText(`Published Post 1 ${runId}`)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(`Published Post 2 ${runId}`)).toBeVisible();
    await expect(page.getByText(`Draft Post 1 ${runId}`)).not.toBeVisible();

    // Clear filter
    const clearButton = page
      .getByRole("button", { name: /clear|reset/i })
      .or(page.getByLabel(/clear filter/i));

    if (await clearButton.isVisible({ timeout: 2000 })) {
      await clearButton.click();

      // Verify all records visible again
      await expect(page.getByText(`Draft Post 1 ${runId}`)).toBeVisible({ timeout: 5000 });
    }

    // Cleanup handled by afterEach
  });
});
