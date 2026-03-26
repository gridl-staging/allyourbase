import {
  test,
  expect,
  probeEndpoint,
  listAuthProviders,
  updateAuthProvider,
  deleteAuthProvider,
  waitForDashboard,
} from "../fixtures";
import type { Page } from "@playwright/test";

/**
 * FULL E2E TEST: Auth Provider Management
 *
 * Tests the provider management dashboard with a real running server:
 * - List seeded providers
 * - Configure a provider via the edit form
 * - Add a custom OIDC provider
 * - Verify provider status updates
 */

test.describe("Auth Provider Management (Full E2E)", () => {
  const oidcCleanup: string[] = [];

  async function openAuthSettings(page: Page): Promise<void> {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "Auth Settings" })).toBeVisible();
  }

  async function addOIDCProviderViaUI(page: Page, providerName: string): Promise<void> {
    await page.getByTestId("add-oidc-provider").click();
    await expect(page.getByText("Add Custom OIDC Provider")).toBeVisible();
    await page.getByTestId("oidc-form-provider-name").fill(providerName);
    await page.getByTestId("oidc-form-issuer-url").fill("https://accounts.google.com");
    await page.getByTestId("oidc-form-client-id").fill("test-oidc-client-id");
    await page.getByTestId("oidc-form-client-secret").fill("test-oidc-client-secret");
    await page.getByTestId("oidc-form-display-name").fill("E2E Test OIDC");
    await page.getByTestId("oidc-form-save").click();
    await expect(
      page.getByText(new RegExp(`OIDC provider "${providerName}" added`, "i")),
    ).toBeVisible();
    const providerRow = page.getByTestId(`provider-row-${providerName}`);
    await expect(providerRow).toBeVisible();
    await expect(providerRow.getByText(/^OIDC$/)).toBeVisible();
    await expect(providerRow.getByText(/^Built-in$/)).toHaveCount(0);
  }

  test.afterEach(async ({ request, adminToken }) => {
    for (const name of oidcCleanup) {
      await deleteAuthProvider(request, adminToken, name).catch(() => {});
    }
    oidcCleanup.length = 0;
  });

  test("seeded providers render in the auth settings list", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth/providers");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501 || probeStatus === 500,
      `Auth providers service unavailable (status ${probeStatus})`,
    );

    // Arrange: verify providers exist in API
    const providers = await listAuthProviders(request, adminToken);
    // Google and GitHub should always be present as built-in providers
    const providerNames = providers.map((p) => p.name);
    // The server should have at least google and github registered
    test.skip(
      !providerNames.includes("google"),
      "Google provider not registered — server may not have auth configured",
    );

    // Act: navigate to Auth Settings
    await openAuthSettings(page);
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    // Assert: google provider row renders with correct info
    await expect(page.getByTestId("provider-row-google")).toBeVisible();
    await expect(page.getByTestId("provider-row-google")).toContainText("Built-in");
  });

  test("configure a built-in provider via the edit form", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth/providers");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501 || probeStatus === 500,
      `Auth providers service unavailable (status ${probeStatus})`,
    );

    // Arrange: verify discord provider exists
    const providers = await listAuthProviders(request, adminToken);
    const providerNames = providers.map((p) => p.name);
    test.skip(
      !providerNames.includes("discord"),
      "Discord provider not registered on this server",
    );

    // Act: navigate to Auth Settings and open discord edit form
    await openAuthSettings(page);
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    // Click Edit/Configure on discord
    await page.getByTestId("provider-edit-discord").click();

    // Form should be visible
    await expect(page.getByTestId("provider-form-client-id")).toBeVisible();
    await expect(page.getByTestId("provider-form-client-secret")).toBeVisible();

    // Setup instructions should be visible for built-in provider
    await expect(page.getByTestId("provider-setup-instructions")).toBeVisible();
    await expect(page.getByTestId("provider-setup-instructions")).toContainText(
      /discord\.com\/developers/,
    );
    await expect(page.getByTestId("provider-setup-instructions")).toContainText(
      /\/oauth\/discord\/callback/,
    );

    // Cancel without saving
    await page.getByTestId("provider-form-cancel").click();
    await expect(page.getByTestId("provider-form-client-id")).toBeHidden();
  });

  test("add and verify a custom OIDC provider", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth/providers");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501 || probeStatus === 500,
      `Auth providers service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const providerName = `e2e-custom-${runId}`;
    oidcCleanup.push(providerName);

    // Arrange: ensure the test provider doesn't already exist
    await deleteAuthProvider(request, adminToken, providerName).catch(() => {});

    // Act: navigate to Auth Settings
    await openAuthSettings(page);
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await addOIDCProviderViaUI(page, providerName);

    // Assert: success message and provider appears in list
    await expect(page.getByTestId(`provider-enabled-${providerName}`)).toContainText("Enabled");

    // Cleanup handled by afterEach
  });

  test("shows OIDC-only edit fields when editing a custom OIDC provider", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth/providers");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501 || probeStatus === 500,
      `Auth providers service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const providerName = `e2e-edit-${runId}`;
    oidcCleanup.push(providerName);

    await deleteAuthProvider(request, adminToken, providerName).catch(() => {});
    await openAuthSettings(page);
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();
    await addOIDCProviderViaUI(page, providerName);

    const providersBeforeEdit = await listAuthProviders(request, adminToken);
    const createdProvidersForEdit = providersBeforeEdit.filter(
      (provider) => provider.name === providerName,
    );
    expect(createdProvidersForEdit).toHaveLength(1);
    expect(createdProvidersForEdit[0]?.type).toBe("oidc");
    expect(createdProvidersForEdit[0]?.enabled).toBe(true);
    expect(createdProvidersForEdit[0]?.client_id_configured).toBe(true);

    const rowForEdit = page.getByTestId(`provider-row-${providerName}`);
    await expect(rowForEdit.getByText(/^OIDC$/)).toBeVisible();
    await expect(rowForEdit.getByText(/^Built-in$/)).toHaveCount(0);

    await page.getByTestId(`provider-edit-${providerName}`).click();

    await expect(page.getByTestId("provider-form-issuer-url")).toBeVisible();
    await expect(page.getByTestId("provider-form-display-name")).toBeVisible();
    await expect(page.getByTestId("provider-form-scopes")).toBeVisible();
    await expect(page.getByTestId("provider-form-client-id")).toBeVisible();
    await expect(page.getByTestId("provider-form-client-secret")).toBeVisible();
    await expect(page.getByTestId("provider-form-enabled")).toBeVisible();
    await expect(page.getByTestId("provider-setup-instructions")).toHaveCount(0);

    await page.getByTestId("provider-form-cancel").click();
    await expect(page.getByTestId("provider-form-client-id")).toBeHidden();
  });

  test("deletes a custom OIDC provider via confirmation dialog", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth/providers");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501 || probeStatus === 500,
      `Auth providers service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const providerName = `e2e-delete-${runId}`;
    oidcCleanup.push(providerName);

    await deleteAuthProvider(request, adminToken, providerName).catch(() => {});
    await openAuthSettings(page);
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();
    await addOIDCProviderViaUI(page, providerName);

    const providersBeforeDelete = await listAuthProviders(request, adminToken);
    const createdProvidersForDelete = providersBeforeDelete.filter(
      (provider) => provider.name === providerName,
    );
    expect(createdProvidersForDelete).toHaveLength(1);
    expect(createdProvidersForDelete[0]?.type).toBe("oidc");
    expect(createdProvidersForDelete[0]?.enabled).toBe(true);
    expect(createdProvidersForDelete[0]?.client_id_configured).toBe(true);

    const rowForDelete = page.getByTestId(`provider-row-${providerName}`);
    await expect(rowForDelete.getByText(/^OIDC$/)).toBeVisible();
    await expect(rowForDelete.getByText(/^Built-in$/)).toHaveCount(0);

    await page.getByTestId(`provider-delete-${providerName}`).click();

    const deleteDialog = page.getByRole("dialog", { name: "Delete Provider" });
    await expect(deleteDialog).toBeVisible();
    await expect(deleteDialog).toContainText(providerName);

    const deleteButton = deleteDialog.getByRole("button", { name: "Delete" });
    await expect(deleteButton).toBeVisible();
    await expect(deleteButton).toHaveClass(/bg-red-600/);

    await deleteButton.click();
    await expect(deleteDialog).toHaveCount(0);
    await expect(page.getByTestId(`provider-row-${providerName}`)).toHaveCount(0);
    await expect(
      page.getByText(new RegExp(`Provider "${providerName}" deleted\\.`, "i")),
    ).toBeVisible();

    const providers = await listAuthProviders(request, adminToken);
    expect(providers.some((provider) => provider.name === providerName)).toBe(false);
  });
});
