import {
  test,
  expect,
  seedSecret,
  cleanupSecret,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Secrets
 *
 * Critical Path: Seed a secret → Navigate to Secrets → Verify seeded secret row
 * with Name/Created/Updated columns, then reveal the value to confirm detail state.
 */

test.describe("Smoke: Secrets", () => {
  const seededSecretNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededSecretNames.length > 0) {
      const name = seededSecretNames.pop();
      if (!name) continue;
      await cleanupSecret(request, adminToken, name).catch(() => {});
    }
  });

  test("seeded secret renders with timestamps and reveal shows value", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const secretName = `SMOKE_SECRET_${runId}`;
    const secretValue = `smoke-secret-value-${runId}`;

    await seedSecret(request, adminToken, secretName, secretValue);
    seededSecretNames.push(secretName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Secrets/i }).click();
    await expect(page.getByRole("heading", { name: /Secrets/i })).toBeVisible({ timeout: 15_000 });

    // Verify table column headers including timestamp columns
    await expect(page.getByRole("columnheader", { name: /Name/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Created/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Updated/i })).toBeVisible();

    // Verify seeded secret row renders
    const secretRow = page.locator("tr").filter({ hasText: secretName }).first();
    await expect(secretRow).toBeVisible({ timeout: 5000 });

    // Reveal the secret value
    await secretRow.getByRole("button", { name: new RegExp(`Reveal\\s+${secretName}`) }).click();
    await expect(page.getByText(secretValue).first()).toBeVisible({ timeout: 5000 });

    // Verify action buttons
    await expect(page.getByRole("button", { name: /Create Secret/i })).toBeVisible();
    await expect(page.getByRole("button", { name: /Rotate JWT Secret/i })).toBeVisible();
  });
});
