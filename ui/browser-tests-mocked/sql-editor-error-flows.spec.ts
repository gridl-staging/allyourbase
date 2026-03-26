import { test, expect, bootstrapMockedAdminApp, mockAdminSqlEditorApis } from "./fixtures";

test.describe("SQL Editor Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("execute-500: shows error message in red panel", async ({ page }) => {
    await mockAdminSqlEditorApis(page, {
      executeResponder: () => ({ status: 500, body: { message: "connection pool exhausted" } }),
    });
    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^SQL Editor$/i }).click();

    await page.getByRole("button", { name: /Execute/i }).click();

    await expect(page.getByText(/connection pool exhausted/i)).toBeVisible();
  });

  test("syntax-error-400: shows error message with query context", async ({ page }) => {
    await mockAdminSqlEditorApis(page, {
      executeResponder: () => ({
        status: 400,
        body: { message: 'relation "nonexistent" does not exist' },
      }),
    });
    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await sqlInput.click();
    await page.keyboard.press("ControlOrMeta+A");
    await page.keyboard.type("SELECT * FROM nonexistent;");

    await page.getByRole("button", { name: /Execute/i }).click();

    await expect(page.getByText(/relation "nonexistent" does not exist/i)).toBeVisible();
    await expect(sqlInput).toContainText("SELECT * FROM nonexistent;");
  });
});
