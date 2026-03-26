/**
 * @module Adapter functions for extracting and applying cookies across different frameworks (Next.js, SvelteKit, Remix).
 */
/**
 * Extracts cookies from a Next.js request object, handling both Headers API and Next.js cookies interface. If headers are available, retrieves the standard cookie header; otherwise constructs a cookie string from the ayb_token and ayb_refresh_token cookies.
 * @param request - Request object with optional headers (Headers API) or cookies interface
 * @returns Cookie header string, or empty string if no cookies are found
 */
export function nextCookieHeader(request: {
  headers?: Headers;
  cookies?: { get(name: string): { value: string } | undefined };
}): string {
  if (request.headers) {
    return request.headers.get("cookie") ?? "";
  }

  if (request.cookies) {
    const token = request.cookies.get("ayb_token")?.value;
    const refreshToken = request.cookies.get("ayb_refresh_token")?.value;
    const parts = [];
    if (token) parts.push(`ayb_token=${token}`);
    if (refreshToken) parts.push(`ayb_refresh_token=${refreshToken}`);
    return parts.join("; ");
  }

  return "";
}

export function applyNextSetCookies(response: { headers: Headers }, setCookieHeaders: string[]): void {
  for (const value of setCookieHeaders) {
    response.headers.append("set-cookie", value);
  }
}

export function svelteKitCookieHeader(event: { request: Request }): string {
  return event.request.headers.get("cookie") ?? "";
}

export function applySvelteKitSetCookies(headers: Headers, setCookieHeaders: string[]): void {
  for (const value of setCookieHeaders) {
    headers.append("set-cookie", value);
  }
}

export function remixCookieHeader(request: Request): string {
  return request.headers.get("cookie") ?? "";
}

export function remixSetCookiesHeaders(setCookieHeaders: string[]): Headers {
  const headers = new Headers();
  for (const value of setCookieHeaders) {
    headers.append("set-cookie", value);
  }
  return headers;
}
