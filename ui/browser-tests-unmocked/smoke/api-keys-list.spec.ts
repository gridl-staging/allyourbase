import {
  test,
  expect,
  probeEndpoint,
  ensureUserByEmail,
  cleanupUserByEmail,
  seedApiKey,
  cleanupApiKeyByName,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: API Keys - List View
 *
 * Critical Path: Navigate to API Keys → Verify seeded key row renders with status
 */

test.describe("Smoke: API Keys List", () => {
  const keyNames: string[] = [];
  const userEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (keyNames.length > 0) {
      const name = keyNames.pop();
      if (!name) continue;
      await cleanupApiKeyByName(request, adminToken, name).catch(() => {});
    }
    while (userEmails.length > 0) {
      const email = userEmails.pop();
      if (!email) continue;
      await cleanupUserByEmail(request, adminToken, email).catch(() => {});
    }
  });

  test("seeded API key appears in list with active status", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/api-keys");
    test.skip(
      probeStatus === 404 || probeStatus === 501,
      `API keys service not configured (status ${probeStatus})`,
    );

    const runId = Date.now();
    const testEmail = `apikey-smoke-${runId}@test.com`;
    const keyName = `smoke-key-${runId}`;

    const user = await ensureUserByEmail(request, adminToken, testEmail);
    userEmails.push(testEmail);

    await seedApiKey(request, adminToken, {
      userId: user.id,
      name: keyName,
      keyPrefix: "ayb_smoke",
      scope: "*",
    });
    keyNames.push(keyName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^API Keys$/i }).click();
    await expect(page.getByRole("heading", { name: /API Keys/i })).toBeVisible({ timeout: 15_000 });

    const row = page.getByRole("row", { name: new RegExp(keyName) }).first();
    await expect(row).toBeVisible({ timeout: 5000 });
    await expect(row.getByText("full access")).toBeVisible();
    await expect(row.getByText("Active")).toBeVisible();
    await expect(row.getByRole("button", { name: /Revoke key/i })).toBeVisible();
  });
});
