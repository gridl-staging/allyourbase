import { test, expect, bootstrapMockedAdminApp, mockAdminEmailTemplateApis } from "./fixtures";

test.describe("Email Templates Preview (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("load-and-verify: seeded templates render and preview content appears", async ({ page }) => {
    await mockAdminEmailTemplateApis(page);

    await page.goto("/admin/");

    await page.getByRole("button", { name: /^Email Templates$/i }).click();

    await expect(page.getByRole("heading", { name: "Email Templates" })).toBeVisible();
    await expect(page.getByText("auth.password_reset")).toBeVisible();
    await expect(page.getByText("app.club_invite")).toBeVisible();

    await expect(page.getByText("Preview for Allyourbase")).toBeVisible({ timeout: 5000 });
  });

  test("shows backend preview validation errors for missing variables", async ({ page }) => {
    await mockAdminEmailTemplateApis(page, {
      previewResponder(request) {
        if (!request.variables.ActionURL) {
          return {
            status: 400,
            body: { message: "missing variable ActionURL" },
          };
        }
        return {
          status: 200,
          body: {
            subject: "ok",
            html: "<p>ok</p>",
            text: "ok",
          },
        };
      },
    });

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Email Templates$/i }).click();

    await expect(page.getByRole("heading", { name: "Email Templates" })).toBeVisible();

    await page.getByLabel("Preview Variables (JSON)").fill(`{
  "AppName": "Sigil"
}`);

    await expect(page.getByText("missing variable ActionURL")).toBeVisible({ timeout: 5000 });
  });

  test("shows client-side JSON parse error and does not send preview request", async ({ page }) => {
    const mockedApis = await mockAdminEmailTemplateApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Email Templates$/i }).click();
    await expect(page.getByRole("heading", { name: "Email Templates" })).toBeVisible();

    await expect(page.getByText("Preview for Allyourbase")).toBeVisible({ timeout: 5000 });
    mockedApis.resetPreviewCalls();

    await page.getByLabel("Preview Variables (JSON)").fill("{");
    await expect(page.getByText("Preview variables must be valid JSON.")).toBeVisible();

    await expect.poll(() => mockedApis.previewCalls, { timeout: 1000, intervals: [200, 400] }).toBe(0);
  });
});
