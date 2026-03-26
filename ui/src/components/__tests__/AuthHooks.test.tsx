import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { AuthHooks } from "../AuthHooks";

vi.mock("../../api_auth_hooks", () => ({
  getAuthHooks: vi.fn(),
}));

import * as api from "../../api_auth_hooks";

const mockHooks = {
  before_sign_up: "validate_email",
  after_sign_up: "provision_workspace",
  custom_access_token: "",
  before_password_reset: "",
  send_email: "custom_mailer",
  send_sms: "",
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.getAuthHooks as ReturnType<typeof vi.fn>).mockResolvedValue(mockHooks);
});

describe("AuthHooks", () => {
  it("displays 6 hook slots with function name or not configured", async () => {
    renderWithProviders(<AuthHooks />);
    await waitFor(() => {
      expect(screen.getByText("validate_email")).toBeInTheDocument();
    });
    expect(screen.getByText("provision_workspace")).toBeInTheDocument();
    expect(screen.getByText("custom_mailer")).toBeInTheDocument();
    // 3 empty slots should show "Not configured"
    const notConfigured = screen.getAllByText("Not configured");
    expect(notConfigured.length).toBe(3);
  });

  it("shows error state on fetch failure", async () => {
    (api.getAuthHooks as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("Auth error"));
    renderWithProviders(<AuthHooks />);
    await waitFor(() => {
      expect(screen.getByText("Auth error")).toBeInTheDocument();
    });
  });
});
