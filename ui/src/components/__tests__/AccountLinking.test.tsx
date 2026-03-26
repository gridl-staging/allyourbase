import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AccountLinking } from "../AccountLinking";
import { createAnonymousSession, getAuthToken, linkEmail } from "../../api";
import type { AuthTokens } from "../../types";

vi.mock("../../api", () => ({
  createAnonymousSession: vi.fn(),
  getAuthToken: vi.fn(),
  linkEmail: vi.fn(),
}));

const mockCreateAnonymousSession = vi.mocked(createAnonymousSession);
const mockGetAuthToken = vi.mocked(getAuthToken);
const mockLinkEmail = vi.mocked(linkEmail);

const TOKENS: AuthTokens = {
  token: "access-token-123",
  refreshToken: "refresh-token-456",
  user: {
    id: "user-1",
    email: "linked@test.com",
    is_anonymous: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
};

describe("AccountLinking", () => {
  const onLinked = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockGetAuthToken.mockReturnValue("existing-user-token");
  });

  it("requires starting an anonymous session before linking", async () => {
    mockGetAuthToken.mockReturnValue(null);
    mockCreateAnonymousSession.mockResolvedValue({
      ...TOKENS,
      user: {
        ...TOKENS.user,
        email: "",
        is_anonymous: true,
      },
    });
    mockLinkEmail.mockResolvedValue(TOKENS);
    const user = userEvent.setup();
    render(<AccountLinking onLinked={onLinked} />);

    expect(screen.getByRole("button", { name: /start anonymous session/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /link account/i })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: /start anonymous session/i }));
    await waitFor(() => {
      expect(mockCreateAnonymousSession).toHaveBeenCalledTimes(1);
    });

    await user.type(screen.getByTestId("link-email-input"), "linked@test.com");
    await user.type(screen.getByTestId("link-password-input"), "securepass123");
    await user.click(screen.getByRole("button", { name: /link account/i }));

    await waitFor(() => {
      expect(mockLinkEmail).toHaveBeenCalledWith("linked@test.com", "securepass123");
    });
  });

  it("renders heading and email/password form", () => {
    render(<AccountLinking onLinked={onLinked} />);
    expect(screen.getByRole("heading", { name: /link your account/i })).toBeInTheDocument();
    expect(screen.getByTestId("link-email-input")).toBeInTheDocument();
    expect(screen.getByTestId("link-password-input")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /link account/i })).toBeInTheDocument();
  });

  it("submits email and password and calls onLinked on success", async () => {
    mockLinkEmail.mockResolvedValue(TOKENS);
    const user = userEvent.setup();
    render(<AccountLinking onLinked={onLinked} />);

    await user.type(screen.getByTestId("link-email-input"), "linked@test.com");
    await user.type(screen.getByTestId("link-password-input"), "securepass123");
    await user.click(screen.getByRole("button", { name: /link account/i }));

    await waitFor(() => {
      expect(mockLinkEmail).toHaveBeenCalledWith("linked@test.com", "securepass123");
    });
    await waitFor(() => {
      expect(onLinked).toHaveBeenCalledWith(TOKENS);
    });
  });

  it("shows error when email is already taken (409)", async () => {
    mockLinkEmail.mockRejectedValue(new Error("email already belongs to another account"));
    const user = userEvent.setup();
    render(<AccountLinking onLinked={onLinked} />);

    await user.type(screen.getByTestId("link-email-input"), "taken@test.com");
    await user.type(screen.getByTestId("link-password-input"), "pass123");
    await user.click(screen.getByRole("button", { name: /link account/i }));

    await waitFor(() => {
      expect(screen.getByText(/already belongs to another account/i)).toBeInTheDocument();
    });
    expect(onLinked).not.toHaveBeenCalled();
  });

  it("shows error when not anonymous (403)", async () => {
    mockLinkEmail.mockRejectedValue(new Error("only anonymous accounts can link credentials"));
    const user = userEvent.setup();
    render(<AccountLinking onLinked={onLinked} />);

    await user.type(screen.getByTestId("link-email-input"), "test@test.com");
    await user.type(screen.getByTestId("link-password-input"), "pass123");
    await user.click(screen.getByRole("button", { name: /link account/i }));

    await waitFor(() => {
      expect(screen.getByText(/only anonymous accounts/i)).toBeInTheDocument();
    });
  });

  it("disables submit button when fields are empty", () => {
    render(<AccountLinking onLinked={onLinked} />);
    expect(screen.getByRole("button", { name: /link account/i })).toBeDisabled();
  });

  it("shows loading state during submission", async () => {
    mockLinkEmail.mockReturnValue(new Promise(() => {})); // never resolves
    const user = userEvent.setup();
    render(<AccountLinking onLinked={onLinked} />);

    await user.type(screen.getByTestId("link-email-input"), "test@test.com");
    await user.type(screen.getByTestId("link-password-input"), "pass123");
    await user.click(screen.getByRole("button", { name: /link account/i }));

    expect(screen.getByRole("button", { name: /linking/i })).toBeDisabled();
  });

  it("shows success message after linking", async () => {
    mockLinkEmail.mockResolvedValue(TOKENS);
    const user = userEvent.setup();
    render(<AccountLinking onLinked={onLinked} />);

    await user.type(screen.getByTestId("link-email-input"), "linked@test.com");
    await user.type(screen.getByTestId("link-password-input"), "securepass123");
    await user.click(screen.getByRole("button", { name: /link account/i }));

    await waitFor(() => {
      expect(screen.getByText(/account linked/i)).toBeInTheDocument();
    });
  });
});
