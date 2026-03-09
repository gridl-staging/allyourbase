import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockApiError } from "../../test-utils";

vi.mock("../../api", () => ({
  checkOAuthAuthorize: vi.fn(),
  submitOAuthConsent: vi.fn(),
  ApiError: MockApiError,
}));

import { OAuthConsent } from "../OAuthConsent";
import { checkOAuthAuthorize, submitOAuthConsent, ApiError } from "../../api";
import type { OAuthConsentPrompt, OAuthConsentResult } from "../../api";

const mockCheckAuthorize = vi.mocked(checkOAuthAuthorize);
const mockSubmitConsent = vi.mocked(submitOAuthConsent);

// Mock window.location for testing URL params.
const originalLocation = window.location;

function setSearchParams(params: Record<string, string>) {
  const qs = new URLSearchParams(params).toString();
  Object.defineProperty(window, "location", {
    value: {
      ...originalLocation,
      pathname: "/oauth/authorize",
      search: `?${qs}`,
      href: `http://localhost/oauth/authorize?${qs}`,
    },
    writable: true,
  });
}

function makeConsentPrompt(
  overrides: Partial<OAuthConsentPrompt> = {},
): OAuthConsentPrompt {
  return {
    requires_consent: true,
    client_id: "ayb_cid_abc123def456abc123def456abc123def456abc123def456",
    client_name: "My Third-Party App",
    redirect_uri: "https://example.com/callback",
    scope: "readonly",
    state: "random-state-123",
    code_challenge: "challenge123",
    code_challenge_method: "S256",
    ...overrides,
  };
}

