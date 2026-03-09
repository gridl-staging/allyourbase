import { AYBClient } from "@allyourbase/js";

const url = import.meta.env.VITE_AYB_URL ?? "http://localhost:8090";
export const ayb = new AYBClient(url);

const EMAIL_KEY = "ayb_email";

// Restore persisted tokens on load.
const token = localStorage.getItem("ayb_token");
const refresh = localStorage.getItem("ayb_refresh_token");
if (token && refresh) {
  ayb.setTokens(token, refresh);
}

export function persistTokens(email?: string) {
  if (ayb.token) localStorage.setItem("ayb_token", ayb.token);
  if (ayb.refreshToken) localStorage.setItem("ayb_refresh_token", ayb.refreshToken);
  if (email) localStorage.setItem(EMAIL_KEY, email);
}

export function clearPersistedTokens() {
  localStorage.removeItem("ayb_token");
  localStorage.removeItem("ayb_refresh_token");
  localStorage.removeItem(EMAIL_KEY);
  ayb.clearTokens();
}

export function getPersistedEmail(): string | null {
  return localStorage.getItem(EMAIL_KEY);
}

export function isLoggedIn(): boolean {
  return !!ayb.token;
}
