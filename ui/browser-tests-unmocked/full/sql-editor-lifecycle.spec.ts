import { test, expect, execSQL, waitForDashboard } from "../fixtures";

/**
 * FULL E2E TEST: SQL Editor Lifecycle
 *
 * Critical Path: Navigate to admin SQL Editor → execute DDL (CREATE TABLE) →
 * execute DML (INSERT) → execute SELECT and verify result table → execute DROP TABLE
 */

test.describe("SQL Editor Lifecycle (Full E2E)", () => {
  const tablesToDrop: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (tablesToDrop.length > 0) {
      const table = tablesToDrop.pop();
      if (!table) continue;
      await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${table}`).catch(() => {});
    }
  });

  test("execute DDL, DML, SELECT, and DROP via admin SQL Editor", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `_test_sql_editor_${runId}`;
    tablesToDrop.push(tableName);

    // Navigate to SQL Editor
    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // DDL: CREATE TABLE
    await sqlInput.fill(`CREATE TABLE ${tableName} (id serial PRIMARY KEY, name text NOT NULL, value int);`);
    await page.getByRole("button", { name: /Execute/i }).click();

    // Verify DDL success feedback
    await expect(page.getByText(/Statement executed successfully/i)).toBeVisible({ timeout: 5000 });

    // DML: INSERT rows
    await sqlInput.fill(`INSERT INTO ${tableName} (name, value) VALUES ('alpha', 10), ('beta', 20), ('gamma', 30);`);
    await page.getByRole("button", { name: /Execute/i }).click();

    // Verify DML row count feedback
    await expect(page.getByText(/3 row.*affected/i)).toBeVisible({ timeout: 5000 });

    // SELECT: query the rows back
    await sqlInput.fill(`SELECT name, value FROM ${tableName} ORDER BY value;`);
    await page.getByRole("button", { name: /Execute/i }).click();

    // Verify result table headers
    await expect(page.getByRole("columnheader", { name: "name" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: "value" })).toBeVisible();

    // Verify result table data
    await expect(page.getByRole("cell", { name: "alpha" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "beta" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "gamma" })).toBeVisible();
    await expect(page.getByText(/3 row/i)).toBeVisible();

    // DDL: DROP TABLE
    await sqlInput.fill(`DROP TABLE ${tableName};`);
    await page.getByRole("button", { name: /Execute/i }).click();

    await expect(page.getByText(/Statement executed successfully/i)).toBeVisible({ timeout: 5000 });

    // Verify table is gone by querying it
    await sqlInput.fill(`SELECT * FROM ${tableName};`);
    await page.getByRole("button", { name: /Execute/i }).click();

    // Expect an error since the table was dropped
    await expect(page.getByText(new RegExp(`relation.*${tableName}.*does not exist|not found`, "i"))).toBeVisible({ timeout: 5000 });

    // Table already dropped — remove from cleanup list
    const idx = tablesToDrop.indexOf(tableName);
    if (idx !== -1) tablesToDrop.splice(idx, 1);
  });
});
