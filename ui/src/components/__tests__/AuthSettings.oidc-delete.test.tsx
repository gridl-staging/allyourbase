import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AuthSettings } from "../AuthSettings";
import {
  deleteAuthProvider,
  getAuthProviders,
  getAuthSettings,
} from "../../api";
import type { AuthSettings as AuthSettingsType } from "../../types";

vi.mock("../../api", () => ({
  deleteAuthProvider: vi.fn(),
  getAuthProviders: vi.fn(),
  getAuthSettings: vi.fn(),
  updateAuthProvider: vi.fn(),
  updateAuthSettings: vi.fn(),
  testAuthProvider: vi.fn(),
}));

const mockDeleteAuthProvider = vi.mocked(deleteAuthProvider);
const mockGetAuthProviders = vi.mocked(getAuthProviders);
const mockGetAuthSettings = vi.mocked(getAuthSettings);

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

describe("AuthSettings OIDC deletion", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows delete button only for OIDC providers, not built-in", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [
        { name: "google", type: "builtin", enabled: true, client_id_configured: true },
        { name: "custom-oidc", type: "oidc", enabled: true, client_id_configured: true },
      ],
    });

    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByTestId("provider-row-google")).toBeVisible();
      expect(screen.getByTestId("provider-row-custom-oidc")).toBeVisible();
    });

    expect(screen.queryByTestId("provider-delete-google")).not.toBeInTheDocument();
    expect(screen.getByTestId("provider-delete-custom-oidc")).toBeVisible();
  });

  it("successful custom OIDC deletion reloads providers from server", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [
        { name: "google", type: "builtin", enabled: true, client_id_configured: true },
        { name: "custom-oidc", type: "oidc", enabled: true, client_id_configured: true },
      ],
    });
    mockDeleteAuthProvider.mockResolvedValueOnce(undefined);
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "google", type: "builtin", enabled: true, client_id_configured: true }],
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByTestId("provider-delete-custom-oidc")).toBeVisible();
    });

    await user.click(screen.getByTestId("provider-delete-custom-oidc"));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: /delete provider/i })).toBeVisible();
    await user.click(within(dialog).getByRole("button", { name: /delete/i }));

    await waitFor(() => {
      expect(mockDeleteAuthProvider).toHaveBeenCalledWith("custom-oidc");
    });

    await waitFor(() => {
      expect(mockGetAuthProviders).toHaveBeenCalledTimes(2);
    });

    await waitFor(() => {
      expect(screen.queryByTestId("provider-row-custom-oidc")).not.toBeInTheDocument();
    });

    expect(screen.getByText(/custom-oidc.*deleted/i)).toBeVisible();
  });

  it("cancel delete does not call API", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "custom-oidc", type: "oidc", enabled: true, client_id_configured: true }],
    });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByTestId("provider-delete-custom-oidc")).toBeVisible();
    });

    await user.click(screen.getByTestId("provider-delete-custom-oidc"));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: /delete provider/i })).toBeVisible();
    await user.click(within(dialog).getByRole("button", { name: /cancel/i }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });

    expect(mockDeleteAuthProvider).not.toHaveBeenCalled();
    expect(screen.getByTestId("provider-row-custom-oidc")).toBeVisible();
  });

  it("failed delete shows error and keeps row", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "custom-oidc", type: "oidc", enabled: true, client_id_configured: true }],
    });
    mockDeleteAuthProvider.mockRejectedValueOnce(new Error("Server refused deletion"));

    const user = userEvent.setup();
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByTestId("provider-delete-custom-oidc")).toBeVisible();
    });

    await user.click(screen.getByTestId("provider-delete-custom-oidc"));

    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: /delete/i }));

    await waitFor(() => {
      expect(screen.getByText(/Server refused deletion/i)).toBeVisible();
    });

    expect(screen.getByTestId("provider-row-custom-oidc")).toBeVisible();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("confirm delete shows transient deleting state while request is in flight", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "custom-oidc", type: "oidc", enabled: true, client_id_configured: true }],
    });
    let resolveDelete: (() => void) | null = null;
    mockDeleteAuthProvider.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        resolveDelete = resolve;
      }),
    );
    mockGetAuthProviders.mockResolvedValueOnce({ providers: [] });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByTestId("provider-delete-custom-oidc")).toBeVisible();
    });

    await user.click(screen.getByTestId("provider-delete-custom-oidc"));
    const dialog = await screen.findByRole("dialog");
    const deleteButton = within(dialog).getByRole("button", { name: /delete/i });
    const cancelButton = within(dialog).getByRole("button", { name: /cancel/i });

    await user.click(deleteButton);

    await waitFor(() => {
      expect(mockDeleteAuthProvider).toHaveBeenCalledWith("custom-oidc");
      expect(deleteButton).toBeDisabled();
      expect(cancelButton).toBeDisabled();
    });

    resolveDelete?.();

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(screen.queryByTestId("provider-row-custom-oidc")).not.toBeInTheDocument();
    });
  });

  it("does not show delete success when reload fails after deletion", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "custom-oidc", type: "oidc", enabled: true, client_id_configured: true }],
    });
    mockDeleteAuthProvider.mockResolvedValueOnce(undefined);
    mockGetAuthProviders.mockRejectedValueOnce(new Error("Reload after delete failed"));

    const user = userEvent.setup();
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByTestId("provider-delete-custom-oidc")).toBeVisible();
    });

    await user.click(screen.getByTestId("provider-delete-custom-oidc"));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: /delete/i }));

    await waitFor(() => {
      expect(mockDeleteAuthProvider).toHaveBeenCalledWith("custom-oidc");
      expect(screen.getByText(/Reload after delete failed/i)).toBeVisible();
    });

    expect(screen.queryByText(/custom-oidc.*deleted/i)).not.toBeInTheDocument();
  });

  it("clears stale edit state when deleting the provider being edited", async () => {
    mockGetAuthSettings.mockResolvedValueOnce(makeSettings());
    mockGetAuthProviders.mockResolvedValueOnce({
      providers: [{ name: "custom-oidc", type: "oidc", enabled: true, client_id_configured: true }],
    });
    mockDeleteAuthProvider.mockResolvedValueOnce(undefined);
    mockGetAuthProviders.mockResolvedValueOnce({ providers: [] });

    const user = userEvent.setup();
    render(<AuthSettings />);

    await waitFor(() => {
      expect(screen.getByTestId("provider-edit-custom-oidc")).toBeVisible();
    });

    await user.click(screen.getByTestId("provider-edit-custom-oidc"));
    expect(screen.getByTestId("provider-form-client-id")).toBeVisible();

    await user.click(screen.getByTestId("provider-delete-custom-oidc"));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: /delete/i }));

    await waitFor(() => {
      expect(screen.queryByTestId("provider-row-custom-oidc")).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId("provider-form-client-id")).not.toBeInTheDocument();
  });
});
