import { test, expect, cleanupUsageMeteringTenant, seedUsageMeteringTenantDailyRows, waitForDashboard } from "../fixtures";
import type { Page, Response } from "@playwright/test";

const USAGE_LIST_PATH = "/api/admin/usage";

async function waitForUsageListResponse(page: Page): Promise<Response> {
  return page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === USAGE_LIST_PATH && response.request().method() === "GET";
  });
}

async function assertUsagePageOutcome(
  page: Page,
  usageResponse: Response,
  seededTenantName: string,
): Promise<void> {
  if (usageResponse.status() === 503) {
    await expect(page.getByText(/usage .* not configured|usage aggregation service not configured/i)).toBeVisible();
    return;
  }

  expect(usageResponse.ok()).toBeTruthy();
  await expect(page.getByRole("cell", { name: seededTenantName })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Usage Trend" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Usage Breakdown" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Tenant Limits" })).toBeVisible();
}

test.describe("Smoke: Usage Metering", () => {
  test("admin can open usage dashboard and view seeded tenant row or 503 fallback", async ({
    page,
    request,
    adminToken,
  }) => {
    const runSuffix = Date.now().toString();
    const seededTenant = await seedUsageMeteringTenantDailyRows(request, adminToken, runSuffix);

    try {
      await page.goto("/admin/");
      await waitForDashboard(page);

      const usageResponsePromise = waitForUsageListResponse(page);
      await page.locator("aside").getByRole("button", { name: /^Usage$/i }).click();
      await expect(page.getByRole("heading", { name: /Usage Metering/i })).toBeVisible({ timeout: 15_000 });

      const usageResponse = await usageResponsePromise;
      await assertUsagePageOutcome(page, usageResponse, seededTenant.tenantName);
    } finally {
      await cleanupUsageMeteringTenant(request, adminToken, seededTenant.tenantId);
    }
  });
});
