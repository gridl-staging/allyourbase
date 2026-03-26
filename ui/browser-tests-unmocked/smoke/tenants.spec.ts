import {
  test,
  expect,
  cleanupTenantDashboardSmokeTenant,
  seedTenantDashboardSmokeTenant,
  waitForDashboard,
} from "../fixtures";
import type { Page, Response } from "@playwright/test";

const TENANT_LIST_PATH = "/api/admin/tenants";

async function waitForTenantListResponse(page: Page): Promise<Response> {
  return page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === TENANT_LIST_PATH && response.request().method() === "GET";
  });
}

async function assertTenantsPageOutcome(
  page: Page,
  tenantListResponse: Response,
  seededTenantName: string,
): Promise<void> {
  if (tenantListResponse.status() === 503) {
    await expect(page.getByText(/failed to load tenants/i)).toBeVisible();
    return;
  }

  expect(tenantListResponse.ok()).toBeTruthy();
  await expect(page.getByRole("button", { name: new RegExp(seededTenantName, "i") })).toBeVisible();
}

test.describe("Smoke: Tenants Dashboard", () => {
  test("admin can open tenants page and view seeded tenant row or 503 fallback", async ({
    page,
    request,
    adminToken,
  }) => {
    const runSuffix = Date.now().toString();
    const seededTenant = await seedTenantDashboardSmokeTenant(request, adminToken, runSuffix);

    try {
      await page.goto("/admin/");
      await waitForDashboard(page);

      const listResponsePromise = waitForTenantListResponse(page);
      await page.locator("aside").getByRole("button", { name: /^Tenants$/i }).click();
      await expect(page.getByTestId("tenants-view")).toBeVisible({ timeout: 5000 });

      const listResponse = await listResponsePromise;
      await assertTenantsPageOutcome(page, listResponse, seededTenant.tenantName);
    } finally {
      await cleanupTenantDashboardSmokeTenant(request, adminToken, seededTenant.tenantId);
    }
  });
});
