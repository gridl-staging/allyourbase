import { test, expect, execSQL, waitForDashboard } from "../fixtures";
import type { Page } from "@playwright/test";

/**
 * FULL E2E TEST: SQL View Lifecycle (table-context SQL tab)
 *
 * Critical Path: Seed table → open table in browser → switch to SQL tab →
 * INSERT via SQL → verify row count → UPDATE via SQL → SELECT to verify →
 * DELETE via SQL → verify empty
 */

test.describe("SQL View Lifecycle (Full E2E)", () => {
  const cleanupSQL: string[] = [];

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
        { timeout: 15000 },
      )
      .toBe(true);

    await tableLink.click();
  }

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of cleanupSQL) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    cleanupSQL.length = 0;
  });

  test("full DML lifecycle via table-context SQL tab", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `full_sql_view_${runId}`;

    cleanupSQL.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Seed table with one row
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY,
        label TEXT NOT NULL,
        amount INT DEFAULT 0
      )`,
    );
    await execSQL(
      request,
      adminToken,
      `INSERT INTO ${tableName} (label, amount) VALUES ('seed-row', 100)`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    // Open table and switch to SQL tab
    await openTableFromSidebar(page, tableName);
    await page.getByRole("button", { name: /^SQL$/i }).click();

    const sqlEditor = page.getByLabel("SQL query");
    await expect(sqlEditor).toBeVisible({ timeout: 5000 });

    // INSERT additional rows via SQL tab
    await sqlEditor.click();
    await page.keyboard.press("ControlOrMeta+A");
    await page.keyboard.type(
      `INSERT INTO ${tableName} (label, amount) VALUES ('row-a', 200), ('row-b', 300);`,
    );
    await page.getByRole("button", { name: /^Execute$/i }).click();

    await expect(page.getByText(/2 row.*affected/i)).toBeVisible({ timeout: 5000 });

    // UPDATE via SQL tab
    await sqlEditor.click();
    await page.keyboard.press("ControlOrMeta+A");
    await page.keyboard.type(
      `UPDATE ${tableName} SET amount = 999 WHERE label = 'seed-row';`,
    );
    await page.getByRole("button", { name: /^Execute$/i }).click();

    await expect(page.getByText(/1 row.*affected/i)).toBeVisible({ timeout: 5000 });

    // SELECT to verify all 3 rows and the updated value
    await sqlEditor.click();
    await page.keyboard.press("ControlOrMeta+A");
    await page.keyboard.type(
      `SELECT label, amount FROM ${tableName} ORDER BY amount;`,
    );
    await page.getByRole("button", { name: /^Execute$/i }).click();

    await expect(page.getByRole("columnheader", { name: "label" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: "amount" })).toBeVisible();
    await expect(page.getByText(/3 row/i)).toBeVisible();
    await expect(page.getByRole("cell", { name: "row-a" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "999" })).toBeVisible();

    // DELETE via SQL tab
    await sqlEditor.click();
    await page.keyboard.press("ControlOrMeta+A");
    await page.keyboard.type(
      `DELETE FROM ${tableName} WHERE label IN ('row-a', 'row-b');`,
    );
    await page.getByRole("button", { name: /^Execute$/i }).click();

    await expect(page.getByText(/2 row.*affected/i)).toBeVisible({ timeout: 5000 });

    // Final SELECT to verify only the seed row remains
    await sqlEditor.click();
    await page.keyboard.press("ControlOrMeta+A");
    await page.keyboard.type(
      `SELECT label, amount FROM ${tableName};`,
    );
    await page.getByRole("button", { name: /^Execute$/i }).click();

    await expect(page.getByText(/1 row/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("cell", { name: "seed-row" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "999" })).toBeVisible();
  });
});
