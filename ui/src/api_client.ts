/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar21_03_dashboard_dx_polish_and_e2e_playwright_journeys/allyourbase_dev/ui/src/api_client.ts.
 */
const ADMIN_TOKEN_KEY = "ayb_admin_token";
const AUTH_TOKEN_KEY = "ayb_auth_token";

/**
 * Converts various header formats to a plain object record.
 * @param headersInit - Headers to normalize, supporting Headers instances, header tuples, or plain objects.
 * @returns A plain object mapping header names to values.
 */
function normalizeHeaders(headersInit?: HeadersInit): Record<string, string> {
  if (!headersInit) {
    return {};
  }
  if (headersInit instanceof Headers) {
    const headers: Record<string, string> = {};
    headersInit.forEach((value, key) => {
      headers[key] = value;
    });
    return headers;
  }
  if (Array.isArray(headersInit)) {
    return Object.fromEntries(headersInit);
  }
  return { ...headersInit };
}

export function getAdminToken(): string | null {
  return localStorage.getItem(ADMIN_TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(ADMIN_TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(ADMIN_TOKEN_KEY);
}

export function getAuthToken(): string | null {
  return localStorage.getItem(AUTH_TOKEN_KEY);
}

export function setAuthToken(token: string) {
  localStorage.setItem(AUTH_TOKEN_KEY, token);
}

export function clearAuthToken() {
  localStorage.removeItem(AUTH_TOKEN_KEY);
}

function dispatchUnauthorizedEvent(eventName: string, clearTokenFn: () => void) {
  clearTokenFn();
  window.dispatchEvent(new Event(eventName));
}

export function emitUnauthorized() {
  dispatchUnauthorizedEvent("ayb:unauthorized", clearToken);
}

function authorizedHeaders(
  headersInit: HeadersInit | undefined,
  token: string | null,
): Record<string, string> {
  const headers = normalizeHeaders(headersInit);
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  return headers;
}

function sameOriginRequestPath(path: string): string {
  if (/^[a-zA-Z][a-zA-Z\\d+.-]*:/.test(path) || path.startsWith("//")) {
    const requestURL = new URL(path, window.location.href);
    if (requestURL.origin !== window.location.origin) {
      throw new Error("Cross-origin API requests are not allowed");
    }
    return `${requestURL.pathname}${requestURL.search}${requestURL.hash}`;
  }
  return path;
}

function fetchWithToken(path: string, init: RequestInit | undefined, token: string | null): Promise<Response> {
  return fetch(sameOriginRequestPath(path), {
    ...init,
    headers: authorizedHeaders(init?.headers, token),
  });
}

/**
 * TODO: Document throwResponseError.
 */
async function throwResponseError(
  res: Response,
  onUnauthorized?: () => void,
): Promise<never> {
  if (res.status === 401) {
    onUnauthorized?.();
  }
  const body = await res.json().catch(() => ({ message: res.statusText }));
  const retryAfterHeader = res.headers.get("Retry-After");
  const parsedRetryAfterSeconds =
    retryAfterHeader !== null ? Number.parseInt(retryAfterHeader, 10) : undefined;
  const retryAfterSeconds =
    parsedRetryAfterSeconds !== undefined && Number.isFinite(parsedRetryAfterSeconds) && parsedRetryAfterSeconds > 0
      ? parsedRetryAfterSeconds
      : undefined;
  throw new ApiError(
    res.status,
    body.message || res.statusText,
    retryAfterSeconds,
  );
}

export async function fetchAdmin(path: string, init?: RequestInit): Promise<Response> {
  return fetchWithToken(path, init, getAdminToken());
}

export async function throwApiError(res: Response): Promise<never> {
  if (!res.ok) {
    return throwResponseError(res, emitUnauthorized);
  }
  throw new Error("throwApiError called with an ok response");
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetchAdmin(path, init);
  if (!res.ok) {
    await throwApiError(res);
  }
  return res.json();
}

export async function requestNoBody(path: string, init?: RequestInit): Promise<void> {
  const res = await fetchAdmin(path, init);
  if (!res.ok) {
    await throwApiError(res);
  }
}

export async function requestAuth<T>(
  path: string,
  init?: RequestInit,
  includeAuthHeader = true,
): Promise<T> {
  const token = includeAuthHeader ? getAuthToken() : null;
  const res = await fetchWithToken(path, init, token);
  if (!res.ok) {
    return throwResponseError(res, () => dispatchUnauthorizedEvent("ayb:auth-unauthorized", clearAuthToken));
  }
  return res.json();
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public retryAfterSeconds?: number,
  ) {
    super(message);
  }
}
