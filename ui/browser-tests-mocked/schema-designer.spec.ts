import { expect, test, bootstrapMockedAdminApp, mockSchemaDesignerApis } from "./fixtures";

test.describe("Schema Designer (Browser Mocked)", () => {
  let pageErrors: string[] = [];

  test.beforeEach(async ({ page }) => {
    pageErrors = [];
    page.on("pageerror", (err) => {
      pageErrors.push(String(err));
    });

    await bootstrapMockedAdminApp(page);
  });

  test("explore-table flow shows details", async ({ page }) => {
    await mockSchemaDesignerApis(page);
    await page.goto("/admin/");
    expect(pageErrors).toEqual([]);
    await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });

    await page.getByTestId("nav-schema-designer").click();

    await expect(page.getByRole("heading", { name: /Schema Designer/i })).toBeVisible();
    await page.getByTestId("schema-node-public.posts").click();

    const detailsPanel = page.getByTestId("schema-details-panel");
    await expect(detailsPanel.getByRole("heading", { name: /public.posts/i })).toBeVisible();
    await expect(detailsPanel.getByText("author_id (uuid)", { exact: true })).toBeVisible();
    await expect(detailsPanel.getByText("posts_author_id_fkey", { exact: true })).toBeVisible();
  });

  test("is usable on smaller viewport widths", async ({ page }) => {
    await mockSchemaDesignerApis(page);
    await page.setViewportSize({ width: 900, height: 700 });
    await page.goto("/admin/");

    await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });
    await page.getByTestId("nav-schema-designer").click();
    await expect(page.getByRole("heading", { name: /Schema Designer/i })).toBeVisible();

    await page.getByTestId("schema-node-public.users").click();
    const detailsPanel = page.getByTestId("schema-details-panel");
    await expect(detailsPanel.getByRole("heading", { name: /public.users/i })).toBeVisible();
  });

  test("shows connection error when schema endpoint returns 500", async ({ page }) => {
    await mockSchemaDesignerApis(page, {
      schemaResponder: () => ({ status: 500, body: { message: "Failed to load schema" } }),
    });
    await page.goto("/admin/");

    // /api/schema 500 prevents app boot — app renders connection error page
    await expect(page.getByRole("heading", { name: /Connection Error/i })).toBeVisible();
    await expect(page.getByText(/Failed to load schema/i)).toBeVisible();
  });

  test("shows empty state when schema has no tables", async ({ page }) => {
    await mockSchemaDesignerApis(page, {
      schemaResponder: () => ({
        status: 200,
        body: { schemas: ["public"], builtAt: "2026-02-28T00:00:00Z", tables: {} },
      }),
    });
    await page.goto("/admin/");
    await expect(page.locator("aside")).toBeVisible({ timeout: 10000 });

    await page.getByTestId("nav-schema-designer").click();
    await expect(page.getByText(/No tables available/i)).toBeVisible();
  });
});
