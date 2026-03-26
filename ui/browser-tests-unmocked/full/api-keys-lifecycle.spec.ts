import {
  test,
  expect,
  probeEndpoint,
  ensureUserByEmail,
  cleanupUserByEmail,
  seedApiKey,
  seedAdminApp,
  cleanupAdminAppByName,
  cleanupApiKeyByName,
  waitForDashboard,
} from "../fixtures";
import type { APIRequestContext, Page } from "@playwright/test";

/**
 * FULL E2E TEST: API Keys Lifecycle
 *
 * Tests complete API key management:
 * - Setup: Create a test user via SQL (required for key creation)
 * - Create API key with name, user, scope
 * - Verify key displayed in creation modal
 * - Verify key appears in list
 * - Revoke API key
 */

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function openAPIKeysPage(
  page: Page,
  request: APIRequestContext,
  adminToken: string,
): Promise<void> {
  const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/api-keys");
  test.skip(
    probeStatus === 503 || probeStatus === 404 || probeStatus === 501 || probeStatus === 500,
    `API keys service unavailable (status ${probeStatus})`,
  );
  await page.goto("/admin/");
  await waitForDashboard(page);
  const apiKeysButton = page.getByRole("button", { name: /^API Keys$/i });
  await expect(apiKeysButton).toBeVisible({ timeout: 5000 });
  await apiKeysButton.click();
  await expect(page.getByRole("heading", { name: /API Keys/i })).toBeVisible({ timeout: 5000 });
}

test.describe("API Keys Lifecycle (Full E2E)", () => {
  const keyNames: string[] = [];
  const appNames: string[] = [];
  const userEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (keyNames.length > 0) {
      const keyName = keyNames.pop();
      if (!keyName) continue;
      await cleanupApiKeyByName(request, adminToken, keyName).catch(() => {});
    }
    while (appNames.length > 0) {
      const appName = appNames.pop();
      if (!appName) continue;
      await cleanupAdminAppByName(request, adminToken, appName).catch(() => {});
    }
    while (userEmails.length > 0) {
      const userEmail = userEmails.pop();
      if (!userEmail) continue;
      await cleanupUserByEmail(request, adminToken, userEmail).catch(() => {});
    }
  });

  test("seeded API key renders in list view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const keyName = `seed-key-${runId}`;
    const testEmail = `apikey-seed-${runId}@test.com`;

    keyNames.push(keyName);
    userEmails.push(testEmail);

    // Arrange: create user and API key via SQL helpers
    const user = await ensureUserByEmail(request, adminToken, testEmail);
    await seedApiKey(
      request,
      adminToken,
      {
        userId: user.id,
        name: keyName,
        keyHash: `seedhash${runId}`,
        keyPrefix: "ayb_seed",
      },
    );

    // Act: navigate to API Keys page
    await openAPIKeysPage(page, request, adminToken);

    // Assert: seeded key name appears in the list
    await expect(page.getByText(keyName).first()).toBeVisible({ timeout: 5000 });

    // Cleanup handled by afterEach
  });

  test("create, view, and revoke app-scoped API key", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const testEmail = `apikey-test-${runId}@test.com`;
    const appName = `orders-app-${runId}`;
    const keyName = `orders-key-${runId}`;
    const appRateLimit = "120 req/60s";
    keyNames.push(keyName);
    appNames.push(appName);
    userEmails.push(testEmail);

    // Arrange: create user and app via SQL helpers.
    const user = await ensureUserByEmail(request, adminToken, testEmail);
    const app = await seedAdminApp(request, adminToken, {
      name: appName,
      ownerUserId: user.id,
      description: "seeded by browser test",
      rateLimitRps: 120,
      rateLimitWindowSeconds: 60,
    });

    // Act: navigate to API Keys.
    await openAPIKeysPage(page, request, adminToken);

    // ============================================================
    // CREATE: Add new API key
    // ============================================================
    const createButton = page.getByRole("button", { name: /create key|new key|add/i });
    await expect(createButton.first()).toBeVisible({ timeout: 3000 });
    await createButton.first().click();

    // Fill creation form
    await page.getByLabel("Key name").fill(keyName);

    // User selector
    const userSelect = page.getByLabel("User");
    await expect(userSelect).toBeVisible({ timeout: 2000 });
    const optCount = await userSelect.getByRole("option").count();
    expect(optCount).toBeGreaterThan(1);
    await userSelect.selectOption({ value: user.id });

    // App selector
    const appSelect = page.getByLabel("App Scope");
    await expect(appSelect).toBeVisible({ timeout: 2000 });
    await appSelect.selectOption({ value: app.id });

    // Submit
    const submitBtn = page.getByRole("button", { name: /^create$|^save$/i });
    await expect(submitBtn).toBeVisible();
    await submitBtn.click();

    // ============================================================
    // VERIFY: Key created modal shows the key
    // ============================================================
    await expect(page.getByText("API Key Created")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(keyName, { exact: true }).first()).toBeVisible({ timeout: 3000 });
    await expect(page.getByText(appName, { exact: true }).first()).toBeVisible({ timeout: 3000 });
    await expect(page.getByText(appRateLimit).first()).toBeVisible({ timeout: 3000 });

    // Close the created modal by clicking Done
    const doneBtn = page.getByRole("button", { name: /^done$/i });
    await expect(doneBtn).toBeVisible({ timeout: 3000 });
    await doneBtn.click();

    // Wait for modal to fully dismiss
    await expect(page.getByText("API Key Created")).not.toBeVisible({ timeout: 3000 });

    // ============================================================
    // VERIFY: Key appears in list
    // ============================================================
    await expect(page.getByText(keyName).first()).toBeVisible({ timeout: 5000 });

    // Verify Active badge on the key's row
    const activeKeyRow = page.getByRole("row", { name: new RegExp(escapeRegExp(keyName)) });
    await expect(activeKeyRow.getByText(appName)).toBeVisible({ timeout: 3000 });
    await expect(activeKeyRow.getByText(`Rate: ${appRateLimit}`)).toBeVisible({ timeout: 3000 });
    await expect(activeKeyRow.getByText("Active")).toBeVisible({ timeout: 2000 });

    // ============================================================
    // REVOKE: Revoke the API key
    // ============================================================
    const revokeButton = activeKeyRow.getByRole("button", { name: "Revoke key" });

    await expect(revokeButton).toBeVisible({ timeout: 3000 });
    await revokeButton.click();

    // Confirm revocation
    const confirmBtn = page.getByRole("button", { name: /^revoke$|^delete$|^confirm$/i });
    await expect(confirmBtn).toBeVisible({ timeout: 2000 });
    await confirmBtn.click();

    // Verify revoked
    const revokedKeyRow = page.getByRole("row", { name: new RegExp(escapeRegExp(keyName)) });
    await expect(revokedKeyRow.getByText("Revoked")).toBeVisible({ timeout: 5000 });

    // Cleanup handled by afterEach
  });
});
