/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_2_auth_jwt_and_private_function_proof/allyourbase_dev/ui/browser-tests-unmocked/fixtures/oauth.ts.
 */
import type { APIRequestContext } from "@playwright/test";
import { createHash, randomBytes } from "crypto";
import { validateResponse } from "./core";

export interface OAuthPKCEPair {
  codeVerifier: string;
  codeChallenge: string;
  codeChallengeMethod: "S256";
}

export function generateOAuthPKCEPair(codeVerifier?: string): OAuthPKCEPair {
  const verifier = codeVerifier ?? randomBytes(32).toString("base64url");
  const codeChallenge = createHash("sha256").update(verifier).digest("base64url");
  return {
    codeVerifier: verifier,
    codeChallenge,
    codeChallengeMethod: "S256",
  };
}

export interface ParsedOAuthRedirectURL {
  code?: string;
  state?: string;
  error?: string;
  errorDescription?: string;
}

export function parseOAuthRedirectURL(redirectTo: string): ParsedOAuthRedirectURL {
  const redirectURL = new URL(redirectTo);
  const searchParams = redirectURL.searchParams;
  return {
    code: searchParams.get("code") || undefined,
    state: searchParams.get("state") || undefined,
    error: searchParams.get("error") || undefined,
    errorDescription: searchParams.get("error_description") || undefined,
  };
}

export interface OAuthAuthorizeRequestOptions {
  responseType: "code";
  clientId: string;
  redirectURI: string;
  scope: string;
  state: string;
  codeChallenge: string;
  codeChallengeMethod: "S256";
  allowedTables?: string[];
}

export interface OAuthConsentPromptResult {
  kind: "requires_consent";
  requiresConsent: true;
  clientID: string;
  clientName: string;
  redirectURI: string;
  scope: string;
  state: string;
  codeChallenge: string;
  codeChallengeMethod: string;
  allowedTables: string[];
}

export interface OAuthRedirectReadyResult {
  kind: "redirect_ready";
  requiresConsent: false;
  redirectTo: string;
}

export type OAuthAuthorizeResult = OAuthConsentPromptResult | OAuthRedirectReadyResult;

function toStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string");
}

/**
 * TODO: Document decodeAuthorizeResult.
 */
function decodeAuthorizeResult(body: unknown): OAuthAuthorizeResult {
  const responseBody = body as Record<string, unknown>;
  if (responseBody.requires_consent === true) {
    if (
      typeof responseBody.client_id !== "string" ||
      typeof responseBody.client_name !== "string" ||
      typeof responseBody.redirect_uri !== "string" ||
      typeof responseBody.scope !== "string" ||
      typeof responseBody.state !== "string" ||
      typeof responseBody.code_challenge !== "string" ||
      typeof responseBody.code_challenge_method !== "string"
    ) {
      throw new Error("Invalid OAuth authorize consent response shape");
    }
    return {
      kind: "requires_consent",
      requiresConsent: true,
      clientID: responseBody.client_id,
      clientName: responseBody.client_name,
      redirectURI: responseBody.redirect_uri,
      scope: responseBody.scope,
      state: responseBody.state,
      codeChallenge: responseBody.code_challenge,
      codeChallengeMethod: responseBody.code_challenge_method,
      allowedTables: toStringArray(responseBody.allowed_tables),
    };
  }

  if (typeof responseBody.redirect_to !== "string") {
    throw new Error("Invalid OAuth authorize redirect response shape");
  }

  return {
    kind: "redirect_ready",
    requiresConsent: false,
    redirectTo: responseBody.redirect_to,
  };
}

/**
 * TODO: Document authorizeOAuthRequest.
 */
export async function authorizeOAuthRequest(
  request: APIRequestContext,
  token: string,
  options: OAuthAuthorizeRequestOptions,
): Promise<OAuthAuthorizeResult> {
  const query = new URLSearchParams({
    response_type: options.responseType,
    client_id: options.clientId,
    redirect_uri: options.redirectURI,
    scope: options.scope,
    state: options.state,
    code_challenge: options.codeChallenge,
    code_challenge_method: options.codeChallengeMethod,
  });
  for (const table of options.allowedTables || []) {
    query.append("allowed_tables", table);
  }

  const response = await request.get(`/api/auth/authorize?${query.toString()}`, {
    headers: {
      Authorization: `Bearer ${token}`,
      Accept: "application/json",
    },
  });
  await validateResponse(response, "OAuth authorize request");
  return decodeAuthorizeResult(await response.json());
}

export interface OAuthConsentRequestOptions extends OAuthAuthorizeRequestOptions {
  decision: "approve" | "deny";
}

/**
 * TODO: Document submitOAuthConsent.
 */
