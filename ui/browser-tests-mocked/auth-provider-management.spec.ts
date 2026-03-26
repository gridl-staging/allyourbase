import { test, expect, bootstrapMockedAdminApp, mockAuthProviderApis } from "./fixtures";

test.describe("Auth Provider Management (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  // ---------------------------------------------------------------
  // Load-and-verify (required per BROWSER_TESTING_STANDARDS_3.md)
  // ---------------------------------------------------------------

  test("load-and-verify: seeded providers render in default list view", async ({ page }) => {
    await mockAuthProviderApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();

    await expect(page.getByRole("heading", { name: "Auth Settings" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    // Verify seeded providers render with correct status
    await expect(page.getByTestId("provider-row-google")).toBeVisible();
    await expect(page.getByTestId("provider-enabled-google")).toContainText("Enabled");
    await expect(page.getByTestId("provider-client-google")).toContainText("Client ID configured");

    await expect(page.getByTestId("provider-row-github")).toBeVisible();
    await expect(page.getByTestId("provider-enabled-github")).toContainText("Enabled");

    await expect(page.getByTestId("provider-row-discord")).toBeVisible();
    await expect(page.getByTestId("provider-enabled-discord")).toContainText("Disabled");
    await expect(page.getByTestId("provider-client-discord")).toContainText("Client ID missing");

    // Verify type labels
    await expect(page.getByTestId("provider-row-google")).toContainText("Built-in");
  });

  // ---------------------------------------------------------------
  // Provider edit (save built-in provider)
  // ---------------------------------------------------------------

  test("edit provider: opens form, fills credentials, saves successfully", async ({ page }) => {
    const apis = await mockAuthProviderApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    // Click Configure on discord (unconfigured)
    await page.getByTestId("provider-edit-discord").click();

    // Form should be visible with Enable checkbox, Client ID, Client Secret
    await expect(page.getByTestId("provider-form-enabled")).toBeVisible();
    await expect(page.getByTestId("provider-form-client-id")).toBeVisible();
    await expect(page.getByTestId("provider-form-client-secret")).toBeVisible();

    // Fill and save
    await page.getByTestId("provider-form-enabled").check();
    await page.getByTestId("provider-form-client-id").fill("discord-id-123");
    await page.getByTestId("provider-form-client-secret").fill("discord-secret-456");
    await page.getByTestId("provider-form-save").click();

    // Verify success message appears
    await expect(page.getByText(/Provider "discord" updated/i)).toBeVisible();

    // Verify the API was called correctly
    expect(apis.updateCalls).toBe(1);
    expect(apis.lastUpdateProvider).toBe("discord");
    expect(apis.lastUpdateBody).toMatchObject({
      enabled: true,
      client_id: "discord-id-123",
      client_secret: "discord-secret-456",
    });

    // Status should update in the list
    await expect(page.getByTestId("provider-enabled-discord")).toContainText("Enabled");
    await expect(page.getByTestId("provider-client-discord")).toContainText("Client ID configured");
  });

  // ---------------------------------------------------------------
  // Provider edit error
  // ---------------------------------------------------------------

  test("edit provider: shows validation error from server", async ({ page }) => {
    await mockAuthProviderApis(page, {
      updateProviderResponse: {
        status: 400,
        body: { message: "auth.oauth.discord.client_secret is required when enabled" },
      },
    });

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await page.getByTestId("provider-edit-discord").click();
    await page.getByTestId("provider-form-enabled").check();
    await page.getByTestId("provider-form-client-id").fill("some-id");
    // Intentionally skip client_secret
    await page.getByTestId("provider-form-save").click();

    // Error message from server should be visible
    await expect(page.getByText(/client_secret is required when enabled/i)).toBeVisible();
  });

  // ---------------------------------------------------------------
  // Cancel edit
  // ---------------------------------------------------------------

  test("edit provider: cancel closes form without saving", async ({ page }) => {
    const apis = await mockAuthProviderApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await page.getByTestId("provider-edit-discord").click();
    await expect(page.getByTestId("provider-form-client-id")).toBeVisible();

    await page.getByTestId("provider-form-cancel").click();

    // Form should close
    await expect(page.getByTestId("provider-form-client-id")).toBeHidden();
    // No API call should have been made
    expect(apis.updateCalls).toBe(0);
  });

  // ---------------------------------------------------------------
  // Test Connection — success
  // ---------------------------------------------------------------

  test("connectivity check: shows success message for reachable provider", async ({ page }) => {
    const apis = await mockAuthProviderApis(page, {
      testProviderResponse: {
        status: 200,
        body: {
          success: true,
          provider: "google",
          message: "authorization endpoint is reachable",
        },
      },
    });

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await page.getByTestId("provider-edit-google").click();
    await page.getByTestId("provider-form-test").click();

    await expect(page.getByText(/authorization endpoint is reachable/i)).toBeVisible();
    expect(apis.testCalls).toBe(1);
    expect(apis.lastTestProvider).toBe("google");
  });

  // ---------------------------------------------------------------
  // Test Connection — failure
  // ---------------------------------------------------------------

  test("connectivity check: shows error message for unreachable provider", async ({ page }) => {
    await mockAuthProviderApis(page, {
      testProviderResponse: {
        status: 200,
        body: {
          success: false,
          provider: "google",
          error: "authorization endpoint unreachable: connection refused",
        },
      },
    });

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await page.getByTestId("provider-edit-google").click();
    await page.getByTestId("provider-form-test").click();

    await expect(page.getByText(/authorization endpoint unreachable/i)).toBeVisible();
  });

  // ---------------------------------------------------------------
  // OIDC: add custom provider
  // ---------------------------------------------------------------

  test("add OIDC provider: opens form, fills fields, saves, appears in list", async ({ page }) => {
    const apis = await mockAuthProviderApis(page, {
      providersListResponse: {
        status: 200,
        body: {
          providers: [
            { name: "google", type: "builtin", enabled: true, client_id_configured: true },
          ],
        },
      },
    });

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    // Click the Add OIDC Provider button
    await page.getByTestId("add-oidc-provider").click();

    // OIDC form should appear
    await expect(page.getByText("Add Custom OIDC Provider")).toBeVisible();
    await expect(page.getByTestId("oidc-form-provider-name")).toBeVisible();
    await expect(page.getByTestId("oidc-form-issuer-url")).toBeVisible();

    // Fill form
    await page.getByTestId("oidc-form-provider-name").fill("my-keycloak");
    await page.getByTestId("oidc-form-issuer-url").fill("https://keycloak.example.com/realms/main");
    await page.getByTestId("oidc-form-client-id").fill("kc-client-id");
    await page.getByTestId("oidc-form-client-secret").fill("kc-secret");
    await page.getByTestId("oidc-form-display-name").fill("Keycloak");

    // Save
    await page.getByTestId("oidc-form-save").click();

    // Success message
    await expect(page.getByText(/OIDC provider "my-keycloak" added/i)).toBeVisible();

    // Provider should appear in the list
    await expect(page.getByTestId("provider-row-my-keycloak")).toBeVisible();
    await expect(page.getByTestId("provider-row-my-keycloak")).toContainText("OIDC");

    // Verify API was called
    expect(apis.updateCalls).toBe(1);
    expect(apis.lastUpdateProvider).toBe("my-keycloak");
    expect(apis.lastUpdateBody).toMatchObject({
      enabled: true,
      issuer_url: "https://keycloak.example.com/realms/main",
      client_id: "kc-client-id",
      client_secret: "kc-secret",
      display_name: "Keycloak",
    });
  });

  // ---------------------------------------------------------------
  // OIDC: validation error (empty name)
  // ---------------------------------------------------------------

  test("add OIDC provider: shows validation error when name is empty", async ({ page }) => {
    const apis = await mockAuthProviderApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await page.getByTestId("add-oidc-provider").click();
    await expect(page.getByTestId("oidc-form-provider-name")).toBeVisible();

    // Leave name empty, fill other fields
    await page.getByTestId("oidc-form-issuer-url").fill("https://auth.example.com");
    await page.getByTestId("oidc-form-client-id").fill("cid");
    await page.getByTestId("oidc-form-save").click();

    // Client-side validation error
    await expect(page.getByText(/provider name is required/i)).toBeVisible();
    // No API call made
    expect(apis.updateCalls).toBe(0);
  });

  // ---------------------------------------------------------------
  // OIDC: cancel form
  // ---------------------------------------------------------------

  test("add OIDC provider: cancel closes form without saving", async ({ page }) => {
    const apis = await mockAuthProviderApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await page.getByTestId("add-oidc-provider").click();
    await expect(page.getByText("Add Custom OIDC Provider")).toBeVisible();

    await page.getByTestId("oidc-form-cancel").click();

    // Form should close, Add button should reappear
    await expect(page.getByTestId("add-oidc-provider")).toBeVisible();
    expect(apis.updateCalls).toBe(0);
  });

  // ---------------------------------------------------------------
  // OIDC: server-side save error
  // ---------------------------------------------------------------

  test("add OIDC provider: shows server error on save failure", async ({ page }) => {
    await mockAuthProviderApis(page, {
      updateProviderResponse: {
        status: 400,
        body: { message: "registering OIDC provider: discovery fetch failed" },
      },
    });

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await page.getByTestId("add-oidc-provider").click();
    await page.getByTestId("oidc-form-provider-name").fill("bad-provider");
    await page.getByTestId("oidc-form-issuer-url").fill("https://bad.example.com");
    await page.getByTestId("oidc-form-client-id").fill("cid");
    await page.getByTestId("oidc-form-client-secret").fill("csecret");
    await page.getByTestId("oidc-form-save").click();

    await expect(page.getByText(/discovery fetch failed/i)).toBeVisible();
  });

  // ---------------------------------------------------------------
  // Setup instructions for built-in providers
  // ---------------------------------------------------------------

  test("setup instructions: shows console link and redirect URI for built-in provider", async ({ page }) => {
    await mockAuthProviderApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await page.getByTestId("provider-edit-google").click();

    // Setup instructions should be visible with console URL
    await expect(page.getByTestId("provider-setup-instructions")).toBeVisible();
    await expect(page.getByTestId("provider-setup-instructions")).toContainText(
      /console\.cloud\.google\.com/,
    );
    // Redirect URI format
    await expect(page.getByTestId("provider-setup-instructions")).toContainText(
      /\/oauth\/google\/callback/,
    );
  });

  // ---------------------------------------------------------------
  // Microsoft-specific: tenant ID field
  // ---------------------------------------------------------------

  test("edit microsoft: shows tenant ID field", async ({ page }) => {
    const apis = await mockAuthProviderApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await page.getByTestId("provider-edit-microsoft").click();

    // Tenant ID field should be visible for microsoft
    await expect(page.getByTestId("provider-form-tenant-id")).toBeVisible();

    // Fill and save
    await page.getByTestId("provider-form-enabled").check();
    await page.getByTestId("provider-form-client-id").fill("ms-client");
    await page.getByTestId("provider-form-client-secret").fill("ms-secret");
    await page.getByTestId("provider-form-tenant-id").fill("contoso.onmicrosoft.com");
    await page.getByTestId("provider-form-save").click();

    await expect(page.getByText(/Provider "microsoft" updated/i)).toBeVisible();
    expect(apis.lastUpdateBody).toMatchObject({
      tenant_id: "contoso.onmicrosoft.com",
    });
  });

  // ---------------------------------------------------------------
  // Apple-specific: team/key/private-key fields
  // ---------------------------------------------------------------

  test("edit apple: shows Apple-specific fields", async ({ page }) => {
    const apis = await mockAuthProviderApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: "OAuth Providers" })).toBeVisible();

    await page.getByTestId("provider-edit-apple").click();

    // Apple-specific fields visible
    await expect(page.getByTestId("provider-form-team-id")).toBeVisible();
    await expect(page.getByTestId("provider-form-key-id")).toBeVisible();
    await expect(page.getByTestId("provider-form-private-key")).toBeVisible();

    // Apple should NOT show Client Secret field (uses JWT instead)
    await expect(page.getByTestId("provider-form-client-secret")).toBeHidden();

    // Fill and save
    await page.getByTestId("provider-form-enabled").check();
    await page.getByTestId("provider-form-client-id").fill("com.example.service");
    await page.getByTestId("provider-form-team-id").fill("TEAM123");
    await page.getByTestId("provider-form-key-id").fill("KEY456");
    await page.getByTestId("provider-form-private-key").fill("-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----");
    await page.getByTestId("provider-form-save").click();

    await expect(page.getByText(/Provider "apple" updated/i)).toBeVisible();
    expect(apis.lastUpdateBody).toMatchObject({
      team_id: "TEAM123",
      key_id: "KEY456",
      private_key: "-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----",
    });
  });
});
