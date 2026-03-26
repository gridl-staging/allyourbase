import {
  test,
  expect,
  probeEndpoint,
  seedSAMLProvider,
  cleanupSAMLProvider,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: SAML Provider Lifecycle
 *
 * Critical Path: Load seeded provider → create provider via UI → edit → delete via UI
 */

test.describe("SAML Provider Lifecycle (Full E2E)", () => {
  const providerNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (providerNames.length > 0) {
      const name = providerNames.pop();
      if (!name) continue;
      await cleanupSAMLProvider(request, adminToken, name).catch(() => {});
    }
  });

  test("load-and-verify seeded provider, then create, edit, and delete via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth/saml");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `SAML service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const seededName = `saml-full-seeded-${runId}`;
    const seededEntityId = `urn:test:seeded-${runId}`;
    const createdName = `saml-full-created-${runId}`;
    const createdEntityId = `urn:test:created-${runId}`;

    await seedSAMLProvider(request, adminToken, {
      name: seededName,
      entity_id: seededEntityId,
    });
    providerNames.push(seededName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^SAML$/i }).click();
    await expect(page.getByRole("heading", { name: /SAML Configuration/i })).toBeVisible({ timeout: 5000 });

    // Verify seeded provider
    const seededRow = page.getByRole("row", { name: new RegExp(seededName) }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });
    await expect(seededRow).toContainText(seededEntityId);

    // Create new SAML provider via UI
    await page.getByRole("button", { name: /Add Provider/i }).click();

    await page.getByLabel("Name").fill(createdName);
    await page.getByLabel("Entity ID").fill(createdEntityId);
    await page.getByLabel("Metadata URL").fill(`https://idp.example.com/${createdName}/metadata`);
    await page.getByRole("button", { name: /^Create$/i }).click();
    providerNames.push(createdName);

    const createdRow = page.getByRole("row", { name: new RegExp(createdName) }).first();
    await expect(createdRow).toBeVisible({ timeout: 5000 });
    await expect(createdRow).toContainText(createdEntityId);

    // Edit the created provider
    await createdRow.getByRole("button", { name: /Edit/i }).click();

    const entityIdInput = page.getByLabel("Entity ID");
    const updatedEntityId = `urn:test:updated-${runId}`;
    await entityIdInput.clear();
    await entityIdInput.fill(updatedEntityId);
    await page.getByRole("button", { name: /^Update$/i }).click();

    await expect(createdRow).toContainText(updatedEntityId, { timeout: 5000 });

    // Delete the created provider
    await createdRow.getByRole("button", { name: /Delete/i }).click();
    await expect(page.getByText(/Delete Provider/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Delete$/i }).click();

    await expect(page.getByRole("row", { name: new RegExp(createdName) })).toHaveCount(0, { timeout: 5000 });
  });
});
