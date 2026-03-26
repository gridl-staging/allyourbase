import {
  test,
  expect,
  probeEndpoint,
  ensureUserByEmail,
  cleanupUserByEmail,
  seedAdminApp,
  cleanupAdminAppByName,
  seedOAuthClient,
  cleanupOAuthClientByName,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: OAuth Clients Lifecycle
 *
 * Critical Path: Load seeded row -> create client via UI -> revoke client via UI
 */

test.describe("OAuth Clients Lifecycle (Full E2E)", () => {
  const clientNames: string[] = [];
  const appNames: string[] = [];
  const userEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (clientNames.length > 0) {
      const clientName = clientNames.pop();
      if (!clientName) continue;
      await cleanupOAuthClientByName(request, adminToken, clientName).catch(() => {});
    }
    while (appNames.length > 0) {
      const appName = appNames.pop();
      if (!appName) continue;
      await cleanupAdminAppByName(request, adminToken, appName).catch(() => {});
    }
    while (userEmails.length > 0) {
      const email = userEmails.pop();
      if (!email) continue;
      await cleanupUserByEmail(request, adminToken, email).catch(() => {});
    }
  });

  test("load-and-verify seeded client, then register and revoke a client via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/oauth/clients");
    test.skip(
      probeStatus === 404 || probeStatus === 501,
      `OAuth clients service not configured (status ${probeStatus})`,
    );

    const runId = Date.now();
    const ownerEmail = `oauth-full-owner-${runId}@test.com`;
    const appName = `oauth-full-app-${runId}`;
    const seededClientName = `oauth-full-seeded-${runId}`;
    const createdClientName = `oauth-full-created-${runId}`;
    const redirectUri = `https://example.test/oauth/${runId}/callback`;

    const owner = await ensureUserByEmail(request, adminToken, ownerEmail);
    userEmails.push(ownerEmail);

    const app = await seedAdminApp(request, adminToken, {
      name: appName,
      ownerUserId: owner.id,
      description: `OAuth lifecycle app ${runId}`,
    });
    appNames.push(appName);

    const seededClient = await seedOAuthClient(request, adminToken, {
      appId: app.id,
      name: seededClientName,
      clientType: "confidential",
      redirectUris: [redirectUri],
      scopes: ["readonly"],
    });
    clientNames.push(seededClientName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /OAuth Clients/i }).click();
    await expect(page.getByRole("heading", { name: /OAuth Clients/i })).toBeVisible({ timeout: 5000 });

    const seededRow = page.getByRole("row", { name: new RegExp(seededClientName) }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });
    await expect(seededRow).toContainText(appName);
    await expect(seededRow).toContainText("Active");
    await expect(seededRow.getByRole("button", { name: /Rotate secret/i })).toBeVisible();
    await expect(seededRow.getByRole("button", { name: /Revoke client/i })).toBeVisible();

    await seededRow.getByRole("button", { name: /Rotate secret/i }).click();
    await expect(page.getByRole("heading", { name: /Rotate Client Secret/i })).toBeVisible({
      timeout: 5000,
    });
    await page.getByRole("button", { name: /^Rotate$/i }).click();
    await expect(page.getByRole("heading", { name: /New Client Secret/i })).toBeVisible({
      timeout: 5000,
    });
    const rotatedSecretLocator = page.getByText(/^ayb_cs_/).first();
    await expect(rotatedSecretLocator).toBeVisible({ timeout: 5000 });
    await expect(rotatedSecretLocator).toHaveText(/^ayb_cs_/);
    const rotatedSecret = (await rotatedSecretLocator.innerText()).trim();
    expect(rotatedSecret).not.toBe(seededClient.clientSecret);
    await page.getByRole("button", { name: /^Done$/i }).click();

    await page.getByRole("button", { name: /Register Client/i }).click();
    await expect(page.getByRole("heading", { name: /Register OAuth Client/i })).toBeVisible({ timeout: 5000 });
    await page.getByLabel(/Client name/i).fill(createdClientName);
    await page.getByLabel(/^App$/i).selectOption(app.id);
    await page.getByLabel(/Redirect URIs/i).fill(redirectUri);
    await page.getByRole("button", { name: /^Register$/i }).click();

    await expect(page.getByRole("heading", { name: /OAuth Client Registered/i })).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Done$/i }).click();

    clientNames.push(createdClientName);

    const createdRow = page.getByRole("row", { name: new RegExp(createdClientName) }).first();
    await expect(createdRow).toBeVisible({ timeout: 5000 });
    await expect(createdRow).toContainText("Active");

    await createdRow.getByRole("button", { name: /Revoke client/i }).click();
    await expect(page.getByRole("heading", { name: /Revoke OAuth Client/i })).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Revoke$/i }).click();

    await expect(createdRow).toContainText("Revoked", { timeout: 5000 });
    await expect(createdRow.getByRole("button", { name: /Revoke client/i })).toHaveCount(0);
  });
});
