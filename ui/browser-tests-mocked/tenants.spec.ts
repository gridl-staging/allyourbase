import {
  test,
  expect,
  bootstrapMockedAdminApp,
  mockTenantAdminApis,
} from "./fixtures";

test.describe("Tenants (Browser Mocked)", () => {
  test("navigates to tenants page, loads detail panels, and performs suspend transition", async ({
    page,
  }) => {
    await bootstrapMockedAdminApp(page);
    const tenantState = await mockTenantAdminApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Tenants$/i }).click();
    const tenantsView = page.getByTestId("tenants-view");

    await expect(tenantsView.getByTestId("tenant-list-panel")).toBeVisible();
    await expect(page.getByText("Acme Labs")).toBeVisible();
    await expect(page.getByText("Beta Ops")).toBeVisible();

    await page.getByRole("button", { name: /Acme Labs/i }).click();
    const infoSection = page.getByTestId("tenant-info-section");
    await expect(infoSection).toBeVisible();
    await expect(infoSection.getByText("acme-labs")).toBeVisible();

    const suspendButton = page.getByRole("button", { name: "Suspend", exact: true });
    await expect(suspendButton).toBeVisible();
    await suspendButton.click();

    await expect
      .poll(() => tenantState.lifecycleCalls.suspend)
      .toBe(1);
    await expect(page.getByRole("button", { name: "Resume", exact: true })).toBeVisible();
    await expect(page.getByText("suspended").first()).toBeVisible();

    await tenantsView.getByRole("button", { name: "Members", exact: true }).click();
    await expect(page.getByTestId("tenant-members-section")).toBeVisible();

    await tenantsView.getByRole("button", { name: "Maintenance", exact: true }).click();
    await expect(page.getByTestId("tenant-maintenance-section")).toBeVisible();

    await tenantsView.getByRole("button", { name: "Audit", exact: true }).click();
    await expect(page.getByTestId("tenant-audit-section")).toBeVisible();
    await expect(page.getByText("tenant.suspended")).toBeVisible();
  });

  test("shows lifecycle-state action visibility for all five states and maintenance toggle round-trip", async ({
    page,
  }) => {
    await bootstrapMockedAdminApp(page);
    await mockTenantAdminApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Tenants$/i }).click();
    const listPanel = page.getByTestId("tenant-list-panel");

    const lifecycleCases = [
      { tenantName: "Acme Labs", canSuspend: true, canResume: false, canDelete: true },
      { tenantName: "Beta Ops", canSuspend: false, canResume: true, canDelete: true },
      { tenantName: "Gamma Provisioning", canSuspend: false, canResume: false, canDelete: false },
      { tenantName: "Delta Deleting", canSuspend: false, canResume: false, canDelete: false },
      { tenantName: "Echo Deleted", canSuspend: false, canResume: false, canDelete: false },
    ];

    for (const testCase of lifecycleCases) {
      await listPanel.getByRole("button", { name: new RegExp(testCase.tenantName, "i") }).click();
      await expect(page.getByRole("heading", { name: testCase.tenantName })).toBeVisible();
      await expect(page.getByRole("button", { name: "Suspend", exact: true })).toHaveCount(
        testCase.canSuspend ? 1 : 0,
      );
      await expect(page.getByRole("button", { name: "Resume", exact: true })).toHaveCount(
        testCase.canResume ? 1 : 0,
      );
      await expect(page.getByRole("button", { name: "Delete", exact: true })).toHaveCount(
        testCase.canDelete ? 1 : 0,
      );
    }

    await listPanel.getByRole("button", { name: /Acme Labs/i }).click();
    await page.getByRole("button", { name: "Maintenance", exact: true }).click();
    await expect(page.getByRole("button", { name: "Enable Maintenance", exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Enable Maintenance", exact: true }).click();
    await expect(page.getByText("Status: Enabled")).toBeVisible();
    await expect(page.getByRole("button", { name: "Disable Maintenance", exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Disable Maintenance", exact: true }).click();
    await expect(page.getByText("Status: Disabled")).toBeVisible();
    await expect(page.getByRole("button", { name: "Enable Maintenance", exact: true })).toBeVisible();
  });
});
