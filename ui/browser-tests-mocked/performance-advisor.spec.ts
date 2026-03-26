import { test, expect, bootstrapMockedAdminApp, mockPerformanceAdvisorApis } from "./fixtures";

test.describe("Performance Advisor (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("changes range and inspects slow query details", async ({ page }) => {
    await mockPerformanceAdvisorApis(page);
    await page.goto("/admin/");

    await page.getByRole("button", { name: /Performance Advisor/i }).click();
    await expect(page.getByRole("heading", { name: /Performance Advisor/i })).toBeVisible();

    await page.getByLabel(/Time range/i).selectOption("24h");
    await page.getByRole("button", { name: /fp1/i }).click();

    await expect(page.getByText(/select \* from posts where author_id/i)).toBeVisible();
  });

  test("shows error when performance endpoint returns 500", async ({ page }) => {
    await mockPerformanceAdvisorApis(page, {
      reportResponder: () => ({ status: 500, body: { message: "Internal server error" } }),
    });
    await page.goto("/admin/");

    await page.getByRole("button", { name: /Performance Advisor/i }).click();
    await expect(page.getByRole("heading", { name: /Performance Advisor/i })).toBeVisible();
    await expect(page.getByText(/Server error while loading telemetry/i)).toBeVisible();
  });

  test("shows empty state when no slow queries", async ({ page }) => {
    await mockPerformanceAdvisorApis(page, {
      reportResponder: () => ({
        status: 200,
        body: {
          generatedAt: "2026-02-28T00:00:00Z",
          stale: false,
          range: "1h",
          queries: [],
        },
      }),
    });
    await page.goto("/admin/");

    await page.getByRole("button", { name: /Performance Advisor/i }).click();
    await expect(page.getByRole("heading", { name: /Performance Advisor/i })).toBeVisible();
    await expect(page.getByText(/No slow queries/i)).toBeVisible();
  });
});
