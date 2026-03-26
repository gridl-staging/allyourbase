/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/auth.ts.
 */
import type { Page, Route } from "@playwright/test";
import { json, type MockApiResponse } from "./core";

/**
 * Optional response overrides for MFA mock endpoints. Customize API behavior for factors listing, TOTP/email/backup enrollment and verification, backup code generation and regeneration, anonymous sign-up, and email linking. Each property accepts {status, body}.
 */
export interface MFAMockOptions {
  factorsResponse?: { status: number; body: unknown };
  totpChallengeResponse?: { status: number; body: unknown };
  totpVerifyResponse?: { status: number; body: unknown };
  totpEnrollConfirmResponse?: { status: number; body: unknown };
  emailChallengeResponse?: { status: number; body: unknown };
  emailVerifyResponse?: { status: number; body: unknown };
  emailEnrollConfirmResponse?: { status: number; body: unknown };
  backupVerifyResponse?: { status: number; body: unknown };
  backupGenerateResponse?: { status: number; body: unknown };
  backupRegenerateResponse?: { status: number; body: unknown };
  backupCountResponse?: { status: number; body: unknown };
  enrollTOTPResponse?: { status: number; body: unknown };
  enrollEmailResponse?: { status: number; body: unknown };
  anonymousResponse?: { status: number; body: unknown };
  linkEmailResponse?: { status: number; body: unknown };
}

export interface MFAMockState {
  totpEnrollConfirmCalls: number;
  totpVerifyCalls: number;
  emailEnrollConfirmCalls: number;
  emailVerifyCalls: number;
  backupGenerateCalls: number;
  backupRegenerateCalls: number;
  backupVerifyCalls: number;
  anonymousCalls: number;
  linkEmailCalls: number;
}

const defaultTokens = {
  token: "mock-access-token",
  refreshToken: "mock-refresh-token",
  user: { id: "user-1", email: "test@example.com", is_anonymous: false },
};

const defaultAnonymousTokens = {
  token: "mock-anon-token",
  refreshToken: "mock-anon-refresh",
  user: { id: "anon-1", email: "", is_anonymous: true },
};

const defaultBackupCodes = [
  "abc12-def34",
  "ghi56-jkl78",
  "mno90-pqr12",
  "stu34-vwx56",
  "yza78-bcd90",
  "efg12-hij34",
  "klm56-nop78",
  "qrs90-tuv12",
  "wxy34-zab56",
  "cde78-fgh90",
];

interface AuthRouteContext {
  route: Route;
  request: ReturnType<Route["request"]>;
  method: string;
  path: string;
}

/**
 * Intercepts MFA endpoints for TOTP enrollment/verify, email enrollment/verify, backup code operations, and anonymous authentication. Supports link-email flow. All responses are customizable via options. @param page - Playwright page @param options - optional custom response bodies for any MFA endpoint @returns promise resolving to MFAMockState tracking enrollment confirmations and verification calls
 */
export async function mockMFAApis(
  page: Page,
  options: MFAMockOptions = {},
): Promise<MFAMockState> {
  const state = createMFAMockState();

  await registerMockedAuthShellRoutes(page);
  await page.route("**/api/auth/mfa/**", async (route) => handleMFARoute(route, options, state));
  await registerAnonymousAuthRoute(page, options, state);
  await registerLinkEmailRoute(page, options, state);

  return state;
}

function createMFAMockState(): MFAMockState {
  return {
    totpEnrollConfirmCalls: 0,
    totpVerifyCalls: 0,
    emailEnrollConfirmCalls: 0,
    emailVerifyCalls: 0,
    backupGenerateCalls: 0,
    backupRegenerateCalls: 0,
    backupVerifyCalls: 0,
    anonymousCalls: 0,
    linkEmailCalls: 0,
  };
}

