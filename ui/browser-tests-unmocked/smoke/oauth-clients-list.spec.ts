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
 * SMOKE TEST: OAuth Clients - List View
 *
 * Critical Path: Navigate to OAuth Clients → verify seeded client row renders with actions
 */

test.describe("Smoke: OAuth Clients List", () => {
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

  test("seeded OAuth client appears in table with active actions", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/oauth/clients");
    test.skip(
      probeStatus === 404 || probeStatus === 501,
      `OAuth clients service not configured (status ${probeStatus})`,
    );

    const runId = Date.now();
    const ownerEmail = `oauth-clients-smoke-owner-${runId}@test.com`;
    const appName = `oauth-smoke-app-${runId}`;
    const clientName = `oauth-smoke-client-${runId}`;

    const owner = await ensureUserByEmail(request, adminToken, ownerEmail);
    userEmails.push(ownerEmail);

    const app = await seedAdminApp(request, adminToken, {
      name: appName,
      ownerUserId: owner.id,
      description: `OAuth smoke app ${runId}`,
    });
    appNames.push(appName);

    await seedOAuthClient(request, adminToken, {
      appId: app.id,
      name: clientName,
      clientType: "confidential",
      redirectUris: [`https://example.test/oauth/${runId}/callback`],
      scopes: ["readonly"],
    });
    clientNames.push(clientName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /OAuth Clients/i }).click();
    await expect(page.getByRole("heading", { name: /OAuth Clients/i })).toBeVisible({ timeout: 15_000 });

    const row = page.getByRole("row", { name: new RegExp(clientName) }).first();
    await expect(row).toBeVisible({ timeout: 5000 });
    await expect(row.getByText(appName)).toBeVisible();
    await expect(row.getByText("confidential")).toBeVisible();
    await expect(row.getByText("Active")).toBeVisible();
    await expect(row.getByRole("button", { name: /Rotate secret/i })).toBeVisible();
    await expect(row.getByRole("button", { name: /Revoke client/i })).toBeVisible();
  });
});
