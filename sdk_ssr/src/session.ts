/**
 * @module Loads and validates server-side sessions from cookies, handling token refresh and maintaining session state.
 */
import { clearSessionCookies, getSessionTokens, serializeCookie } from "./cookies";
import type { CookieOptions, SessionLoadResult, SSRClientLike } from "./types";

function isAuthError(err: unknown): boolean {
  const status = (err as { status?: number })?.status;
  return status === 401 || status === 403;
}

/**
 * Validates or refreshes server-side session from cookies, returning authenticated session data with user info and Set-Cookie headers, or a null session if authentication fails.
 * 
 * @param input - Object with cookieHeader (HTTP cookie string), client (SSR auth client), and optional cookieOptions.
 * @returns Session data with authenticated user and Set-Cookie headers if successful, or null session with clear-cookie headers if authentication fails.
 * @throws Re-throws non-authentication errors from client operations.
 */
export async function loadServerSession(input: {
  cookieHeader: string;
  client: SSRClientLike;
  cookieOptions?: CookieOptions;
}): Promise<SessionLoadResult> {
  const { cookieHeader, client, cookieOptions } = input;
  const { token, refreshToken } = getSessionTokens(cookieHeader, cookieOptions);

  if (!token || !refreshToken) {
    return { session: null, setCookieHeaders: [] };
  }

  client.setTokens(token, refreshToken);

  try {
    const user = await client.auth.me();
    return {
      session: { token, refreshToken, user },
      setCookieHeaders: [],
    };
  } catch (meErr) {
    if (!isAuthError(meErr)) {
      throw meErr;
    }
  }

  try {
    const refreshed = await client.auth.refresh();
    client.setTokens(refreshed.token, refreshed.refreshToken);

    const user = refreshed.user ?? (await client.auth.me());
    const setCookieHeaders = [
      serializeCookie(cookieOptions?.accessTokenName ?? "ayb_token", refreshed.token, cookieOptions),
      serializeCookie(cookieOptions?.refreshTokenName ?? "ayb_refresh_token", refreshed.refreshToken, cookieOptions),
    ];

    return {
      session: {
        token: refreshed.token,
        refreshToken: refreshed.refreshToken,
        user,
      },
      setCookieHeaders,
    };
  } catch {
    client.clearTokens();
    return {
      session: null,
      setCookieHeaders: clearSessionCookies(cookieOptions),
    };
  }
}

export async function loadServerUser(input: {
  cookieHeader: string;
  client: SSRClientLike;
  cookieOptions?: CookieOptions;
}): Promise<Record<string, unknown> | null> {
  const result = await loadServerSession(input);
  return result.session?.user ?? null;
}