async function registerMockedAuthShellRoutes(page: Page): Promise<void> {
  await page.route("**/api/admin/status", async (route) => json(route, 200, { auth: true }));
  await page.route("**/api/schema", async (route) =>
    json(route, 200, {
      tables: {},
      schemas: ["public"],
      builtAt: "2026-02-24T00:00:00Z",
    }),
  );
}

async function handleMFARoute(
  route: Route,
  options: MFAMockOptions,
  state: MFAMockState,
): Promise<void> {
  const context = createAuthRouteContext(route);

  if (await handleMFAFactorRoute(context, options)) return;
  if (await handleMFATOTPRoute(context, options, state)) return;
  if (await handleMFAEmailRoute(context, options, state)) return;
  if (await handleMFABackupRoute(context, options, state)) return;

  await json(context.route, 404, { message: "not mocked" });
}

function createAuthRouteContext(route: Route): AuthRouteContext {
  const request = route.request();
  return {
    route,
    request,
    method: request.method(),
    path: new URL(request.url()).pathname,
  };
}

async function handleMFAFactorRoute(context: AuthRouteContext, options: MFAMockOptions): Promise<boolean> {
  if (context.path.endsWith("/mfa/factors") && context.method === "GET") {
    const response = options.factorsResponse ?? {
      status: 200,
      body: { factors: [{ id: "f-1", method: "totp", enabled: true }] },
    };
    await json(context.route, response.status, response.body);
    return true;
  }

  return false;
}

/**
 * TODO: Document handleMFATOTPRoute.
 */
async function handleMFATOTPRoute(
  context: AuthRouteContext,
  options: MFAMockOptions,
  state: MFAMockState,
): Promise<boolean> {
  if (context.path.endsWith("/mfa/totp/challenge") && context.method === "POST") {
    const response = options.totpChallengeResponse ?? {
      status: 200,
      body: { challenge_id: "ch-totp-1" },
    };
    await json(context.route, response.status, response.body);
    return true;
  }

  if (context.path.endsWith("/mfa/totp/verify") && context.method === "POST") {
    state.totpVerifyCalls += 1;
    const response = options.totpVerifyResponse ?? { status: 200, body: defaultTokens };
    await json(context.route, response.status, response.body);
    return true;
  }

  if (context.path.endsWith("/mfa/totp/enroll") && context.method === "POST") {
    const response = options.enrollTOTPResponse ?? {
      status: 200,
      body: {
        factor_id: "f-totp-1",
        uri: "otpauth://totp/TestApp:test@example.com?secret=JBSWY3DPEHPK3PXP&issuer=TestApp",
        secret: "JBSWY3DPEHPK3PXP",
      },
    };
    await json(context.route, response.status, response.body);
    return true;
  }

  if (context.path.endsWith("/mfa/totp/enroll/confirm") && context.method === "POST") {
    state.totpEnrollConfirmCalls += 1;
    const response = options.totpEnrollConfirmResponse ?? {
      status: 200,
      body: { message: "TOTP enrollment confirmed" },
    };
    await json(context.route, response.status, response.body);
    return true;
  }

  return false;
}

/**
 * TODO: Document handleMFAEmailRoute.
 */
async function handleMFAEmailRoute(
  context: AuthRouteContext,
  options: MFAMockOptions,
  state: MFAMockState,
): Promise<boolean> {
  if (context.path.endsWith("/mfa/email/challenge") && context.method === "POST") {
    const response = options.emailChallengeResponse ?? {
      status: 200,
      body: { challenge_id: "ch-email-1" },
    };
    await json(context.route, response.status, response.body);
    return true;
  }

  if (context.path.endsWith("/mfa/email/verify") && context.method === "POST") {
    state.emailVerifyCalls += 1;
    const response = options.emailVerifyResponse ?? { status: 200, body: defaultTokens };
    await json(context.route, response.status, response.body);
    return true;
  }

  if (context.path.endsWith("/mfa/email/enroll") && context.method === "POST") {
    const response = options.enrollEmailResponse ?? {
      status: 200,
      body: { message: "verification code sent to your email" },
    };
    await json(context.route, response.status, response.body);
    return true;
  }

  if (context.path.endsWith("/mfa/email/enroll/confirm") && context.method === "POST") {
    state.emailEnrollConfirmCalls += 1;
    const response = options.emailEnrollConfirmResponse ?? {
      status: 200,
      body: { message: "Email MFA enrollment confirmed" },
    };
    await json(context.route, response.status, response.body);
    return true;
  }

  return false;
}

