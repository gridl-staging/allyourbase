import { test, expect, execSQL, seedRecord } from "../fixtures";

/**
 * FULL E2E TEST: Collections CRUD Operations
 *
 * Tests all CRUD operations via admin UI:
 * - Create record
 * - Read (view) records
 * - Update record
 * - Delete record
 */

test.describe("Collections CRUD (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded record renders in table view", async ({ page, request, adminToken }) => {
    const runId = Date.now();

    pendingCleanup.push("DROP TABLE IF EXISTS crud_test_products;");

    // Arrange: create table and seed a record via API
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE IF NOT EXISTS crud_test_products (
        id SERIAL PRIMARY KEY, name TEXT NOT NULL, price DECIMAL(10,2) NOT NULL,
        stock INTEGER DEFAULT 0, description TEXT, created_at TIMESTAMPTZ DEFAULT NOW()
      );`,
    );
    await seedRecord(request, adminToken, "crud_test_products", {
      name: `Seed Product ${runId}`,
      price: 49.99,
      stock: 5,
    });

    // Act: navigate to the table
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    const sidebar = page.locator("aside");
    await expect(sidebar.getByText("crud_test_products", { exact: true })).toBeVisible({ timeout: 10000 });
    await sidebar.getByText("crud_test_products", { exact: true }).click();

    // Assert: seeded record appears in the table
    await expect(page.getByRole("cell", { name: `Seed Product ${runId}` })).toBeVisible({ timeout: 5000 });

    // Cleanup handled by afterEach
  });

  test("complete CRUD lifecycle via UI", async ({ page, request, adminToken }) => {
    pendingCleanup.push("DROP TABLE IF EXISTS crud_test_products;");

    // ============================================================
    // Setup: Create test table
    // ============================================================
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.getByLabel("SQL query");

    const createTableSQL = `
      CREATE TABLE IF NOT EXISTS crud_test_products (
        id SERIAL PRIMARY KEY,
        name TEXT NOT NULL,
        price DECIMAL(10,2) NOT NULL,
        stock INTEGER DEFAULT 0,
        description TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `;

    await sqlInput.fill(createTableSQL);
    await page.getByRole("button", { name: /run|execute/i }).click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // ============================================================
    // CREATE: Add a new product
    // ============================================================
    await page.reload();
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await expect(sidebar).toBeVisible();

    await expect(sidebar.getByText("crud_test_products", { exact: true })).toBeVisible({ timeout: 10000 });
    await sidebar.getByText("crud_test_products", { exact: true }).click();

    await page.getByRole("button", { name: "New Row" }).click();
    await expect(page.getByText("New Record")).toBeVisible();

    await page.getByLabel("name").fill("Laptop");
    await page.getByLabel("price").fill("999.99");
    await page.getByLabel("stock").fill("10");
    await page.getByLabel("description").fill("High-performance laptop");

    await page.getByRole("button", { name: "Create" }).click();

    // ============================================================
    // READ: Verify product appears in list
    // ============================================================
    await expect(page.getByRole("cell", { name: "Laptop", exact: true })).toBeVisible();
    await expect(page.getByRole("cell", { name: "999.99" })).toBeVisible();

    // ============================================================
    // UPDATE: Edit the product
    // ============================================================
    const laptopRow = page.locator("tr").filter({ hasText: "Laptop" });
    await laptopRow.getByRole("button", { name: /edit/i }).click();

    const priceInput = page.getByLabel("price");
    await priceInput.clear();
    await priceInput.fill("899.99");

    const stockInput = page.getByLabel("stock");
    await stockInput.clear();
    await stockInput.fill("15");

    const saveButton = page.getByRole("button", { name: /save changes|^save$|^update$/i }).first();
    await saveButton.click();

    const laptopRowAfterUpdate = page.locator("tr").filter({ hasText: "Laptop" });
    await expect(laptopRowAfterUpdate.getByRole("cell", { name: "899.99" })).toBeVisible({ timeout: 5000 });

    // Dismiss any open drawer/overlay before proceeding
    await page.keyboard.press("Escape");

    // ============================================================
    // DELETE: Remove the product
    // ============================================================
    const updatedRow = page.locator("tr").filter({ hasText: "Laptop" });
    await updatedRow.getByRole("button", { name: /delete/i }).first().click();

    await expect(page.getByRole("heading", { name: /delete record/i })).toBeVisible({ timeout: 3000 });
    await page.getByRole("button", { name: "Delete", exact: true }).last().click();

    await expect(page.getByRole("cell", { name: "Laptop", exact: true })).not.toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("cell", { name: /no rows/i })).toBeVisible();

    // Cleanup handled by afterEach
  });
});
