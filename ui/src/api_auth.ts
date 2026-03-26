import type {
  AuthSettings,
  OAuthProviderInfo,
  OAuthProviderListResponse,
  UpdateAuthProviderRequest,
  TestProviderResult,
  AuthTokens,
  TOTPEnrollment,
  MFAFactor,
} from "./types";
import {
  request,
  requestAuth,
  requestNoBody,
  setToken,
  setAuthToken,
} from "./api_client";

async function requestAndStoreAuthToken(
  path: string,
  init?: RequestInit,
  includeAuthHeader = true,
): Promise<AuthTokens> {
  const tokens = await requestAuth<AuthTokens>(path, init, includeAuthHeader);
  setAuthToken(tokens.token);
  return tokens;
}

// --- Admin Auth ---

export async function getAdminStatus(): Promise<{ auth: boolean }> {
  return request("/api/admin/status");
}

export async function adminLogin(password: string): Promise<string> {
  const res = await request<{ token: string }>("/api/admin/auth", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ password }),
  });
  setToken(res.token);
  return res.token;
}

// --- Auth Settings & Providers ---

export async function getAuthSettings(): Promise<AuthSettings> {
  return request("/api/admin/auth-settings");
}

export async function updateAuthSettings(settings: AuthSettings): Promise<AuthSettings> {
  return request("/api/admin/auth-settings", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(settings),
  });
}

export async function getAuthProviders(): Promise<OAuthProviderListResponse> {
  return request("/api/admin/auth/providers");
}

export async function updateAuthProvider(
  provider: string,
  payload: UpdateAuthProviderRequest,
): Promise<OAuthProviderInfo> {
  return request(`/api/admin/auth/providers/${encodeURIComponent(provider)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export async function deleteAuthProvider(provider: string): Promise<void> {
  return requestNoBody(`/api/admin/auth/providers/${encodeURIComponent(provider)}`, {
    method: "DELETE",
  });
}

export async function testAuthProvider(provider: string): Promise<TestProviderResult> {
  return request(`/api/admin/auth/providers/${encodeURIComponent(provider)}/test`, {
    method: "POST",
  });
}

// --- Auth Flows ---

export async function createAnonymousSession(): Promise<AuthTokens> {
  return requestAndStoreAuthToken("/api/auth/anonymous", { method: "POST" }, false);
}

export async function linkEmail(email: string, password: string): Promise<AuthTokens> {
  return requestAndStoreAuthToken("/api/auth/link/email", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
}

export async function linkOAuth(provider: string, accessToken: string): Promise<AuthTokens> {
  return requestAndStoreAuthToken("/api/auth/link/oauth", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ provider, access_token: accessToken }),
  });
}

// --- MFA Factor Listing ---

export async function getMFAFactors(): Promise<{ factors: MFAFactor[] }> {
  return requestAuth("/api/auth/mfa/factors");
}

// --- TOTP MFA ---

export async function enrollTOTP(): Promise<TOTPEnrollment> {
  return requestAuth("/api/auth/mfa/totp/enroll", { method: "POST" });
}

export async function confirmTOTPEnroll(code: string): Promise<{ message: string }> {
  return requestAuth("/api/auth/mfa/totp/enroll/confirm", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code }),
  });
}

export async function challengeTOTP(): Promise<{ challenge_id: string }> {
  return requestAuth("/api/auth/mfa/totp/challenge", { method: "POST" });
}

export async function verifyTOTP(challengeId: string, code: string): Promise<AuthTokens> {
  return requestAndStoreAuthToken("/api/auth/mfa/totp/verify", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ challenge_id: challengeId, code }),
  });
}

// --- SMS MFA ---

export async function challengeSMSMFA(): Promise<{ message: string }> {
  return requestAuth("/api/auth/mfa/sms/challenge", { method: "POST" });
}

export async function verifySMSMFA(code: string): Promise<AuthTokens> {
  return requestAndStoreAuthToken("/api/auth/mfa/sms/verify", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code }),
  });
}

// --- Email MFA ---

export async function enrollEmailMFA(): Promise<{ message: string }> {
  return requestAuth("/api/auth/mfa/email/enroll", { method: "POST" });
}

export async function confirmEmailMFAEnroll(code: string): Promise<{ message: string }> {
  return requestAuth("/api/auth/mfa/email/enroll/confirm", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code }),
  });
}

export async function challengeEmailMFA(): Promise<{ challenge_id: string }> {
  return requestAuth("/api/auth/mfa/email/challenge", { method: "POST" });
}

export async function verifyEmailMFA(challengeId: string, code: string): Promise<AuthTokens> {
  return requestAndStoreAuthToken("/api/auth/mfa/email/verify", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ challenge_id: challengeId, code }),
  });
}

// --- Backup Codes ---

export async function generateBackupCodes(): Promise<{ codes: string[] }> {
  return requestAuth("/api/auth/mfa/backup/generate", { method: "POST" });
}

export async function regenerateBackupCodes(): Promise<{ codes: string[] }> {
  return requestAuth("/api/auth/mfa/backup/regenerate", { method: "POST" });
}

export async function getBackupCodeCount(): Promise<{ remaining: number }> {
  return requestAuth("/api/auth/mfa/backup/count");
}

export async function verifyBackupCode(code: string): Promise<AuthTokens> {
  return requestAndStoreAuthToken("/api/auth/mfa/backup/verify", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code }),
  });
}