/**
 * TODO: Document handleMFABackupRoute.
 */
async function handleMFABackupRoute(
  context: AuthRouteContext,
  options: MFAMockOptions,
  state: MFAMockState,
): Promise<boolean> {
  if (context.path.endsWith("/mfa/backup/generate") && context.method === "POST") {
    state.backupGenerateCalls += 1;
    const response = options.backupGenerateResponse ?? {
      status: 200,
      body: { codes: defaultBackupCodes },
    };
    await json(context.route, response.status, response.body);
    return true;
  }

  if (context.path.endsWith("/mfa/backup/regenerate") && context.method === "POST") {
    state.backupRegenerateCalls += 1;
    const response = options.backupRegenerateResponse ?? {
      status: 200,
      body: { codes: defaultBackupCodes },
    };
    await json(context.route, response.status, response.body);
    return true;
  }

  if (context.path.endsWith("/mfa/backup/verify") && context.method === "POST") {
    state.backupVerifyCalls += 1;
    const response = options.backupVerifyResponse ?? { status: 200, body: defaultTokens };
    await json(context.route, response.status, response.body);
    return true;
  }

  if (context.path.endsWith("/mfa/backup/count") && context.method === "GET") {
    const response = options.backupCountResponse ?? {
      status: 200,
      body: { remaining: 5 },
    };
    await json(context.route, response.status, response.body);
    return true;
  }

  return false;
}

async function registerAnonymousAuthRoute(
  page: Page,
  options: MFAMockOptions,
  state: MFAMockState,
): Promise<void> {
  await page.route("**/api/auth/anonymous", async (route) => {
    state.anonymousCalls += 1;
    const response = options.anonymousResponse ?? { status: 201, body: defaultAnonymousTokens };
    return json(route, response.status, response.body);
  });
}

async function registerLinkEmailRoute(
  page: Page,
  options: MFAMockOptions,
  state: MFAMockState,
): Promise<void> {
  await page.route("**/api/auth/link/email", async (route) => {
    state.linkEmailCalls += 1;
    const response = options.linkEmailResponse ?? { status: 200, body: defaultTokens };
    return json(route, response.status, response.body);
  });
}

// ---------------------------------------------------------------------------
// Auth Provider Management mock
// ---------------------------------------------------------------------------

export interface AuthProviderMockState {
  listCalls: number;
  updateCalls: number;
  deleteCalls: number;
  testCalls: number;
  lastUpdateBody: Record<string, unknown> | null;
  lastUpdateProvider: string | null;
  lastDeleteProvider: string | null;
  lastTestProvider: string | null;
}

export interface AuthProviderMockOptions {
  authSettingsResponse?: MockApiResponse;
  providersListResponse?: MockApiResponse;
  updateProviderResponse?: MockApiResponse | ((provider: string, body: Record<string, unknown>) => MockApiResponse);
  deleteProviderResponse?: MockApiResponse;
  testProviderResponse?: MockApiResponse | ((provider: string) => MockApiResponse);
}

const defaultAuthSettings = {
  magic_link_enabled: false,
  sms_enabled: false,
  email_mfa_enabled: false,
  anonymous_auth_enabled: false,
  totp_enabled: false,
};

const defaultProvidersList = {
  providers: [
    { name: "google", type: "builtin", enabled: true, client_id_configured: true },
    { name: "github", type: "builtin", enabled: true, client_id_configured: true },
    { name: "discord", type: "builtin", enabled: false, client_id_configured: false },
    { name: "microsoft", type: "builtin", enabled: false, client_id_configured: false },
    { name: "apple", type: "builtin", enabled: false, client_id_configured: false },
  ],
};

