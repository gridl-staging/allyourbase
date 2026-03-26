import { test, expect, execSQL, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Schema Designer - Seeded Table Visibility
 *
 * Critical Path: Create table → refresh schema → verify table node + detail panel render
 */

test.describe("Smoke: Schema Designer", () => {
  const tableNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (tableNames.length > 0) {
      const tableName = tableNames.pop();
      if (!tableName) continue;
      await execSQL(
        request,
        adminToken,
        `DROP TABLE IF EXISTS ${tableName};`,
      ).catch(() => {});
    }
  });

  test("seeded table appears as a schema node with details", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `smoke_schema_${runId}`;
    tableNames.push(tableName);

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE IF NOT EXISTS ${tableName} (
         id SERIAL PRIMARY KEY,
         label TEXT NOT NULL
       );`,
    );

    for (let attempt = 0; attempt < 3; attempt++) {
      await page.goto("/admin/");
      await waitForDashboard(page);

      const refreshSchema = page.getByRole("button", { name: "Refresh schema" });
      if (await refreshSchema.isVisible({ timeout: 2000 })) {
        await refreshSchema.click();
      }

      await page.getByTestId("nav-schema-designer").click();
      await expect(page.getByRole("heading", { name: /Schema Designer/i })).toBeVisible();

      const tableNode = page.getByTestId(`schema-node-public.${tableName}`);
      if (await tableNode.isVisible({ timeout: 3000 })) {
        await tableNode.click();
        await expect(
          page.getByTestId("schema-details-panel").getByRole("heading", { name: `public.${tableName}` }),
        ).toBeVisible({ timeout: 5000 });
        await expect(page.getByTestId("schema-details-panel").getByText("label (text)", { exact: true })).toBeVisible();
        return;
      }
    }

    throw new Error(`Seeded schema node did not appear for table ${tableName} after retries`);
  });
});
