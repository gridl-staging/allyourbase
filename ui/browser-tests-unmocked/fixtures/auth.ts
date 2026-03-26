/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-unmocked/fixtures/auth.ts.
 */
import type { APIRequestContext, Page } from "@playwright/test";
import { createHmac } from "crypto";
import { execSQL, probeEndpoint, sqlLiteral, validateResponse } from "./core";

/**
 * TODO: Document base32Decode.
 */
function base32Decode(encoded: string): Buffer {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const character of encoded.toUpperCase().replace(/=+$/, "")) {
    const value = alphabet.indexOf(character);
    if (value === -1) {
      continue;
    }
    bits += value.toString(2).padStart(5, "0");
  }
  const bytes: number[] = [];
  for (let i = 0; i + 8 <= bits.length; i += 8) {
    bytes.push(parseInt(bits.substring(i, i + 8), 2));
  }
  return Buffer.from(bytes);
}

export function generateTOTPCode(base32Secret: string): string {
  const key = base32Decode(base32Secret);
  const step = Math.floor(Date.now() / 1000 / 30);
  const buffer = Buffer.alloc(8);
  buffer.writeBigInt64BE(BigInt(step));
  const hmac = createHmac("sha1", key);
  hmac.update(buffer);
  const hash = hmac.digest();
  const offset = hash[hash.length - 1] & 0xf;
  const code = (hash.readUInt32BE(offset) & 0x7fffffff) % 1000000;
  return code.toString().padStart(6, "0");
}

/**
 * TODO: Document promoteSessionToAAL2WithTOTP.
 */
export async function promoteSessionToAAL2WithTOTP(
  request: APIRequestContext,
  page: Page,
  email: string,
  password: string,
  totpSecret: string,
): Promise<void> {
  const loginRes = await request.post("/api/auth/login", {
    data: { email, password },
  });
  await validateResponse(loginRes, "Login for AAL2 step-up");
  const loginBody = await loginRes.json();
  if (
    !loginBody?.mfa_pending ||
    typeof loginBody?.mfa_token !== "string" ||
    loginBody.mfa_token.length === 0
  ) {
    throw new Error("Expected MFA pending token during AAL2 step-up login");
  }

  const pendingToken = loginBody.mfa_token as string;
  const newChallenge = async (): Promise<string> => {
    const challengeRes = await request.post("/api/auth/mfa/totp/challenge", {
      headers: { Authorization: `Bearer ${pendingToken}` },
    });
    await validateResponse(challengeRes, "Create TOTP challenge for AAL2 step-up");
    const challengeBody = await challengeRes.json();
    if (
      typeof challengeBody?.challenge_id !== "string" ||
      challengeBody.challenge_id.length === 0
    ) {
      throw new Error("AAL2 step-up challenge succeeded but no challenge_id was returned");
    }
    return challengeBody.challenge_id;
  };

  const tryVerify = async (challengeID: string, code: string): Promise<string | null> => {
    const verifyRes = await request.post("/api/auth/mfa/totp/verify", {
      headers: { Authorization: `Bearer ${pendingToken}` },
      data: { challenge_id: challengeID, code },
    });
    if (!verifyRes.ok()) {
      if (verifyRes.status() === 401) {
        return null;
      }
      let detail = "";
      try {
        detail = (await verifyRes.text()).trim();
      } catch {
        // Ignore parse issues and report status-only error.
      }
      const suffix = detail.length > 0 ? `: ${detail}` : "";
      throw new Error(`AAL2 step-up verify failed with status ${verifyRes.status()}${suffix}`);
    }
    const verifyBody = await verifyRes.json();
    if (typeof verifyBody?.token !== "string" || verifyBody.token.length === 0) {
      throw new Error("AAL2 step-up verify succeeded but no access token was returned");
    }
    return verifyBody.token as string;
  };

  const firstChallengeID = await newChallenge();
  let upgradedToken = await tryVerify(firstChallengeID, generateTOTPCode(totpSecret));
  if (!upgradedToken) {
    const retryChallengeID = await newChallenge();
    upgradedToken = await tryVerify(retryChallengeID, generateTOTPCode(totpSecret));
  }
  if (!upgradedToken) {
    throw new Error("AAL2 step-up failed: TOTP code was rejected twice");
  }

  await page.evaluate((token: string) => {
    window.localStorage.setItem("ayb_auth_token", token);
  }, upgradedToken);
}

