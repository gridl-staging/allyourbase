import { test, expect, execSQL } from "../fixtures";
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

const TEST_PASSWORD_HASH = "$argon2id$v=19$m=65536,t=3,p=4$dGVzdHNhbHQ$dGVzdGhhc2g";

function sqlLiteral(value: string): string {
  return value.replace(/'/g, "''");
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function createUser(
  request: APIRequestContext,
  adminToken: string,
  email: string,
): Promise<string> {
  const escapedEmail = sqlLiteral(email);
  await execSQL(
    request,
    adminToken,
    `INSERT INTO _ayb_users (email, password_hash) VALUES ('${escapedEmail}', '${TEST_PASSWORD_HASH}') ON CONFLICT (email) DO NOTHING;`,
  );
  const result = await execSQL(
    request,
    adminToken,
    `SELECT id FROM _ayb_users WHERE email = '${escapedEmail}';`,
  );
  const userID = result.rows[0]?.[0];
  if (typeof userID !== "string") {
    throw new Error(`Expected user id for email ${email}`);
  }
  return userID;
}

async function createApp(
  request: APIRequestContext,
  adminToken: string,
  ownerUserID: string,
  name: string,
  rateLimitRps: number,
  rateLimitWindowSeconds: number,
): Promise<string> {
  const result = await execSQL(
    request,
    adminToken,
    `INSERT INTO _ayb_apps (name, description, owner_user_id, rate_limit_rps, rate_limit_window_seconds) VALUES ('${sqlLiteral(name)}', 'seeded by browser test', '${ownerUserID}', ${rateLimitRps}, ${rateLimitWindowSeconds}) RETURNING id;`,
  );
  const appID = result.rows[0]?.[0];
  if (typeof appID !== "string") {
    throw new Error(`Expected app id for app ${name}`);
  }
  return appID;
}

async function openAPIKeysPage(page: Page): Promise<void> {
  await page.goto("/admin/");
  const apiKeysButton = page.getByRole("button", { name: /^API Keys$/i });
  await expect(apiKeysButton).toBeVisible({ timeout: 5000 });
  await apiKeysButton.click();
  await expect(page.getByRole("heading", { name: /API Keys/i })).toBeVisible({ timeout: 5000 });
}

test.describe("API Keys Lifecycle (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded API key renders in list view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const keyName = `seed-key-${runId}`;
    const testEmail = `apikey-seed-${runId}@test.com`;

    // Register cleanup early so afterEach runs it even on failure
    pendingCleanup.push(
      `DELETE FROM _ayb_api_keys WHERE name = '${keyName}';`,
      `DELETE FROM _ayb_users WHERE email = '${testEmail}';`,
    );

    // Arrange: create user and API key via SQL helpers
    const userId = await createUser(request, adminToken, testEmail);
    await execSQL(
      request,
      adminToken,
      `INSERT INTO _ayb_api_keys (name, key_hash, key_prefix, user_id) VALUES ('${keyName}', 'seedhash${runId}', 'ayb_seed', '${userId}');`,
    );

    // Act: navigate to API Keys page
    await openAPIKeysPage(page);

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
    const escapedAppName = sqlLiteral(appName);

    // Register cleanup early so afterEach runs it even on failure
    pendingCleanup.push(
      `DELETE FROM _ayb_api_keys WHERE name = '${keyName}';`,
      `DELETE FROM _ayb_apps WHERE name = '${escapedAppName}';`,
      `DELETE FROM _ayb_users WHERE email = '${testEmail}';`,
    );

    // Arrange: create user and app via SQL helpers.
    const userID = await createUser(request, adminToken, testEmail);
    const appID = await createApp(request, adminToken, userID, appName, 120, 60);

    // Act: navigate to API Keys.
    await openAPIKeysPage(page);

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
    await userSelect.selectOption({ value: userID });

    // App selector
    const appSelect = page.getByLabel("App Scope");
    await expect(appSelect).toBeVisible({ timeout: 2000 });
    await appSelect.selectOption({ value: appID });

    // Submit
    const submitBtn = page.getByRole("button", { name: /^create$|^save$/i });
    await expect(submitBtn).toBeVisible();
    await submitBtn.click();

    // ============================================================
    // VERIFY: Key created modal shows the key
    // ============================================================
    await expect(page.getByText("API Key Created")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(keyName)).toBeVisible({ timeout: 3000 });
    await expect(page.getByText(appName)).toBeVisible({ timeout: 3000 });
    await expect(page.getByText(appRateLimit)).toBeVisible({ timeout: 3000 });

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