describe("OAuthConsent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setSearchParams({
      response_type: "code",
      client_id: "ayb_cid_abc123def456abc123def456abc123def456abc123def456",
      redirect_uri: "https://example.com/callback",
      scope: "readonly",
      state: "random-state-123",
      code_challenge: "challenge123",
      code_challenge_method: "S256",
    });
  });

  afterAll(() => {
    Object.defineProperty(window, "location", {
      value: originalLocation,
      writable: true,
    });
  });

  it("shows loading state while checking authorization", () => {
    mockCheckAuthorize.mockReturnValue(new Promise(() => {}));
    render(<OAuthConsent />);
    expect(screen.getByText("Checking authorization...")).toBeInTheDocument();
  });

  it("shows error when authorization check fails", async () => {
    mockCheckAuthorize.mockRejectedValueOnce(new Error("server error"));
    render(<OAuthConsent />);

    await waitFor(() => {
      expect(screen.getByText("server error")).toBeInTheDocument();
    });
  });

  it("renders consent form with app name", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(
      makeConsentPrompt({ client_name: "My Third-Party App" }),
    );
    render(<OAuthConsent />);

    await waitFor(() => {
      expect(screen.getByText("My Third-Party App")).toBeInTheDocument();
    });
  });

  it("shows human-readable readonly scope description", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(
      makeConsentPrompt({ scope: "readonly" }),
    );
    render(<OAuthConsent />);

    await waitFor(() => {
      expect(screen.getByText("Read your data")).toBeInTheDocument();
    });
  });

  it("shows human-readable readwrite scope description", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(
      makeConsentPrompt({ scope: "readwrite" }),
    );
    render(<OAuthConsent />);

    await waitFor(() => {
      expect(screen.getByText("Read and modify your data")).toBeInTheDocument();
    });
  });

  it("shows human-readable full access scope description", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(
      makeConsentPrompt({ scope: "*" }),
    );
    render(<OAuthConsent />);

    await waitFor(() => {
      expect(screen.getByText("Full access to your account")).toBeInTheDocument();
    });
  });

  it("shows allowed tables when specified", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(
      makeConsentPrompt({ allowed_tables: ["posts", "comments"] }),
    );
    render(<OAuthConsent />);

    await waitFor(() => {
      expect(screen.getByText(/posts/)).toBeInTheDocument();
      expect(screen.getByText(/comments/)).toBeInTheDocument();
    });
  });

  it("shows approve and deny buttons", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(makeConsentPrompt());
    render(<OAuthConsent />);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Approve" }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Deny" }),
      ).toBeInTheDocument();
    });
  });

  it("approve button submits consent with decision approve", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(makeConsentPrompt());
    mockSubmitConsent.mockResolvedValueOnce({
      redirect_to: "https://example.com/callback?code=abc&state=random-state-123",
    });

    // Mock window.location.assign for redirect
    const assignMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: { ...window.location, assign: assignMock },
      writable: true,
    });

    render(<OAuthConsent />);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Approve" }),
      ).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(mockSubmitConsent).toHaveBeenCalledWith(
        expect.objectContaining({
          decision: "approve",
          client_id: "ayb_cid_abc123def456abc123def456abc123def456abc123def456",
          scope: "readonly",
          state: "random-state-123",
        }),
      );
    });
  });

  it("deny button submits consent with decision deny", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(makeConsentPrompt());
    mockSubmitConsent.mockResolvedValueOnce({
      redirect_to:
        "https://example.com/callback?error=access_denied&state=random-state-123",
    });

    const assignMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: { ...window.location, assign: assignMock },
      writable: true,
    });

    render(<OAuthConsent />);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Deny" }),
      ).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Deny" }));

    await waitFor(() => {
      expect(mockSubmitConsent).toHaveBeenCalledWith(
        expect.objectContaining({
          decision: "deny",
        }),
      );
    });
  });

  it("redirects to redirect_to URL after consent", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(makeConsentPrompt());
    const redirectUrl =
      "https://example.com/callback?code=abc123&state=random-state-123";
    mockSubmitConsent.mockResolvedValueOnce({ redirect_to: redirectUrl });

    const assignMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: { ...window.location, assign: assignMock },
      writable: true,
    });

    render(<OAuthConsent />);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Approve" }),
      ).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(assignMock).toHaveBeenCalledWith(redirectUrl);
    });
  });

  it("shows redirect_to when consent already exists", async () => {
    const redirectUrl =
      "https://example.com/callback?code=existing&state=random-state-123";
    mockCheckAuthorize.mockResolvedValueOnce(
      makeConsentPrompt({
        requires_consent: false,
        redirect_to: redirectUrl,
      }),
    );

    const assignMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: { ...window.location, assign: assignMock },
      writable: true,
    });

    render(<OAuthConsent />);

    await waitFor(() => {
      expect(assignMock).toHaveBeenCalledWith(redirectUrl);
    });
  });

  it("shows authorization heading", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(makeConsentPrompt());
    render(<OAuthConsent />);

    await waitFor(() => {
      expect(screen.getByText("Authorization Request")).toBeInTheDocument();
    });
  });

  it("shows app wants access message", async () => {
    mockCheckAuthorize.mockResolvedValueOnce(
      makeConsentPrompt({ client_name: "Cool App" }),
    );
    render(<OAuthConsent />);

    await waitFor(() => {
      expect(screen.getByText("Cool App")).toBeInTheDocument();
      expect(
        screen.getByText(/wants access to your account/),
      ).toBeInTheDocument();
    });
  });

  it("shows error when missing required URL params", async () => {
    Object.defineProperty(window, "location", {
      value: { ...originalLocation, search: "", href: "http://localhost/oauth/authorize" },
      writable: true,
    });

    render(<OAuthConsent />);

    await waitFor(() => {
      expect(screen.getByText(/Missing required parameters/)).toBeInTheDocument();
    });
  });

  it("redirects to login with return_to when user is unauthenticated (401)", async () => {
    const assignMock = vi.fn();
    const authorizeHref =
      "http://localhost/oauth/authorize?response_type=code&client_id=ayb_cid_abc123def456abc123def456abc123def456abc123def456&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback&scope=readonly&state=random-state-123&code_challenge=challenge123&code_challenge_method=S256";
    Object.defineProperty(window, "location", {
      value: {
        ...originalLocation,
        pathname: "/oauth/authorize",
        search: new URL(authorizeHref).search,
        href: authorizeHref,
        assign: assignMock,
      },
      writable: true,
    });

    mockCheckAuthorize.mockRejectedValueOnce(
      new ApiError(401, "authenticated user session is required"),
    );

    render(<OAuthConsent />);

    await waitFor(() => {
      expect(assignMock).toHaveBeenCalledTimes(1);
    });

    const redirectUrl = assignMock.mock.calls[0][0] as string;
    expect(redirectUrl).toMatch(/^\/\?return_to=/);
    // The return_to value should contain the full authorize URL.
    const returnTo = decodeURIComponent(
      redirectUrl.replace(/^\/\?return_to=/, ""),
    );
    expect(returnTo.startsWith("/oauth/authorize?")).toBe(true);
    expect(returnTo.startsWith("http")).toBe(false);
    expect(returnTo).toContain("client_id=");
    expect(returnTo).toContain("state=random-state-123");
  });
});
