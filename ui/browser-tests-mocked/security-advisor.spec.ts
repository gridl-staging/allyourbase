import { test, expect, bootstrapMockedAdminApp, mockSecurityAdvisorApis } from "./fixtures";

test.describe("Security Advisor (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("filters and inspects recommendation details", async ({ page }) => {
    await mockSecurityAdvisorApis(page);
    await page.goto("/admin/");

    await page.getByRole("button", { name: /Security Advisor/i }).click();
    await expect(page.getByRole("heading", { name: /Security Advisor/i })).toBeVisible();

    await page.getByLabel(/Severity/i).selectOption("critical");
    await page.getByRole("button", { name: /RLS disabled on public.posts/i }).click();

    await expect(page.getByText(/Enable RLS and add restrictive policy/i)).toBeVisible();
  });

  test("shows error when security endpoint returns 500", async ({ page }) => {
    await mockSecurityAdvisorApis(page, {
      reportResponder: () => ({ status: 500, body: { message: "Internal server error" } }),
    });
    await page.goto("/admin/");

    await page.getByRole("button", { name: /Security Advisor/i }).click();
    await expect(page.getByRole("heading", { name: /Security Advisor/i })).toBeVisible();
    await expect(page.getByText(/Server error while loading telemetry/i)).toBeVisible();
  });

  test("shows empty state when no findings", async ({ page }) => {
    await mockSecurityAdvisorApis(page, {
      reportResponder: () => ({
        status: 200,
        body: {
          evaluatedAt: "2026-02-28T00:00:00Z",
          stale: false,
          findings: [],
        },
      }),
    });
    await page.goto("/admin/");

    await page.getByRole("button", { name: /Security Advisor/i }).click();
    await expect(page.getByRole("heading", { name: /Security Advisor/i })).toBeVisible();
    await expect(page.getByText(/No findings/i)).toBeVisible();
  });
});
