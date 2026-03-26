import { test, expect, bootstrapMockedAdminApp, mockAdminFDWApis } from "./fixtures";

test.describe("FDW Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("servers-list-500: shows error text", async ({ page }) => {
    await mockAdminFDWApis(page, {
      listServersResponder: () => ({ status: 500, body: { message: "fdw extension not available" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^FDW$/i }).click();

    await expect(page.getByText(/fdw extension not available/i)).toBeVisible();
  });

  test("tables-list-500: shows error text", async ({ page }) => {
    await mockAdminFDWApis(page, {
      listTablesResponder: () => ({ status: 500, body: { message: "table scan failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^FDW$/i }).click();

    await expect(page.getByText(/table scan failed/i)).toBeVisible();
  });

  test("drop-server-500: shows error", async ({ page }) => {
    await mockAdminFDWApis(page, {
      dropServerResponder: () => ({ status: 500, body: { message: "cascade drop failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^FDW$/i }).click();
    await expect(page.getByRole("button", { name: "Drop remote_pg" })).toBeVisible();

    await page.getByRole("button", { name: "Drop remote_pg" }).click();
    await page.getByRole("button", { name: "Drop", exact: true }).click();

    await expect(page.getByText(/cascade drop failed/i)).toBeVisible();
  });

  test("import-tables-500: shows error", async ({ page }) => {
    await mockAdminFDWApis(page, {
      importTablesResponder: () => ({ status: 500, body: { message: "import failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^FDW$/i }).click();
    await expect(page.getByRole("button", { name: "Drop remote_pg" })).toBeVisible();

    await page.getByRole("button", { name: "Import Tables" }).click();
    await page.getByRole("combobox").selectOption("remote_pg");
    await page.getByPlaceholder("Remote schema").fill("public");
    await page.getByRole("button", { name: "Import", exact: true }).click();

    await expect(page.getByText(/import failed/i)).toBeVisible();
  });

  test("empty-state: shows no servers and no tables messages", async ({ page }) => {
    await mockAdminFDWApis(page, {
      listServersResponder: () => ({ status: 200, body: { servers: [] } }),
      listTablesResponder: () => ({ status: 200, body: { tables: [] } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^FDW$/i }).click();

    await expect(page.getByText(/No foreign servers/i)).toBeVisible();
    await expect(page.getByText(/No foreign tables/i)).toBeVisible();
  });
});
