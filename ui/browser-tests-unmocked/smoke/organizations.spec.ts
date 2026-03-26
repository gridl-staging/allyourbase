import {
  test,
  expect,
  cleanupOrganizationDashboardSmokeOrg,
  seedOrganizationDashboardSmokeOrg,
  waitForDashboard,
} from "../fixtures";
import type { Page, Response } from "@playwright/test";

const ORG_LIST_PATH = "/api/admin/orgs";

async function waitForOrgListResponse(page: Page): Promise<Response> {
  return page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === ORG_LIST_PATH && response.request().method() === "GET";
  });
}

async function assertOrganizationsPageOutcome(
  page: Page,
  orgListResponse: Response,
  seededOrgName: string,
): Promise<void> {
  if (orgListResponse.status() === 503) {
    await expect(page.getByText(/failed to load organizations/i)).toBeVisible();
    return;
  }

  expect(orgListResponse.ok()).toBeTruthy();
  await expect(
    page.getByTestId("org-list-panel").getByRole("button", { name: new RegExp(seededOrgName, "i") }),
  ).toBeVisible();
}

test.describe("Smoke: Organizations Dashboard", () => {
  test("admin can open organizations page and view seeded org row or 503 fallback", async ({
    page,
    request,
    adminToken,
  }) => {
    const runSuffix = Date.now().toString();
    const seededOrg = await seedOrganizationDashboardSmokeOrg(request, adminToken, runSuffix);

    try {
      await page.goto("/admin/");
      await waitForDashboard(page);

      const listResponsePromise = waitForOrgListResponse(page);
      await page.locator("aside").getByRole("button", { name: /^Organizations$/i }).click();
      await expect(page.getByTestId("organizations-view")).toBeVisible({ timeout: 5000 });

      const listResponse = await listResponsePromise;
      await assertOrganizationsPageOutcome(page, listResponse, seededOrg.orgName);
    } finally {
      await cleanupOrganizationDashboardSmokeOrg(request, adminToken, seededOrg.orgId);
    }
  });
});
