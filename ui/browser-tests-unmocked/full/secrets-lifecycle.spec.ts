import {
  test,
  expect,
  probeEndpoint,
  seedSecret,
  cleanupSecret,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Secrets Lifecycle
 *
 * Critical Path: Load seeded secret → create secret via UI → reveal → update → delete via UI
 */

test.describe("Secrets Lifecycle (Full E2E)", () => {
  const secretNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (secretNames.length > 0) {
      const name = secretNames.pop();
      if (!name) continue;
      await cleanupSecret(request, adminToken, name).catch(() => {});
    }
  });

  test("load-and-verify seeded secret, then create, reveal, update, and delete via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/secrets");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Secrets service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const seededName = `SECRET_FULL_SEEDED_${runId}`;
    const seededValue = `seeded-value-${runId}`;
    const createdName = `SECRET_FULL_CREATED_${runId}`;
    const createdValue = `created-value-${runId}`;
    const updatedValue = `updated-value-${runId}`;

    await seedSecret(request, adminToken, seededName, seededValue);
    secretNames.push(seededName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Secrets$/i }).click();
    await expect(page.getByRole("heading", { name: /Secrets/i })).toBeVisible({ timeout: 5000 });

    // Verify seeded secret
    const seededRow = page.getByRole("row", { name: new RegExp(seededName) }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });

    // Reveal seeded secret
    await seededRow.getByRole("button", { name: /Reveal/i }).click();
    await expect(seededRow.getByText(seededValue)).toBeVisible({ timeout: 5000 });

    // Create new secret via UI
    await page.getByRole("button", { name: /Create Secret/i }).click();
    await expect(page.getByText(/New Secret/i)).toBeVisible({ timeout: 5000 });

    await page.getByLabel("Name").fill(createdName);
    await page.getByLabel("Value").fill(createdValue);
    await page.getByRole("button", { name: /^Create$/i }).click();
    secretNames.push(createdName);

    const createdRow = page.getByRole("row", { name: new RegExp(createdName) }).first();
    await expect(createdRow).toBeVisible({ timeout: 5000 });

    // Update the created secret
    await createdRow.getByRole("button", { name: /Update/i }).click();
    await expect(page.getByText(new RegExp(`Update ${createdName}`, "i"))).toBeVisible({ timeout: 5000 });

    await page.getByLabel("Value").clear();
    await page.getByLabel("Value").fill(updatedValue);
    await page.getByRole("button", { name: /^Update$/i }).click();

    // Verify update took effect by revealing
    await createdRow.getByRole("button", { name: /Reveal/i }).click();
    await expect(createdRow.getByText(updatedValue)).toBeVisible({ timeout: 5000 });

    // Delete the created secret
    await createdRow.getByRole("button", { name: /Delete/i }).click();
    await expect(page.getByRole("heading", { name: /Delete Secret/i })).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Delete$/i }).click();

    await expect(page.getByRole("row", { name: new RegExp(createdName) })).toHaveCount(0, { timeout: 5000 });
  });
});
