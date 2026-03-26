import { test, expect, probeEndpoint, waitForDashboard } from "../fixtures";

/**
 * FULL E2E TEST: Backups & PITR Lifecycle
 *
 * Critical Path: Navigate to Backups → trigger backup via UI → verify it appears
 * in backup list → filter by status → verify PITR section renders with target time input
 */

test.describe("Backups Lifecycle (Full E2E)", () => {
  test("trigger backup, verify in list, filter, and inspect PITR section", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/backups");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Backup service unavailable (status ${probeStatus})`,
    );
    const triggerProbeStatus = await probeEndpoint(request, adminToken, "/api/admin/backups", {
      method: "POST",
    });
    test.skip(
      triggerProbeStatus === 503 || triggerProbeStatus === 404 || triggerProbeStatus === 501,
      `Backup trigger unavailable (status ${triggerProbeStatus})`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Backups/i }).click();
    await expect(page.getByRole("heading", { name: /Backups/i })).toBeVisible({ timeout: 5000 });

    // Verify page structure is loaded
    await expect(page.getByText("Manage database backups and point-in-time recovery")).toBeVisible();

    // Trigger a backup via UI
    const triggerButton = page.getByRole("button", { name: /Trigger Backup/i });
    await expect(triggerButton).toBeVisible();
    await triggerButton.click();

    // Verify backup triggered toast or the backup appearing in the list
    const backupResult = page.getByText(/Backup triggered|pending|running/i);
    await expect(backupResult.first()).toBeVisible({ timeout: 10000 });

    // Verify table structure with rows
    await expect(page.getByRole("columnheader", { name: /Status/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Type/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Database/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Size/i })).toBeVisible();

    // Filter by status using the labeled select
    const statusFilter = page.getByLabel("Status");
    await statusFilter.selectOption("completed");

    const applyButton = page.getByRole("button", { name: /Apply/i });
    await applyButton.click();

    // Wait for filter to take effect
    await expect(page.getByRole("columnheader", { name: /Status/i })).toBeVisible({ timeout: 5000 });

    // Reset filters
    const resetButton = page.getByRole("button", { name: /Reset/i });
    await resetButton.click();

    await expect(page.getByRole("columnheader", { name: /Status/i })).toBeVisible({ timeout: 5000 });

    // Verify PITR section renders — target time input and controls
    const pitrTargetInput = page.getByLabel("Target Time");
    await expect(pitrTargetInput).toBeVisible({ timeout: 5000 });

    await expect(page.getByRole("button", { name: /Validate/i })).toBeVisible();
    await expect(page.getByRole("button", { name: /Start Restore/i })).toBeVisible();

    // Verify dry run checkbox
    await expect(page.getByLabel("Dry run")).toBeVisible();
  });
});
