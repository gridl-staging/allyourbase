import { bootstrapMockedAdminApp, expect, test } from "./fixtures";
import { mockOrgAdminApis } from "./fixtures-orgs";

test.describe("Organizations (Browser Mocked)", () => {
  test("navigates organizations page and exercises core org admin flows", async ({ page }) => {
    await bootstrapMockedAdminApp(page);
    const orgState = await mockOrgAdminApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Organizations$/i }).click();

    const orgView = page.getByTestId("organizations-view");
    const listPanel = orgView.getByTestId("org-list-panel");

    await expect(listPanel).toBeVisible();
    await expect(listPanel.getByText("Acme Inc")).toBeVisible();
    await expect(listPanel.getByText("Beta Corp")).toBeVisible();

    await listPanel.getByRole("button", { name: /Acme Inc/i }).click();
    await expect(page.getByRole("heading", { name: "Acme Inc" })).toBeVisible();
    await expect(page.getByText(/2 teams/i)).toBeVisible();

    await orgView.getByRole("button", { name: "Members", exact: true }).click();
    await expect(page.getByTestId("org-members-section")).toBeVisible();

    await page.getByLabel("New Member User ID").fill("u-9");
    await page.getByLabel("New Member Role").selectOption("viewer");
    await page.getByRole("button", { name: "Add Member", exact: true }).click();

    await expect.poll(() => orgState.addOrgMemberCalls).toBe(1);
    await expect(page.getByTestId("org-member-row-u-9")).toBeVisible();

    await page
      .getByTestId("org-member-row-u-1")
      .getByRole("button", { name: "Remove", exact: true })
      .click();

    await expect.poll(() => orgState.lastOwnerProtectionHits).toBe(1);
    await expect(page.getByText(/cannot remove last owner/i)).toBeVisible();

    await orgView.getByRole("button", { name: "Teams", exact: true }).click();
    await expect(page.getByTestId("org-teams-section")).toBeVisible();

    await page.getByLabel("Team Name").fill("QA");
    await page.getByLabel("Team Slug").fill("qa");
    await page.getByRole("button", { name: "Create Team", exact: true }).click();

    await expect.poll(() => orgState.createTeamCalls).toBe(1);
    await expect(page.getByRole("button", { name: /^QA qa$/ })).toBeVisible();

    await orgView.getByRole("button", { name: "Tenants", exact: true }).click();
    await expect(page.getByTestId("org-tenants-section")).toBeVisible();

    await page.getByLabel("Tenant ID").fill("t-9");
    await page.getByRole("button", { name: "Assign Tenant", exact: true }).click();

    await expect.poll(() => orgState.assignTenantCalls).toBe(1);
    await expect(page.getByText("Tenant t-9")).toBeVisible();

    await orgView.getByRole("button", { name: "Usage", exact: true }).click();
    const usageSection = page.getByTestId("org-usage-section");
    await expect(usageSection).toBeVisible();
    await expect(page.getByText("API Requests")).toBeVisible();
    await usageSection.getByLabel("Period").selectOption("week");
    await usageSection.getByRole("button", { name: "Apply Filters", exact: true }).click();

    await expect.poll(() => orgState.lastUsageQuery).toEqual({
      period: "week",
      from: null,
      to: null,
    });

    await usageSection.getByLabel("From", { exact: true }).fill("2026-03-01");
    await usageSection.getByLabel("To", { exact: true }).fill("2026-03-07");
    await usageSection.getByRole("button", { name: "Apply Filters", exact: true }).click();

    await expect.poll(() => orgState.lastUsageQuery).toEqual({
      period: null,
      from: "2026-03-01",
      to: "2026-03-07",
    });

    await orgView.getByRole("button", { name: "Audit", exact: true }).click();
    const auditSection = page.getByTestId("org-audit-section");
    await expect(auditSection).toBeVisible();
    await expect(page.getByText("org.created")).toBeVisible();
    await auditSection.getByLabel("From", { exact: true }).fill("2026-03-01");
    await auditSection.getByLabel("To", { exact: true }).fill("2026-03-10");
    await auditSection.getByLabel("Action").fill("org.member.add");
    await auditSection.getByLabel("Result").fill("success");
    await auditSection.getByLabel("Actor ID").fill("u-2");
    await auditSection.getByRole("button", { name: "Apply Filters", exact: true }).click();

    await expect.poll(() => orgState.lastAuditQuery).toEqual({
      limit: 50,
      offset: 0,
      from: "2026-03-01",
      to: "2026-03-10",
      action: "org.member.add",
      result: "success",
      actorId: "u-2",
    });

    await page.getByRole("button", { name: "Delete", exact: true }).click();

    await expect.poll(() => orgState.deleteConfirmTrueCalls).toBe(1);
    await expect(listPanel.getByRole("button", { name: /Acme Inc/i })).toHaveCount(0);
  });
});