export async function createAnonymousAuthSessionToken(
  request: APIRequestContext,
): Promise<string> {
  const res = await request.post("/api/auth/anonymous");
  await validateResponse(res, "Create anonymous auth session");
  const body = await res.json();
  if (typeof body?.token !== "string" || body.token.length === 0) {
    throw new Error("Anonymous auth session created but no token was returned");
  }
  return body.token;
}

/**
 * TODO: Document createLinkedEmailAuthSessionToken.
 */
export async function createLinkedEmailAuthSessionToken(
  request: APIRequestContext,
  email: string,
  password: string,
): Promise<string> {
  const anonymousToken = await createAnonymousAuthSessionToken(request);
  const res = await request.post("/api/auth/link/email", {
    headers: {
      Authorization: `Bearer ${anonymousToken}`,
      "Content-Type": "application/json",
    },
    data: { email, password },
  });
  await validateResponse(res, `Link anonymous auth session for ${email}`);
  const body = await res.json();
  if (typeof body?.token !== "string" || body.token.length === 0) {
    throw new Error("Linked auth session created but no token was returned");
  }
  if (body?.user?.is_anonymous === true) {
    throw new Error("Linked auth session still returned an anonymous user");
  }
  return body.token;
}

export async function getAuthMeWithToken(
  request: APIRequestContext,
  token: string,
) {
  return request.get("/api/auth/me", {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function loginEmailAuthSessionToken(
  request: APIRequestContext,
  email: string,
  password: string,
): Promise<string> {
  const loginRes = await request.post("/api/auth/login", {
    data: { email, password },
  });
  await validateResponse(loginRes, `Login linked user ${email}`);
  const loginBody = (await loginRes.json()) as { token?: string };
  if (typeof loginBody.token !== "string" || loginBody.token.length === 0) {
    throw new Error(`Login linked user ${email} returned no auth token`);
  }
  return loginBody.token;
}

/**
 * Resolves an auth user's id by email from the internal users table.
 */
export async function resolveAuthUserIdByEmail(
  request: APIRequestContext,
  token: string,
  email: string,
): Promise<string> {
  const safeEmail = sqlLiteral(email);
  const result = await execSQL(
    request,
    token,
    `SELECT id::text FROM _ayb_users WHERE email = '${safeEmail}' LIMIT 1`,
  );
  if (result.rows.length !== 1) {
    throw new Error(`Expected exactly one auth user row for ${email}`);
  }
  const row = result.rows[0];
  if (!Array.isArray(row)) {
    throw new Error(`Expected SQL row array while resolving auth user id for ${email}`);
  }
  const userId = row[0];
  if (typeof userId !== "string" || userId.length === 0) {
    throw new Error(`Expected string auth user id while resolving ${email}`);
  }
  return userId;
}

/**
 * TODO: Document ensureAuthSettings.
 */
export async function ensureAuthSettings(
  request: APIRequestContext,
  token: string,
  overrides: Record<string, boolean>,
): Promise<void> {
  const getRes = await request.get("/api/admin/auth-settings", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(getRes, "Get auth settings");
  const current = await getRes.json();
  const updated = { ...current, ...overrides };
  const putRes = await request.put("/api/admin/auth-settings", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: updated,
  });
  await validateResponse(putRes, "Update auth settings");
}

/**
 * TODO: Document getAuthSettingsUnavailableSkipReason.
 */
export async function getAuthSettingsUnavailableSkipReason(
  request: APIRequestContext,
  token: string,
): Promise<string | null> {
  const authSettingsProbeStatus = await probeEndpoint(request, token, "/api/admin/auth-settings");
  if (authSettingsProbeStatus === 404 || authSettingsProbeStatus === 501 || authSettingsProbeStatus === 503) {
    return `Auth settings service unavailable (status ${authSettingsProbeStatus})`;
  }
  if (authSettingsProbeStatus >= 500) {
    throw new Error(
      `Auth settings probe returned status ${authSettingsProbeStatus}; expected available status before auth-dependent browser proof`,
    );
  }
  return null;
}

/**
 * TODO: Document overrideEmailMFACode.
 */
export async function overrideEmailMFACode(
  request: APIRequestContext,
  token: string,
  knownCode: string,
): Promise<void> {
  await execSQL(request, token, "CREATE EXTENSION IF NOT EXISTS pgcrypto");
  const safeCode = sqlLiteral(knownCode);
  await execSQL(
    request,
    token,
    `UPDATE _ayb_mfa_challenges
     SET otp_code_hash = crypt('${safeCode}', gen_salt('bf', 10))
     WHERE id = (
       SELECT c.id FROM _ayb_mfa_challenges c
       JOIN _ayb_user_mfa f ON c.factor_id = f.id
       WHERE f.method = 'email' AND c.verified_at IS NULL
       ORDER BY c.created_at DESC LIMIT 1
     )`,
  );
}

/**
 * TODO: Document cleanupAuthUser.
 */
export async function cleanupAuthUser(
  request: APIRequestContext,
  token: string,
  email: string,
): Promise<void> {
  const safeEmail = sqlLiteral(email);
  const uid = `(SELECT id FROM _ayb_users WHERE email = '${safeEmail}')`;
  const fid = `(SELECT id FROM _ayb_user_mfa WHERE user_id IN ${uid})`;
  await execSQL(request, token, `DELETE FROM _ayb_mfa_backup_codes WHERE user_id IN ${uid}`).catch(
    () => {},
  );
  await execSQL(request, token, `DELETE FROM _ayb_mfa_challenges WHERE factor_id IN ${fid}`).catch(
    () => {},
  );
  await execSQL(request, token, `DELETE FROM _ayb_user_mfa WHERE user_id IN ${uid}`).catch(
    () => {},
  );
  await execSQL(request, token, `DELETE FROM _ayb_users WHERE email = '${safeEmail}'`).catch(
    () => {},
  );
}

export async function fetchAuthHooksConfig(
  request: APIRequestContext,
  token: string,
): Promise<Record<string, string>> {
  const res = await request.get("/api/admin/auth/hooks", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch auth hooks config");
  return await res.json();
}

interface OAuthProviderInfo {
  name: string;
  type: "builtin" | "oidc";
  enabled: boolean;
  client_id_configured: boolean;
}

export async function listAuthProviders(
  request: APIRequestContext,
  token: string,
): Promise<OAuthProviderInfo[]> {
  const res = await request.get("/api/admin/auth/providers", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "List auth providers");
  const body = await res.json();
  return body.providers ?? [];
}

/**
 * TODO: Document updateAuthProvider.
 */
export async function updateAuthProvider(
  request: APIRequestContext,
  token: string,
  provider: string,
  payload: Record<string, unknown>,
): Promise<OAuthProviderInfo> {
  const res = await request.put(`/api/admin/auth/providers/${encodeURIComponent(provider)}`, {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: payload,
  });
  await validateResponse(res, `Update auth provider ${provider}`);
  return await res.json();
}

export async function deleteAuthProvider(
  request: APIRequestContext,
  token: string,
  provider: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/auth/providers/${encodeURIComponent(provider)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 204 && res.status() !== 404) {
    await validateResponse(res, `Delete auth provider ${provider}`);
  }
}
