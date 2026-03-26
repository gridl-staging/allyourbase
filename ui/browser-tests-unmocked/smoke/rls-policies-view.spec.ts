import { test, expect, probeEndpoint, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: RLS Policies - View
 *
 * Critical Path: Navigate to RLS Policies → verify table list and policy management controls
 */

test.describe("Smoke: RLS Policies", () => {
  test("rls view loads with tables list and policy controls", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/rls");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `RLS service unavailable (status ${probeStatus})`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^RLS Policies$/i }).click();
    await expect(page.getByRole("heading", { name: /^Tables$/i })).toBeVisible({ timeout: 15_000 });

    // Selected-table policy controls should render for the default selected table
    await expect(
      page.getByRole("button", { name: /Enable RLS|Disable RLS/i }),
    ).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: /Add Policy/i })).toBeVisible({ timeout: 5000 });

    // Empty-policy or policy-list states are both valid and meaningful
    const emptyPolicies = page.getByText(/No policies on this table/i);
    const policyAction = page.getByRole("button", { name: /Delete policy|View SQL/i }).first();
    await expect(emptyPolicies.or(policyAction)).toBeVisible({ timeout: 5000 });
  });
});