export async function submitOAuthConsent(
  request: APIRequestContext,
  token: string,
  options: OAuthConsentRequestOptions,
): Promise<OAuthRedirectReadyResult> {
  const data: Record<string, unknown> = {
    decision: options.decision,
    response_type: options.responseType,
    client_id: options.clientId,
    redirect_uri: options.redirectURI,
    scope: options.scope,
    state: options.state,
    code_challenge: options.codeChallenge,
    code_challenge_method: options.codeChallengeMethod,
  };
  if (options.allowedTables !== undefined) {
    data.allowed_tables = options.allowedTables;
  }

  const response = await request.post("/api/auth/authorize/consent", {
    headers: {
      Authorization: `Bearer ${token}`,
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    data,
  });
  await validateResponse(response, "OAuth consent request");
  const body = await response.json();
  if (typeof body?.redirect_to !== "string") {
    throw new Error("Invalid OAuth consent redirect response shape");
  }
  return {
    kind: "redirect_ready",
    requiresConsent: false,
    redirectTo: body.redirect_to,
  };
}

export interface OAuthTokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token?: string;
  scope: string;
}

interface OAuthClientAuthBasic {
  method: "basic";
  clientId: string;
  clientSecret: string;
}

interface OAuthClientAuthBody {
  method: "body";
  clientId: string;
  clientSecret?: string;
}

export type OAuthClientAuth = OAuthClientAuthBasic | OAuthClientAuthBody;

interface TokenRequestCommonOptions {
  clientAuth: OAuthClientAuth;
}

/**
 * TODO: Document applyOAuthClientAuth.
 */
function applyOAuthClientAuth(
  form: URLSearchParams,
  headers: Record<string, string>,
  clientAuth: OAuthClientAuth,
): void {
  if (clientAuth.method === "basic") {
    const encodedCredentials = Buffer.from(
      `${clientAuth.clientId}:${clientAuth.clientSecret}`,
      "utf-8",
    ).toString("base64");
    headers.Authorization = `Basic ${encodedCredentials}`;
    return;
  }

  form.set("client_id", clientAuth.clientId);
  if (clientAuth.clientSecret !== undefined) {
    form.set("client_secret", clientAuth.clientSecret);
  }
}

/**
 * TODO: Document decodeOAuthTokenResponse.
 */
function decodeOAuthTokenResponse(body: unknown): OAuthTokenResponse {
  const responseBody = body as Record<string, unknown>;
  if (
    typeof responseBody.access_token !== "string" ||
    typeof responseBody.token_type !== "string" ||
    typeof responseBody.expires_in !== "number" ||
    typeof responseBody.scope !== "string"
  ) {
    throw new Error("Invalid OAuth token response shape");
  }
  if (
    responseBody.refresh_token !== undefined &&
    typeof responseBody.refresh_token !== "string"
  ) {
    throw new Error("Invalid OAuth token response shape");
  }
  return responseBody as OAuthTokenResponse;
}

/**
 * TODO: Document submitOAuthTokenForm.
 */
async function submitOAuthTokenForm(
  request: APIRequestContext,
  form: URLSearchParams,
  clientAuth: OAuthClientAuth,
): Promise<OAuthTokenResponse> {
  const headers: Record<string, string> = {
    "Content-Type": "application/x-www-form-urlencoded",
  };
  applyOAuthClientAuth(form, headers, clientAuth);
  const response = await request.post("/api/auth/token", {
    headers,
    data: form.toString(),
  });
  await validateResponse(response, "OAuth token exchange");
  return decodeOAuthTokenResponse(await response.json());
}

export interface OAuthAuthorizationCodeTokenOptions extends TokenRequestCommonOptions {
  code: string;
  redirectURI: string;
  codeVerifier: string;
}

export async function exchangeOAuthAuthorizationCode(
  request: APIRequestContext,
  options: OAuthAuthorizationCodeTokenOptions,
): Promise<OAuthTokenResponse> {
  if (options.codeVerifier === "") {
    throw new Error("codeVerifier is required");
  }
  const form = new URLSearchParams({
    grant_type: "authorization_code",
    code: options.code,
    redirect_uri: options.redirectURI,
    code_verifier: options.codeVerifier,
  });
  return submitOAuthTokenForm(request, form, options.clientAuth);
}

export interface OAuthClientCredentialsTokenOptions extends TokenRequestCommonOptions {
  scope: string;
  allowedTables?: string[];
}

export async function exchangeOAuthClientCredentials(
  request: APIRequestContext,
  options: OAuthClientCredentialsTokenOptions,
): Promise<OAuthTokenResponse> {
  const form = new URLSearchParams({
    grant_type: "client_credentials",
    scope: options.scope,
  });
  for (const table of options.allowedTables || []) {
    form.append("allowed_tables", table);
  }
  return submitOAuthTokenForm(request, form, options.clientAuth);
}

export interface OAuthRefreshTokenOptions extends TokenRequestCommonOptions {
  refreshToken: string;
}

export async function exchangeOAuthRefreshToken(
  request: APIRequestContext,
  options: OAuthRefreshTokenOptions,
): Promise<OAuthTokenResponse> {
  const form = new URLSearchParams({
    grant_type: "refresh_token",
    refresh_token: options.refreshToken,
  });
  return submitOAuthTokenForm(request, form, options.clientAuth);
}
