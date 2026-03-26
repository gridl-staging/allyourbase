import { test, expect, seedSAMLProvider, cleanupSAMLProvider, probeEndpoint, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: SAML Configuration
 *
 * Critical Path: Seed a SAML provider → Navigate to SAML → Verify the seeded
 * provider name and entity ID render in the table body.
 */

test.describe("Smoke: SAML", () => {
  const providerNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (providerNames.length > 0) {
      const name = providerNames.pop();
      if (!name) continue;
      await cleanupSAMLProvider(request, adminToken, name).catch(() => {});
    }
  });

  test("seeded SAML provider renders in the configuration table", async ({ page, request, adminToken }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/auth/saml");
    test.skip(
      status === 501 || status === 404,
      `SAML service not configured (status ${status})`,
    );

    const runId = Date.now();
    const providerName = `smoke-saml-${runId}`;
    const entityId = `urn:smoke:${runId}`;
    await seedSAMLProvider(request, adminToken, { name: providerName, entity_id: entityId });
    providerNames.push(providerName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /SAML/i }).click();
    await expect(page.getByRole("heading", { name: /SAML Configuration/i })).toBeVisible({ timeout: 15_000 });

    // Verify table column headers
    await expect(page.getByRole("columnheader", { name: /Name/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Entity ID/i })).toBeVisible();

    // Verify seeded provider appears in the table body
    await expect(page.getByText(providerName)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(entityId)).toBeVisible();

    // Verify add-provider button
    await expect(page.getByRole("button", { name: /Add Provider/i })).toBeVisible();
  });
});
