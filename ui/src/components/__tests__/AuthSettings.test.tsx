import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AuthSettings } from "../AuthSettings";
import {
  getAuthProviders,
  getAuthSettings,
  updateAuthProvider,
  updateAuthSettings,
  testAuthProvider,
} from "../../api";
import type { AuthSettings as AuthSettingsType, OAuthProviderInfo } from "../../types";

vi.mock("../../api", () => ({
  getAuthProviders: vi.fn(),
  getAuthSettings: vi.fn(),
  updateAuthProvider: vi.fn(),
  updateAuthSettings: vi.fn(),
  testAuthProvider: vi.fn(),
}));

const mockGetAuthProviders = vi.mocked(getAuthProviders);
const mockGetAuthSettings = vi.mocked(getAuthSettings);
const mockUpdateAuthProvider = vi.mocked(updateAuthProvider);
const mockUpdateAuthSettings = vi.mocked(updateAuthSettings);
const mockTestAuthProvider = vi.mocked(testAuthProvider);
type User = ReturnType<typeof userEvent.setup>;

function makeSettings(overrides: Partial<AuthSettingsType> = {}): AuthSettingsType {
  return {
    magic_link_enabled: false,
    sms_enabled: false,
    email_mfa_enabled: false,
    anonymous_auth_enabled: false,
    totp_enabled: false,
    ...overrides,
  };
}

function makeProviders(overrides: Partial<OAuthProviderInfo>[] = []): OAuthProviderInfo[] {
  const base: OAuthProviderInfo[] = [
    { name: "google", type: "builtin", enabled: true, client_id_configured: true },
    { name: "discord", type: "builtin", enabled: false, client_id_configured: false },
    { name: "custom-oidc", type: "oidc", enabled: true, client_id_configured: false },
  ];
  return base.map((provider, index) => ({ ...provider, ...(overrides[index] || {}) }));
}

async function openProviderEditor(user: User, providerName: string) {
  await waitFor(() => {
    expect(screen.getByTestId(`provider-edit-${providerName}`)).toBeVisible();
  });
  await user.click(screen.getByTestId(`provider-edit-${providerName}`));
}

async function openOIDCProviderForm(user: User) {
  await waitFor(() => {
    expect(screen.getByTestId("add-oidc-provider")).toBeVisible();
  });
  await user.click(screen.getByTestId("add-oidc-provider"));
}

