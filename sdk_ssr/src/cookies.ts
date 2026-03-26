/**
 * @module Utilities for parsing and serializing HTTP cookies with configurable token names and session token attributes.
 */
import type { CookieOptions } from "./types";

const DEFAULT_ACCESS_TOKEN_NAME = "ayb_token";
const DEFAULT_REFRESH_TOKEN_NAME = "ayb_refresh_token";

function resolveOptions(options?: CookieOptions): Required<CookieOptions> {
  return {
    accessTokenName: options?.accessTokenName ?? DEFAULT_ACCESS_TOKEN_NAME,
    refreshTokenName: options?.refreshTokenName ?? DEFAULT_REFRESH_TOKEN_NAME,
    path: options?.path ?? "/",
    domain: options?.domain ?? "",
    secure: options?.secure ?? true,
    sameSite: options?.sameSite ?? "lax",
    maxAge: options?.maxAge ?? 60 * 60 * 24 * 30,
    httpOnly: options?.httpOnly ?? true,
  };
}

/**
 * Parses a cookie header string into a key-value object.
 * @param cookieHeader - The raw cookie header value (typically from the Cookie HTTP header)
 * @returns An object mapping cookie names to their decoded values; empty object if the header is empty or malformed
 */
export function parseCookieHeader(cookieHeader: string): Record<string, string> {
  const out: Record<string, string> = {};
  if (!cookieHeader.trim()) return out;

  const pairs = cookieHeader.split(";");
  for (const pair of pairs) {
    const raw = pair.trim();
    if (!raw) continue;
    const idx = raw.indexOf("=");
    if (idx <= 0) continue;
    const key = raw.slice(0, idx).trim();
    const value = raw.slice(idx + 1).trim();
    out[key] = decodeURIComponent(value);
  }

  return out;
}

export function getSessionTokens(
  cookieHeader: string,
  options?: CookieOptions,
): { token: string | null; refreshToken: string | null } {
  const cfg = resolveOptions(options);
  const parsed = parseCookieHeader(cookieHeader);

  return {
    token: parsed[cfg.accessTokenName] ?? null,
    refreshToken: parsed[cfg.refreshTokenName] ?? null,
  };
}

export function serializeCookie(
  name: string,
  value: string,
  options?: CookieOptions,
): string {
  const cfg = resolveOptions(options);
  const parts = [`${name}=${encodeURIComponent(value)}`, `Path=${cfg.path}`, `Max-Age=${cfg.maxAge}`];

  if (cfg.domain) parts.push(`Domain=${cfg.domain}`);
  if (cfg.httpOnly) parts.push("HttpOnly");
  if (cfg.secure) parts.push("Secure");
  parts.push(`SameSite=${cfg.sameSite[0].toUpperCase()}${cfg.sameSite.slice(1)}`);

  return parts.join("; ");
}

export function clearSessionCookies(options?: CookieOptions): string[] {
  const cfg = resolveOptions(options);
  const clearOpts: CookieOptions = {
    ...cfg,
    maxAge: 0,
  };

  return [
    serializeCookie(cfg.accessTokenName, "", clearOpts),
    serializeCookie(cfg.refreshTokenName, "", clearOpts),
  ];
}