type ProviderRecord = Record<string, unknown>;

/**
 * Mocks auth provider configuration endpoints. Maintains provider list state that persists across update and delete operations. Supports auth settings endpoints, provider listing, update, test connection, and delete. @param page - Playwright page @param options - optional response overrides for auth settings, provider list, update, delete, and test @returns promise resolving to AuthProviderMockState tracking operations and last modified provider
 */
export async function mockAuthProviderApis(
  page: Page,
  options: AuthProviderMockOptions = {},
): Promise<AuthProviderMockState> {
  const state = createAuthProviderMockState();
  const currentProviders = cloneProvidersList(options.providersListResponse?.body);

  await page.route("**/api/**", async (route) =>
    handleAuthProviderApiRoute(route, options, state, currentProviders),
  );

  return state;
}

function createAuthProviderMockState(): AuthProviderMockState {
  return {
    listCalls: 0,
    updateCalls: 0,
    deleteCalls: 0,
    testCalls: 0,
    lastUpdateBody: null,
    lastUpdateProvider: null,
    lastDeleteProvider: null,
    lastTestProvider: null,
  };
}

function cloneProvidersList(body: unknown): ProviderRecord[] {
  const providers = (body as { providers?: unknown[] } | undefined)?.providers ?? defaultProvidersList.providers;
  return JSON.parse(JSON.stringify(providers)) as ProviderRecord[];
}

/**
 * TODO: Document handleAuthProviderApiRoute.
 */
async function handleAuthProviderApiRoute(
  route: Route,
  options: AuthProviderMockOptions,
  state: AuthProviderMockState,
  currentProviders: ProviderRecord[],
): Promise<void> {
  const context = createAuthRouteContext(route);

  if (await handleAuthProviderShellRoute(context, options)) return;
  if (await handleProviderListRoute(context, state, options, currentProviders)) return;
  if (await handleUpdateProviderRoute(context, state, options, currentProviders)) return;
  if (await handleTestProviderRoute(context, state, options)) return;
  if (await handleDeleteProviderRoute(context, state, options, currentProviders)) return;

  await json(context.route, 500, {
    message: `Unhandled mocked API route: ${context.method} ${context.path}`,
  });
}

/**
 * TODO: Document handleAuthProviderShellRoute.
 */
async function handleAuthProviderShellRoute(
  context: AuthRouteContext,
  options: AuthProviderMockOptions,
): Promise<boolean> {
  if (context.method === "GET" && context.path === "/api/admin/status") {
    await json(context.route, 200, { auth: true });
    return true;
  }

  if (context.method === "GET" && context.path === "/api/schema") {
    await json(context.route, 200, {
      tables: {},
      schemas: ["public"],
      builtAt: "2026-02-24T00:00:00Z",
    });
    return true;
  }

  if (context.method === "GET" && context.path === "/api/admin/auth-settings") {
    const response = options.authSettingsResponse ?? { status: 200, body: defaultAuthSettings };
    await json(context.route, response.status, response.body);
    return true;
  }

  if (context.method === "PUT" && context.path === "/api/admin/auth-settings") {
    await json(context.route, 200, context.request.postDataJSON());
    return true;
  }

  return false;
}

async function handleProviderListRoute(
  context: AuthRouteContext,
  state: AuthProviderMockState,
  options: AuthProviderMockOptions,
  currentProviders: ProviderRecord[],
): Promise<boolean> {
  if (context.method === "GET" && context.path === "/api/admin/auth/providers") {
    state.listCalls += 1;
    const status = options.providersListResponse?.status ?? 200;
    await json(context.route, status, { providers: currentProviders });
    return true;
  }

  return false;
}

/**
 * TODO: Document handleUpdateProviderRoute.
 */
