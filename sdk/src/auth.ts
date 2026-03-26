/**
 * @module OAuth-capable authentication client supporting email/password login, token management, and both popup and redirect-based OAuth flows.
 */
import { AYBError } from "./errors";
import {
  normalizeAuthResponse,
  normalizeUser,
  openPopup,
} from "./helpers";
import type {
  AuthResponse,
  OAuthOptions,
  OAuthProvider,
  User,
} from "./types";

interface AuthClientRuntime {
  request<T>(path: string, init?: RequestInit & { skipAuth?: boolean }): Promise<T>;
  refreshToken: string | null;
  clearTokens(): void;
  setTokensInternal(token: string, refreshToken: string): void;
  emitAuthEvent(event: "SIGNED_IN" | "SIGNED_OUT" | "TOKEN_REFRESHED"): void;
  getBaseURL(): string;
}

/**
 * Manages email/password authentication, OAuth sign-in, token refresh, password reset, email verification, and account operations.
 */
export class AuthClient {
  constructor(private client: AuthClientRuntime) {}

  /** Register a new user account. */
  async register(email: string, password: string): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    const normalized = normalizeAuthResponse(res);
    this.client.setTokensInternal(normalized.token, normalized.refreshToken);
    this.client.emitAuthEvent("SIGNED_IN");
    return normalized;
  }

  /** Log in with email and password. */
  async login(email: string, password: string): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    const normalized = normalizeAuthResponse(res);
    this.client.setTokensInternal(normalized.token, normalized.refreshToken);
    this.client.emitAuthEvent("SIGNED_IN");
    return normalized;
  }

  /** Refresh the access token using the stored refresh token. */
  async refresh(): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: this.client.refreshToken }),
    });
    const normalized = normalizeAuthResponse(res);
    this.client.setTokensInternal(normalized.token, normalized.refreshToken);
    this.client.emitAuthEvent("TOKEN_REFRESHED");
    return normalized;
  }

  /** Log out (revoke the refresh token). */
  async logout(): Promise<void> {
    await this.client.request<void>("/api/auth/logout", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: this.client.refreshToken }),
    });
    this.client.clearTokens();
    this.client.emitAuthEvent("SIGNED_OUT");
  }

  /** Get the current authenticated user. */
  async me(): Promise<User> {
    const user = await this.client.request<User>("/api/auth/me");
    return normalizeUser(user);
  }

  /** Delete the current authenticated user's account. */
  async deleteAccount(): Promise<void> {
    await this.client.request<void>("/api/auth/me", { method: "DELETE" });
    this.client.clearTokens();
    this.client.emitAuthEvent("SIGNED_OUT");
  }

  /** Request a password reset email. */
  async requestPasswordReset(email: string): Promise<void> {
    await this.client.request<void>("/api/auth/password-reset", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
    });
  }

  /** Confirm a password reset with a token. */
  async confirmPasswordReset(token: string, password: string): Promise<void> {
    await this.client.request<void>("/api/auth/password-reset/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token, password }),
    });
  }

  /** Verify an email address with a token. */
  async verifyEmail(token: string): Promise<void> {
    await this.client.request<void>("/api/auth/verify", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    });
  }

  /** Resend the email verification (requires auth). */
  async resendVerification(): Promise<void> {
    await this.client.request<void>("/api/auth/verify/resend", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
    });
  }

  /**
   * Sign in with an OAuth provider using a popup + SSE flow.
   * Opens a popup immediately to avoid browser popup blockers.
   */
  /**
   * Initiate OAuth sign-in via popup or custom redirect flow. Opens popup to bypass browser blockers.
   * @param provider - OAuth provider identifier
   * @param options - Optional scopes array and urlCallback for custom redirect handling
   * @returns AuthResponse with tokens and user data
   * @throws If popup is blocked by browser or OAuth flow fails
   */
  async signInWithOAuth(
    provider: OAuthProvider,
    options?: OAuthOptions,
  ): Promise<AuthResponse> {
    let popup: Window | null = null;
    if (!options?.urlCallback) {
      popup = openPopup();
      if (!popup) {
        throw new AYBError(
          403,
          "Popup was blocked by the browser. Use urlCallback for redirect flow.",
          "oauth/popup-blocked",
        );
      }
    }

    try {
      const { clientId, waitForAuth, close } = await this.connectOAuthSSE();
      let oauthURL = `${this.client.getBaseURL()}/api/auth/oauth/${provider}?state=${clientId}`;
      if (options?.scopes?.length) {
        oauthURL += `&scopes=${encodeURIComponent(options.scopes.join(","))}`;
      }

      if (options?.urlCallback) {
        await options.urlCallback(oauthURL);
      } else if (popup) {
        popup.location.href = oauthURL;
      }

      const result = await waitForAuth(popup);
      this.client.setTokensInternal(result.token, result.refreshToken);
      this.client.emitAuthEvent("SIGNED_IN");
      close();
      return result;
    } catch (err) {
      popup?.close();
      throw err;
    }
  }

  /**
   * Parse OAuth tokens from a URL hash fragment after redirect-based OAuth.
   */
  /**
   * Parse OAuth response tokens from URL hash and apply them internally. Cleans up the URL history.
   * @returns AuthResponse with token and refreshToken if present, otherwise null
   */
  handleOAuthRedirect(): AuthResponse | null {
    if (typeof window === "undefined") return null;
    const hash = window.location.hash;
    if (!hash) return null;
    const params = new URLSearchParams(hash.slice(1));
    const token = params.get("token");
    const refreshToken = params.get("refreshToken");
    if (!token || !refreshToken) return null;
    this.client.setTokensInternal(token, refreshToken);
    this.client.emitAuthEvent("SIGNED_IN");
    window.history.replaceState(
      null,
      "",
      window.location.pathname + window.location.search,
    );
    return { token, refreshToken, user: {} as User };
  }

  /**
   * Establish an EventSource connection for OAuth authentication events.
   * @returns Object with clientId for state parameter, waitForAuth function to handle OAuth completion, and close function to clean up the connection
   * @throws If SSE connection fails to establish
   */
  private connectOAuthSSE(): Promise<{
    clientId: string;
    waitForAuth: (popup: Window | null) => Promise<AuthResponse>;
    close: () => void;
  }> {
    return new Promise((resolve, reject) => {
      const url = `${this.client.getBaseURL()}/api/realtime?oauth=true`;
      const es = new EventSource(url);
      let settled = false;

      const cleanup = () => {
        es.close();
      };

      es.addEventListener("connected", (e: MessageEvent) => {
        const data = JSON.parse(e.data) as { clientId: string };

        const waitForAuth = (popup: Window | null): Promise<AuthResponse> => {
          return new Promise<AuthResponse>((resolveAuth, rejectAuth) => {
            const timeout = setTimeout(() => {
              cleanup();
              rejectAuth(new AYBError(408, "OAuth sign-in timed out", "oauth/timeout"));
            }, 5 * 60 * 1000);

            let popupPoll: ReturnType<typeof setInterval> | undefined;
            if (popup) {
              popupPoll = setInterval(() => {
                if (popup.closed) {
                  clearInterval(popupPoll);
                  clearTimeout(timeout);
                  cleanup();
                  rejectAuth(
                    new AYBError(
                      499,
                      "OAuth popup was closed by the user",
                      "oauth/popup-closed",
                    ),
                  );
                }
              }, 500);
            }

            es.addEventListener("oauth", (oauthEvt: MessageEvent) => {
              clearTimeout(timeout);
              if (popupPoll) clearInterval(popupPoll);
              popup?.close();

              const result = JSON.parse(oauthEvt.data) as {
                token?: string;
                refreshToken?: string;
                user?: User;
                error?: string;
              };

              if (result.error) {
                cleanup();
                rejectAuth(new AYBError(401, result.error, "oauth/provider-error"));
                return;
              }

              if (!result.token || !result.refreshToken) {
                cleanup();
                rejectAuth(
                  new AYBError(500, "OAuth response missing tokens", "oauth/missing-tokens"),
                );
                return;
              }

              resolveAuth({
                token: result.token,
                refreshToken: result.refreshToken,
                user: result.user ? normalizeUser(result.user as User) : ({} as User),
              });
            });
          });
        };

        resolve({ clientId: data.clientId, waitForAuth, close: cleanup });
      });

      es.onerror = () => {
        if (!settled) {
          settled = true;
          cleanup();
          reject(
            new AYBError(
              503,
              "Failed to connect to OAuth SSE channel",
              "oauth/sse-failed",
            ),
          );
        }
      };
    });
  }
}
