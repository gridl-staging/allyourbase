import { test, expect, seedCustomDomain, cleanupCustomDomain, probeEndpoint, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Custom Domains
 *
 * Critical Path: Seed a domain → Navigate to Custom Domains → Verify the seeded
 * domain row renders with hostname, status, and environment fields.
 */

test.describe("Smoke: Custom Domains", () => {
  const domainIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (domainIDs.length > 0) {
      const id = domainIDs.pop();
      if (!id) continue;
      await cleanupCustomDomain(request, adminToken, id).catch(() => {});
    }
  });

  test("seeded domain renders with status and environment in the table", async ({ page, request, adminToken }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/domains");
    test.skip(
      status === 501 || status === 404,
      `Custom domains service not configured (status ${status})`,
    );

    const runId = Date.now();
    const hostname = `smoke-${runId}.example.com`;
    const seeded = await seedCustomDomain(request, adminToken, hostname, { environment: "staging" });
    domainIDs.push(seeded.id);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Custom Domains/i }).click();
    await expect(page.getByRole("heading", { name: /Custom Domains/i })).toBeVisible({ timeout: 15_000 });

    // Verify table column headers
    await expect(page.getByRole("columnheader", { name: /Hostname/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Status/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Environment/i })).toBeVisible();

    // Verify seeded domain row renders with real field values
    const domainRow = page.locator("tr").filter({ hasText: hostname }).first();
    await expect(domainRow).toBeVisible({ timeout: 5000 });
    await expect(domainRow).toContainText(seeded.status);
    await expect(domainRow).toContainText(seeded.environment);
    await expect(domainRow).toContainText(seeded.verificationRecord);

    // Verify add-domain button
    await expect(page.getByRole("button", { name: /Add Domain/i })).toBeVisible();
  });
});