async function handleUpdateProviderRoute(
  context: AuthRouteContext,
  state: AuthProviderMockState,
  options: AuthProviderMockOptions,
  currentProviders: ProviderRecord[],
): Promise<boolean> {
  const updateMatch = context.path.match(/^\/api\/admin\/auth\/providers\/([^/]+)$/);
  if (context.method !== "PUT" || !updateMatch) {
    return false;
  }

  state.updateCalls += 1;
  const providerName = decodeURIComponent(updateMatch[1]);
  const body = context.request.postDataJSON() as Record<string, unknown>;
  state.lastUpdateProvider = providerName;
  state.lastUpdateBody = body;

  const response =
    typeof options.updateProviderResponse === "function"
      ? options.updateProviderResponse(providerName, body)
      : options.updateProviderResponse;

  const status = response?.status ?? 200;
  const responseBody = response?.body ?? buildUpdatedProvider(providerName, body, currentProviders);

  if (status === 200) {
    applyProviderUpdate(currentProviders, buildUpdatedProvider(providerName, body, currentProviders));
  }

  await json(context.route, status, responseBody);
  return true;
}

function buildUpdatedProvider(
  providerName: string,
  body: Record<string, unknown>,
  currentProviders: ProviderRecord[],
): ProviderRecord {
  const existing = currentProviders.find((provider) => provider.name === providerName);
  return {
    name: providerName,
    type: body.issuer_url ? "oidc" : "builtin",
    enabled: body.enabled ?? false,
    client_id_configured: !!(body.client_id || existing?.client_id_configured),
  };
}

function applyProviderUpdate(currentProviders: ProviderRecord[], updatedProvider: ProviderRecord): void {
  const existingIndex = currentProviders.findIndex(
    (provider) => provider.name === updatedProvider.name,
  );
  if (existingIndex >= 0) {
    currentProviders[existingIndex] = updatedProvider;
    return;
  }

  currentProviders.push(updatedProvider);
}

/**
 * TODO: Document handleTestProviderRoute.
 */
async function handleTestProviderRoute(
  context: AuthRouteContext,
  state: AuthProviderMockState,
  options: AuthProviderMockOptions,
): Promise<boolean> {
  const testMatch = context.path.match(/^\/api\/admin\/auth\/providers\/([^/]+)\/test$/);
  if (context.method !== "POST" || !testMatch) {
    return false;
  }

  state.testCalls += 1;
  const providerName = decodeURIComponent(testMatch[1]);
  state.lastTestProvider = providerName;

  const response =
    typeof options.testProviderResponse === "function"
      ? options.testProviderResponse(providerName)
      : options.testProviderResponse ?? {
          status: 200,
          body: {
            success: true,
            provider: providerName,
            message: "authorization endpoint is reachable",
          },
        };

  await json(context.route, response.status, response.body);
  return true;
}

/**
 * TODO: Document handleDeleteProviderRoute.
 */
async function handleDeleteProviderRoute(
  context: AuthRouteContext,
  state: AuthProviderMockState,
  options: AuthProviderMockOptions,
  currentProviders: ProviderRecord[],
): Promise<boolean> {
  const deleteMatch = context.path.match(/^\/api\/admin\/auth\/providers\/([^/]+)$/);
  if (context.method !== "DELETE" || !deleteMatch) {
    return false;
  }

  state.deleteCalls += 1;
  const providerName = decodeURIComponent(deleteMatch[1]);
  state.lastDeleteProvider = providerName;

  if (options.deleteProviderResponse) {
    await json(
      context.route,
      options.deleteProviderResponse.status,
      options.deleteProviderResponse.body,
    );
    return true;
  }

  const providerIndex = currentProviders.findIndex((provider) => provider.name === providerName);
  if (providerIndex >= 0) {
    currentProviders.splice(providerIndex, 1);
  }

  await context.route.fulfill({ status: 204 });
  return true;
}
