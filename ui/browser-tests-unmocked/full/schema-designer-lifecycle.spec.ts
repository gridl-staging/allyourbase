import { test, expect, execSQL, waitForDashboard } from "../fixtures";
import type { Page } from "@playwright/test";

async function openSchemaDesigner(page: Page): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);

  const refreshSchema = page.getByRole("button", { name: "Refresh schema" });
  if (await refreshSchema.isVisible({ timeout: 2000 })) {
    await refreshSchema.click();
  }

  await page.getByTestId("nav-schema-designer").click();
  await expect(page.getByRole("heading", { name: /Schema Designer/i })).toBeVisible({ timeout: 5000 });
}

test.describe("Schema Designer Lifecycle (Full E2E)", () => {
  const cleanupSQL: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (cleanupSQL.length > 0) {
      const sql = cleanupSQL.pop();
      if (!sql) continue;
      await execSQL(request, adminToken, sql).catch(() => {});
    }
  });

  test("renders seeded nodes, details, zoom controls, and auto-arrange behavior", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const parentTable = `full_schema_parent_${runId}`;
    const childTable = `full_schema_child_${runId}`;
    const fkName = `fk_${childTable}_parent`;
    const idxName = `idx_${childTable}_parent_id`;

    cleanupSQL.push(`DROP TABLE IF EXISTS ${parentTable};`);
    cleanupSQL.push(`DROP TABLE IF EXISTS ${childTable};`);

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE IF NOT EXISTS ${parentTable} (
         id SERIAL PRIMARY KEY,
         name TEXT NOT NULL
       );`,
    );

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE IF NOT EXISTS ${childTable} (
         id SERIAL PRIMARY KEY,
         parent_id INTEGER NOT NULL,
         label TEXT NOT NULL,
         CONSTRAINT ${fkName} FOREIGN KEY (parent_id) REFERENCES ${parentTable}(id)
       );`,
    );

    await execSQL(
      request,
      adminToken,
      `CREATE INDEX IF NOT EXISTS ${idxName} ON ${childTable} (parent_id);`,
    );

    await openSchemaDesigner(page);

    const parentNode = page.getByTestId(`schema-node-public.${parentTable}`);
    const childNode = page.getByTestId(`schema-node-public.${childTable}`);
    await expect(parentNode).toBeVisible({ timeout: 10000 });
    await expect(childNode).toBeVisible({ timeout: 10000 });

    await childNode.click();
    const detailsPanel = page.getByTestId("schema-details-panel");
    await expect(detailsPanel.getByRole("heading", { name: `public.${childTable}` })).toBeVisible({ timeout: 5000 });
    await expect(detailsPanel.getByText("parent_id (integer)", { exact: true })).toBeVisible();
    await expect(detailsPanel.getByText("label (text)", { exact: true })).toBeVisible();
    await expect(detailsPanel.getByText(fkName, { exact: true })).toBeVisible();
    await expect(detailsPanel.getByText(idxName, { exact: true })).toBeVisible();

    const zoomLevel = page.getByTestId("schema-zoom-level");
    const baselineZoom = await zoomLevel.textContent();
    await page.getByRole("button", { name: "Zoom In" }).click();
    await expect(zoomLevel).not.toHaveText(baselineZoom ?? "", { timeout: 5000 });
    await page.getByRole("button", { name: "Zoom Out" }).click();
    await expect(zoomLevel).toHaveText(baselineZoom ?? "", { timeout: 5000 });

    const beforeAutoArrangeStyles = await Promise.all([
      parentNode.getAttribute("style"),
      childNode.getAttribute("style"),
    ]);

    await page.getByRole("button", { name: "Auto Arrange" }).click();
    await expect
      .poll(async () => {
        const afterAutoArrangeStyles = await Promise.all([
          parentNode.getAttribute("style"),
          childNode.getAttribute("style"),
        ]);
        return afterAutoArrangeStyles.some((style, index) => style !== beforeAutoArrangeStyles[index]);
      })
      .toBe(true);

    await expect(page.getByTestId("schema-edges")).toBeVisible();
  });
});
