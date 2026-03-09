import { AYBClient } from "@allyourbase/js";

const TOKEN_KEY = "ayb_token";
const REFRESH_KEY = "ayb_refresh_token";
const EMAIL_KEY = "ayb_email";

export const ayb = new AYBClient(
  import.meta.env.VITE_AYB_URL ?? "http://localhost:8090",
);

// Restore tokens from localStorage on load
const savedToken = localStorage.getItem(TOKEN_KEY);
const savedRefresh = localStorage.getItem(REFRESH_KEY);
if (savedToken && savedRefresh) {
  ayb.setTokens(savedToken, savedRefresh);
}

export function persistTokens(email?: string) {
  if (ayb.token && ayb.refreshToken) {
    localStorage.setItem(TOKEN_KEY, ayb.token);
    localStorage.setItem(REFRESH_KEY, ayb.refreshToken);
  }
  if (email) localStorage.setItem(EMAIL_KEY, email);
}

export function clearPersistedTokens() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_KEY);
  localStorage.removeItem(EMAIL_KEY);
}

export function getPersistedEmail(): string | null {
  return localStorage.getItem(EMAIL_KEY);
}

export function isLoggedIn(): boolean {
  return ayb.token !== null;
}
