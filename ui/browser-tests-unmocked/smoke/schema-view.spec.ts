import { test, expect, execSQL, waitForDashboard } from "../fixtures";
import type { Page } from "@playwright/test";

/**
 * SMOKE TEST: Schema View
 *
 * Critical Path: Open seeded table → switch to Schema tab → verify structural schema details
 */

test.describe("Smoke: Schema View", () => {
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

  test("seeded table schema renders columns and indexes", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `smoke_schema_view_${runId}`;
    const rowTitle = `Schema View Row ${runId}`;

    cleanupSQL.push(`DROP TABLE IF EXISTS ${tableName};`);

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY,
        title TEXT NOT NULL,
        created_at TIMESTAMPTZ DEFAULT now()
      );

      INSERT INTO ${tableName} (title) VALUES ('${rowTitle}');`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    await openTableFromSidebar(page, tableName);
    await page.getByRole("button", { name: /^Schema$/i }).click();

    await expect(page.getByRole("heading", { name: "Columns" })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByRole("columnheader", { name: "Name" }).first()).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Type" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Nullable" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Default" })).toBeVisible();

    const schemaTable = page.getByRole("table").first();
    await expect(schemaTable).toContainText("id");
    await expect(schemaTable).toContainText("title");
    await expect(schemaTable).toContainText("created_at");
    await expect(schemaTable).toContainText("text");

    await expect(page.getByRole("heading", { name: "Indexes" })).toBeVisible();
    await expect(page.getByRole("cell", { name: `${tableName}_pkey`, exact: true })).toBeVisible();
  });
});