describe("AuthSettings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetAuthProviders.mockResolvedValue({ providers: makeProviders() });
  });

  it("renders heading", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    render(<AuthSettings />);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /Auth Settings/i })).toBeVisible();
    });
  });

  it("shows loading state", () => {
    mockGetAuthSettings.mockReturnValue(new Promise(() => {}));
    render(<AuthSettings />);
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("shows error state on fetch failure", async () => {
    mockGetAuthSettings.mockRejectedValueOnce(new Error("Network error"));
    render(<AuthSettings />);
    await waitFor(() => {
      expect(screen.getByText(/Network error/)).toBeInTheDocument();
    });
  });

  it("displays all feature toggles with correct initial state", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(
      makeSettings({ totp_enabled: true, anonymous_auth_enabled: false }),
    );
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByText("Auth Settings")).toBeVisible();
    });

    // TOTP should be checked
    const totpToggle = screen.getByTestId("toggle-totp_enabled");
    expect(totpToggle).toBeChecked();

    // Anonymous should be unchecked
    const anonToggle = screen.getByTestId("toggle-anonymous_auth_enabled");
    expect(anonToggle).not.toBeChecked();

    // Email MFA should be unchecked
    const emailToggle = screen.getByTestId("toggle-email_mfa_enabled");
    expect(emailToggle).not.toBeChecked();

    // SMS should be unchecked
    const smsToggle = screen.getByTestId("toggle-sms_enabled");
    expect(smsToggle).not.toBeChecked();

    // Magic Link should be unchecked
    const mlToggle = screen.getByTestId("toggle-magic_link_enabled");
    expect(mlToggle).not.toBeChecked();
  });

  it("lists OAuth providers with enabled and client-id status", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: makeProviders(),
    });

    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /OAuth Providers/i })).toBeVisible();
    });

    expect(screen.getByTestId("provider-row-google")).toBeVisible();
    expect(screen.getByTestId("provider-enabled-google")).toHaveTextContent("Enabled");
    expect(screen.getByTestId("provider-client-google")).toHaveTextContent("Client ID configured");

    expect(screen.getByTestId("provider-row-discord")).toBeVisible();
    expect(screen.getByTestId("provider-enabled-discord")).toHaveTextContent("Disabled");
    expect(screen.getByTestId("provider-client-discord")).toHaveTextContent("Client ID missing");

    // Verify OIDC type label renders correctly
    expect(screen.getByTestId("provider-row-custom-oidc")).toBeVisible();
    expect(screen.getByTestId("provider-row-custom-oidc")).toHaveTextContent("OIDC");
    expect(screen.getByTestId("provider-row-google")).toHaveTextContent("Built-in");
  });

  it("shows provider section error when provider list cannot be loaded", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockRejectedValueOnce(new Error("Provider API down"));

    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByText(/Provider API down/i)).toBeVisible();
    });
  });

  it("opens provider form and saves built-in provider config", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "google", type: "builtin", enabled: false, client_id_configured: false }],
    });
    mockUpdateAuthProvider.mockResolvedValueOnce({
      name: "google",
      type: "builtin",
      enabled: true,
      client_id_configured: true,
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "google");
    await user.click(screen.getByTestId("provider-form-enabled"));
    await user.type(screen.getByTestId("provider-form-client-id"), "google-client-id");
    await user.type(screen.getByTestId("provider-form-client-secret"), "google-client-secret");
    await user.click(screen.getByTestId("provider-form-save"));

    await waitFor(() => {
      expect(mockUpdateAuthProvider).toHaveBeenCalledWith("google", {
        enabled: true,
        client_id: "google-client-id",
        client_secret: "google-client-secret",
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("provider-enabled-google")).toHaveTextContent("Enabled");
      expect(screen.getByTestId("provider-client-google")).toHaveTextContent("Client ID configured");
    });
  });

  it("sends tenant_id when editing microsoft provider", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "microsoft", type: "builtin", enabled: false, client_id_configured: false }],
    });
    mockUpdateAuthProvider.mockResolvedValueOnce({
      name: "microsoft",
      type: "builtin",
      enabled: true,
      client_id_configured: true,
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "microsoft");
    await user.click(screen.getByTestId("provider-form-enabled"));
    await user.type(screen.getByTestId("provider-form-client-id"), "microsoft-client-id");
    await user.type(screen.getByTestId("provider-form-client-secret"), "microsoft-client-secret");
    await user.type(screen.getByTestId("provider-form-tenant-id"), "contoso");
    await user.click(screen.getByTestId("provider-form-save"));

    await waitFor(() => {
      expect(mockUpdateAuthProvider).toHaveBeenCalledWith("microsoft", {
        enabled: true,
        client_id: "microsoft-client-id",
        client_secret: "microsoft-client-secret",
        tenant_id: "contoso",
      });
    });
  });

  it("sends apple team/key/private key fields when editing apple provider", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "apple", type: "builtin", enabled: false, client_id_configured: false }],
    });
    mockUpdateAuthProvider.mockResolvedValueOnce({
      name: "apple",
      type: "builtin",
      enabled: true,
      client_id_configured: true,
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "apple");
    await user.click(screen.getByTestId("provider-form-enabled"));
    await user.type(screen.getByTestId("provider-form-client-id"), "apple-services-id");
    await user.type(screen.getByTestId("provider-form-team-id"), "TEAM123");
    await user.type(screen.getByTestId("provider-form-key-id"), "KEY123");
    await user.type(screen.getByTestId("provider-form-private-key"), "-----BEGIN PRIVATE KEY-----");
    await user.click(screen.getByTestId("provider-form-save"));

    await waitFor(() => {
      expect(mockUpdateAuthProvider).toHaveBeenCalledWith("apple", {
        enabled: true,
        client_id: "apple-services-id",
        team_id: "TEAM123",
        key_id: "KEY123",
        private_key: "-----BEGIN PRIVATE KEY-----",
      });
    });
  });

  it("shows OIDC-only edit fields for OIDC providers", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [
        { name: "google", type: "builtin", enabled: true, client_id_configured: true },
        { name: "custom-oidc", type: "oidc", enabled: true, client_id_configured: true },
      ],
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "google");
    expect(screen.queryByTestId("provider-form-issuer-url")).not.toBeInTheDocument();
    expect(screen.queryByTestId("provider-form-display-name")).not.toBeInTheDocument();
    expect(screen.queryByTestId("provider-form-scopes")).not.toBeInTheDocument();

    await openProviderEditor(user, "custom-oidc");
    expect(screen.getByTestId("provider-form-issuer-url")).toBeVisible();
    expect(screen.getByTestId("provider-form-display-name")).toBeVisible();
    expect(screen.getByTestId("provider-form-scopes")).toBeVisible();
  });

  it("saves OIDC provider edits through updateAuthProvider with OIDC fields", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "custom-oidc", type: "oidc", enabled: false, client_id_configured: false }],
    });
    mockUpdateAuthProvider.mockResolvedValueOnce({
      name: "custom-oidc",
      type: "oidc",
      enabled: true,
      client_id_configured: true,
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "custom-oidc");
    await user.click(screen.getByTestId("provider-form-enabled"));
    await user.type(screen.getByTestId("provider-form-client-id"), "oidc-client-id");
    await user.type(screen.getByTestId("provider-form-client-secret"), "oidc-client-secret");
    await user.type(screen.getByTestId("provider-form-issuer-url"), " https://issuer.example.com ");
    await user.type(screen.getByTestId("provider-form-display-name"), "  Custom OIDC  ");
    await user.type(screen.getByTestId("provider-form-scopes"), "openid   profile  email");
    await user.click(screen.getByTestId("provider-form-save"));

    await waitFor(() => {
      expect(mockUpdateAuthProvider).toHaveBeenCalledWith("custom-oidc", {
        enabled: true,
        client_id: "oidc-client-id",
        client_secret: "oidc-client-secret",
        issuer_url: "https://issuer.example.com",
        display_name: "Custom OIDC",
        scopes: ["openid", "profile", "email"],
      });
    });
  });

  it("toggling a setting calls updateAuthSettings with correct payload", async () => {
    const initial = makeSettings();
    mockGetAuthSettings.mockResolvedValueOnce(initial);
    mockUpdateAuthSettings.mockResolvedValueOnce(
      makeSettings({ totp_enabled: true }),
    );

    const user = userEvent.setup();
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByTestId("toggle-totp_enabled")).toBeInTheDocument();
    });

    // Toggle TOTP on
    await user.click(screen.getByTestId("toggle-totp_enabled"));

    await waitFor(() => {
      expect(mockUpdateAuthSettings).toHaveBeenCalledWith(
        expect.objectContaining({ totp_enabled: true }),
      );
    });
  });

  it("shows success feedback after toggling", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockUpdateAuthSettings.mockResolvedValueOnce(
      makeSettings({ anonymous_auth_enabled: true }),
    );

    const user = userEvent.setup();
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByTestId("toggle-anonymous_auth_enabled")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("toggle-anonymous_auth_enabled"));

    await waitFor(() => {
      expect(screen.getByText(/Settings updated/i)).toBeInTheDocument();
    });
  });

  it("shows error feedback when update fails", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockUpdateAuthSettings.mockRejectedValueOnce(new Error("Update failed"));

    const user = userEvent.setup();
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByTestId("toggle-totp_enabled")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("toggle-totp_enabled"));

    await waitFor(() => {
      expect(screen.getByText(/Update failed/i)).toBeInTheDocument();
    });
  });

  it("displays descriptive labels for each feature", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByText("TOTP MFA")).toBeVisible();
      expect(screen.getByText("Anonymous Auth")).toBeVisible();
      expect(screen.getByText("Email MFA")).toBeVisible();
      expect(screen.getByText("SMS Auth")).toBeVisible();
      expect(screen.getByText("Magic Link")).toBeVisible();
    });
  });

  // --- Test Connection button ---

  it("shows Test Connection button in provider form and calls testAuthProvider on click", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "google", type: "builtin", enabled: true, client_id_configured: true }],
    });
    mockTestAuthProvider.mockResolvedValueOnce({
      success: true,
      provider: "google",
      message: "authorization endpoint is reachable",
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "google");
    const testBtn = screen.getByTestId("provider-form-test");
    expect(testBtn).toBeVisible();
    await user.click(testBtn);

    await waitFor(() => {
      expect(mockTestAuthProvider).toHaveBeenCalledWith("google");
    });

    await waitFor(() => {
      expect(screen.getByText(/authorization endpoint is reachable/i)).toBeVisible();
    });
  });

  it("shows error message when test connection fails", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "github", type: "builtin", enabled: true, client_id_configured: true }],
    });
    mockTestAuthProvider.mockResolvedValueOnce({
      success: false,
      provider: "github",
      error: "authorization endpoint unreachable: connection refused",
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "github");
    await user.click(screen.getByTestId("provider-form-test"));

    await waitFor(() => {
      expect(screen.getByText(/authorization endpoint unreachable/i)).toBeVisible();
    });
  });

  it("shows Testing... state while test connection is in progress", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "google", type: "builtin", enabled: true, client_id_configured: true }],
    });
    // Never resolving promise to keep the loading state
    mockTestAuthProvider.mockReturnValueOnce(new Promise(() => {}));

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "google");
    await user.click(screen.getByTestId("provider-form-test"));

    await waitFor(() => {
      expect(screen.getByTestId("provider-form-test")).toHaveTextContent("Testing...");
    });
  });

  // --- Custom OIDC provider form ---

  it("shows Add OIDC Provider button and opens OIDC form", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({ providers: makeProviders() });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openOIDCProviderForm(user);

    expect(screen.getByTestId("oidc-form-provider-name")).toBeVisible();
    expect(screen.getByTestId("oidc-form-issuer-url")).toBeVisible();
    expect(screen.getByTestId("oidc-form-client-id")).toBeVisible();
    expect(screen.getByTestId("oidc-form-client-secret")).toBeVisible();
    expect(screen.getByTestId("oidc-form-display-name")).toBeVisible();
    expect(screen.getByTestId("oidc-form-scopes")).toBeVisible();
  });

  it("saves custom OIDC provider via updateAuthProvider", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockRejectedValueOnce(new Error("Provider API down"));
    mockUpdateAuthProvider.mockResolvedValueOnce({
      name: "my-keycloak",
      type: "oidc",
      enabled: true,
      client_id_configured: true,
    });
    // Re-fetch providers after save includes the new provider
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "my-keycloak", type: "oidc", enabled: true, client_id_configured: true }],
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openOIDCProviderForm(user);
    await user.type(screen.getByTestId("oidc-form-provider-name"), "my-keycloak");
    await user.type(screen.getByTestId("oidc-form-issuer-url"), "https://keycloak.example.com/realms/main");
    await user.type(screen.getByTestId("oidc-form-client-id"), "kc-client");
    await user.type(screen.getByTestId("oidc-form-client-secret"), "kc-secret");
    await user.type(screen.getByTestId("oidc-form-display-name"), "Keycloak");
    await user.clear(screen.getByTestId("oidc-form-scopes"));
    await user.type(screen.getByTestId("oidc-form-scopes"), "openid profile email");
    await user.click(screen.getByTestId("oidc-form-save"));

    await waitFor(() => {
      expect(mockUpdateAuthProvider).toHaveBeenCalledWith("my-keycloak", {
        enabled: true,
        issuer_url: "https://keycloak.example.com/realms/main",
        client_id: "kc-client",
        client_secret: "kc-secret",
        display_name: "Keycloak",
        scopes: ["openid", "profile", "email"],
      });
    });

    // Verify the new provider appears in the list after save
    await waitFor(() => {
      expect(screen.getByTestId("provider-row-my-keycloak")).toBeVisible();
    });
    expect(screen.queryByText(/Provider API down/i)).not.toBeInTheDocument();
  });

  it("does not show OIDC success when reload fails after save", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({ providers: [] });
    mockUpdateAuthProvider.mockResolvedValueOnce({
      name: "my-keycloak",
      type: "oidc",
      enabled: true,
      client_id_configured: true,
    });
    mockGetAuthProviders.mockRejectedValueOnce(new Error("Reload failed"));

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openOIDCProviderForm(user);
    await user.type(screen.getByTestId("oidc-form-provider-name"), "my-keycloak");
    await user.type(screen.getByTestId("oidc-form-issuer-url"), "https://keycloak.example.com/realms/main");
    await user.type(screen.getByTestId("oidc-form-client-id"), "kc-client");
    await user.type(screen.getByTestId("oidc-form-client-secret"), "kc-secret");
    await user.click(screen.getByTestId("oidc-form-save"));

    await waitFor(() => {
      expect(mockUpdateAuthProvider).toHaveBeenCalledWith(
        "my-keycloak",
        expect.objectContaining({
          enabled: true,
          issuer_url: "https://keycloak.example.com/realms/main",
          client_id: "kc-client",
          client_secret: "kc-secret",
        }),
      );
    });

    await waitFor(() => {
      expect(screen.getByText(/Reload failed/i)).toBeVisible();
    });
    expect(screen.queryByText(/my-keycloak.*added/i)).not.toBeInTheDocument();
  });

  it("validates OIDC provider name is not empty", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({ providers: [] });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openOIDCProviderForm(user);
    // Leave provider name empty, fill issuer URL
    await user.type(screen.getByTestId("oidc-form-issuer-url"), "https://auth.example.com");
    await user.click(screen.getByTestId("oidc-form-save"));

    await waitFor(() => {
      expect(screen.getByText(/provider name is required/i)).toBeVisible();
    });
    expect(mockUpdateAuthProvider).not.toHaveBeenCalled();
  });

  // --- Provider setup instructions ---

  it("shows setup instructions link for google provider", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "google", type: "builtin", enabled: false, client_id_configured: false }],
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "google");
    const instructions = screen.getByTestId("provider-setup-instructions");
    expect(instructions).toBeVisible();
    expect(instructions).toHaveTextContent(/console\.cloud\.google\.com/i);
  });

  it("shows redirect URI format in setup instructions", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "github", type: "builtin", enabled: false, client_id_configured: false }],
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "github");
    const instructions = screen.getByTestId("provider-setup-instructions");
    expect(instructions).toBeVisible();
    expect(instructions).toHaveTextContent(/\/oauth\/github\/callback/);
  });

  it("does not show setup instructions for OIDC providers in built-in form", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "my-oidc", type: "oidc", enabled: true, client_id_configured: true }],
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await openProviderEditor(user, "my-oidc");
    expect(screen.queryByTestId("provider-setup-instructions")).not.toBeInTheDocument();
  });
});
