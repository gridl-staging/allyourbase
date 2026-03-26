import { randomUUID } from "crypto";
import { test, expect, execSQL, waitForDashboard, getOrganizationById } from "../fixtures";
import type { Page } from "@playwright/test";

function sqlLiteral(value: string): string {
  return value.replace(/'/g, "''");
}

async function openOrganizationsPage(page: Page): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page
    .locator("aside")
    .getByRole("button", { name: /^Organizations$/i })
    .click();
  await expect(page.getByTestId("organizations-view")).toBeVisible({ timeout: 5000 });
}

test.describe("Organizations Lifecycle (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("create, update, and delete organization", async ({ page, request, adminToken }) => {
    const runID = randomUUID().replace(/-/g, "").slice(0, 10);
    const orgName = `Lifecycle Org ${runID}`;
    const orgSlug = `org-lifecycle-${runID}`;
    const updatedOrgName = `Lifecycle Org Updated ${runID}`;
    const updatedOrgSlug = `org-lifecycle-updated-${runID}`;

    pendingCleanup.push(
      `DELETE FROM _ayb_organizations WHERE slug = '${sqlLiteral(orgSlug)}' OR slug = '${sqlLiteral(updatedOrgSlug)}'`,
    );

    await openOrganizationsPage(page);

    await page.getByLabel("Create Org Name").fill(orgName);
    await page.getByLabel("Create Org Slug").fill(orgSlug);
    await page.getByLabel("Create Org Plan Tier").selectOption("pro");
    await page.getByRole("button", { name: "Create Org" }).click();

    await expect(page.getByRole("heading", { name: orgName })).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("org-info-section")).toBeVisible({ timeout: 5000 });

    const createdOrgResult = await execSQL(
      request,
      adminToken,
      `SELECT id FROM _ayb_organizations WHERE slug = '${sqlLiteral(orgSlug)}'`,
    );
    const orgID = createdOrgResult.rows[0]?.[0];
    if (typeof orgID !== "string") {
      throw new Error(`Expected org id for slug ${orgSlug}`);
    }
    pendingCleanup.push(`DELETE FROM _ayb_organizations WHERE id = '${sqlLiteral(orgID)}'`);

    await page.getByLabel("Organization Name").fill(updatedOrgName);
    await page.getByLabel("Organization Slug").fill(updatedOrgSlug);
    await page.getByRole("button", { name: "Save Info" }).click();
    await expect(page.getByRole("heading", { name: updatedOrgName })).toBeVisible({
      timeout: 5000,
    });

    const updatedOrgResult = await execSQL(
      request,
      adminToken,
      `SELECT name, slug FROM _ayb_organizations WHERE id = '${sqlLiteral(orgID)}'`,
    );
    expect(updatedOrgResult.rows[0]?.[0]).toBe(updatedOrgName);
    expect(updatedOrgResult.rows[0]?.[1]).toBe(updatedOrgSlug);

    await page.getByRole("button", { name: "Delete" }).click();
    await expect(page.getByRole("heading", { name: updatedOrgName })).not.toBeVisible({
      timeout: 5000,
    });

    const deletedOrgResult = await getOrganizationById(request, adminToken, orgID);
    expect(deletedOrgResult.status).toBe(404);
  });
});
