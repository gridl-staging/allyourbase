import type { APIRequestContext, TestInfo } from "@playwright/test";
import {
  assertSafeSQLIdentifier,
  authorizeOAuthRequest,
  buildParallelSafeRunID,
  cleanupAdminAppByName,
  cleanupAuthUser,
  cleanupOAuthClientByName,
  createLinkedEmailAuthSessionToken,
  dropTableIfExists,
  exchangeOAuthAuthorizationCode,
  ensureAuthSettings,
  execSQL,
  expect,
  fetchAuthSettings,
  getAuthSettingsUnavailableSkipReason,
  generateOAuthPKCEPair,
  listRecords,
  parseOAuthRedirectURL,
  probeEndpoint,
  resolveAuthUserIdByEmail,
  seedAdminApp,
  seedOAuthClient,
  seedRecord,
  submitOAuthConsent,
  test,
} from "../fixtures";

interface CleanupState {
  anonymousAuthEnabled?: boolean;
  appName?: string;
  clientName?: string;
  emailA?: string;
  emailB?: string;
  readonlyTable?: string;
  scopedAllowedTable?: string;
  scopedDeniedTable?: string;
  ownerTable?: string;
}

interface OAuthAuthCodeTokenOptions {
  clientID: string;
  clientSecret: string;
  redirectURI: string;
  userToken: string;
  scope: string;
  state: string;
  codeVerifier: string;
  allowedTables?: string[];
}

interface OAuthRestrictionIdentifiers {
  runID: string;
  readonlyTable: string;
  scopedAllowedTable: string;
  scopedDeniedTable: string;
  ownerTable: string;
  ownerPolicy: string;
  emailA: string;
  emailB: string;
  passwordA: string;
  passwordB: string;
  appName: string;
  clientName: string;
  redirectURI: string;
}

interface OAuthRestrictionContext extends OAuthRestrictionIdentifiers {
  linkedUserTokenA: string;
  linkedUserTokenB: string;
  userAID: string;
  oauthClientID: string;
  oauthClientSecret: string;
}

function validateOAuthRestrictionIdentifiers(
  identifiers: OAuthRestrictionIdentifiers,
): OAuthRestrictionIdentifiers {
  return {
    ...identifiers,
    readonlyTable: assertSafeSQLIdentifier(identifiers.readonlyTable, "OAuth readonly table"),
    scopedAllowedTable: assertSafeSQLIdentifier(
      identifiers.scopedAllowedTable,
      "OAuth allowed table",
    ),
    scopedDeniedTable: assertSafeSQLIdentifier(
      identifiers.scopedDeniedTable,
      "OAuth denied table",
    ),
    ownerTable: assertSafeSQLIdentifier(identifiers.ownerTable, "OAuth owner table"),
    ownerPolicy: assertSafeSQLIdentifier(identifiers.ownerPolicy, "OAuth owner policy"),
  };
}

function buildOAuthRestrictionIdentifiers(testInfo: TestInfo): OAuthRestrictionIdentifiers {
  const runID = buildParallelSafeRunID(testInfo);
  return validateOAuthRestrictionIdentifiers({
    runID,
    readonlyTable: `oauth_readonly_${runID}`,
    scopedAllowedTable: `oauth_allowed_${runID}`,
    scopedDeniedTable: `oauth_denied_${runID}`,
    ownerTable: `oauth_owner_${runID}`,
    ownerPolicy: `owner_only_${runID}`,
    emailA: `oauth-auth-a-${runID}@example.com`,
    emailB: `oauth-auth-b-${runID}@example.com`,
    passwordA: `TestPassA!${runID}`,
    passwordB: `TestPassB!${runID}`,
    appName: `oauth-auth-app-${runID}`,
    clientName: `oauth-auth-client-${runID}`,
    redirectURI: `https://oauth.example.test/callback/${runID}`,
  });
}

// Always mints a real auth-code token through authorize + consent + token exchange.
async function mintOAuthAuthCodeToken(
  request: APIRequestContext,
  options: OAuthAuthCodeTokenOptions,
): Promise<string> {
  const pkce = generateOAuthPKCEPair(options.codeVerifier);

  const authorizeResult = await authorizeOAuthRequest(request, options.userToken, {
    responseType: "code",
    clientId: options.clientID,
    redirectURI: options.redirectURI,
    scope: options.scope,
    state: options.state,
    codeChallenge: pkce.codeChallenge,
    codeChallengeMethod: pkce.codeChallengeMethod,
    allowedTables: options.allowedTables,
  });

  const redirectTo =
    authorizeResult.kind === "requires_consent"
      ? (
          await submitOAuthConsent(request, options.userToken, {
            decision: "approve",
            responseType: "code",
            clientId: options.clientID,
            redirectURI: options.redirectURI,
            scope: options.scope,
            state: options.state,
            codeChallenge: pkce.codeChallenge,
            codeChallengeMethod: pkce.codeChallengeMethod,
            allowedTables: options.allowedTables,
          })
        ).redirectTo
      : authorizeResult.redirectTo;

  const redirect = parseOAuthRedirectURL(redirectTo);
  expect(redirect.error).toBeUndefined();
  expect(redirect.code).toBeTruthy();
  expect(redirect.state).toBe(options.state);

  const tokenResponse = await exchangeOAuthAuthorizationCode(request, {
    code: redirect.code as string,
    redirectURI: options.redirectURI,
    codeVerifier: pkce.codeVerifier,
    clientAuth: {
      method: "body",
      clientId: options.clientID,
      clientSecret: options.clientSecret,
    },
  });

  expect(tokenResponse.access_token.length).toBeGreaterThan(0);
  return tokenResponse.access_token;
}

async function setupOAuthRestrictionContext(
  request: APIRequestContext,
  adminToken: string,
  testInfo: TestInfo,
  cleanupByTestID: Map<string, CleanupState>,
): Promise<OAuthRestrictionContext> {
  const identifiers = buildOAuthRestrictionIdentifiers(testInfo);

  const originalAuthSettings = await fetchAuthSettings(request, adminToken);
  cleanupByTestID.set(testInfo.testId, {
    anonymousAuthEnabled: originalAuthSettings.anonymous_auth_enabled,
    appName: identifiers.appName,
    clientName: identifiers.clientName,
    emailA: identifiers.emailA,
    emailB: identifiers.emailB,
    readonlyTable: identifiers.readonlyTable,
    scopedAllowedTable: identifiers.scopedAllowedTable,
    scopedDeniedTable: identifiers.scopedDeniedTable,
    ownerTable: identifiers.ownerTable,
  });

  await ensureAuthSettings(request, adminToken, { anonymous_auth_enabled: true });
  const linkedUserTokenA = await createLinkedEmailAuthSessionToken(
    request,
    identifiers.emailA,
    identifiers.passwordA,
  );
  const linkedUserTokenB = await createLinkedEmailAuthSessionToken(
    request,
    identifiers.emailB,
    identifiers.passwordB,
  );
  const userAID = await resolveAuthUserIdByEmail(request, adminToken, identifiers.emailA);

  const app = await seedAdminApp(request, adminToken, {
    name: identifiers.appName,
    ownerUserId: userAID,
    description: `oauth auth restrictions ${identifiers.runID}`,
  });
  const oauthClient = await seedOAuthClient(request, adminToken, {
    appId: app.id,
    name: identifiers.clientName,
    clientType: "confidential",
    redirectUris: [identifiers.redirectURI],
    scopes: ["readonly", "readwrite"],
  });

  expect(typeof oauthClient.clientSecret).toBe("string");
  expect((oauthClient.clientSecret as string).length).toBeGreaterThan(0);

  return {
    ...identifiers,
    linkedUserTokenA,
    linkedUserTokenB,
    userAID,
    oauthClientID: oauthClient.clientId,
    oauthClientSecret: oauthClient.clientSecret as string,
  };
}

async function createRestrictionTables(
  request: APIRequestContext,
  adminToken: string,
  context: OAuthRestrictionContext,
): Promise<void> {
  await execSQL(
    request,
    adminToken,
    `CREATE TABLE ${context.readonlyTable} (id serial PRIMARY KEY, name text NOT NULL, user_id uuid);
     CREATE TABLE ${context.scopedAllowedTable} (id serial PRIMARY KEY, name text NOT NULL, user_id uuid);
     CREATE TABLE ${context.scopedDeniedTable} (id serial PRIMARY KEY, name text NOT NULL, user_id uuid);
     CREATE TABLE ${context.ownerTable} (id serial PRIMARY KEY, name text NOT NULL, user_id uuid NOT NULL);
     ALTER TABLE ${context.ownerTable} ENABLE ROW LEVEL SECURITY;
     DROP POLICY IF EXISTS ${context.ownerPolicy} ON ${context.ownerTable};
     CREATE POLICY ${context.ownerPolicy}
       ON ${context.ownerTable}
       FOR ALL
       USING (user_id = current_setting('ayb.user_id', true)::uuid)
       WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid)`,
  );
}

async function assertReadonlyLane(
  request: APIRequestContext,
  context: OAuthRestrictionContext,
): Promise<void> {
  const readonlyOAuthToken = await mintOAuthAuthCodeToken(request, {
    clientID: context.oauthClientID,
    clientSecret: context.oauthClientSecret,
    redirectURI: context.redirectURI,
    userToken: context.linkedUserTokenA,
    scope: "readonly",
    state: `readonly-${context.runID}`,
    codeVerifier: `readonly-verifier-${context.runID}`,
  });

  const readonlyItems = await listRecords(request, readonlyOAuthToken, context.readonlyTable);
  expect(readonlyItems).toHaveLength(0);
  await expect(
    seedRecord(request, readonlyOAuthToken, context.readonlyTable, {
      name: "readonly-should-fail",
    }),
  ).rejects.toThrow(/status 403|readonly|write operations/i);
}

async function assertAllowedTablesLane(
  request: APIRequestContext,
  context: OAuthRestrictionContext,
): Promise<void> {
  const allowedTablesOAuthToken = await mintOAuthAuthCodeToken(request, {
    clientID: context.oauthClientID,
    clientSecret: context.oauthClientSecret,
    redirectURI: context.redirectURI,
    userToken: context.linkedUserTokenA,
    scope: "readwrite",
    state: `allowed-${context.runID}`,
    codeVerifier: `allowed-verifier-${context.runID}`,
    allowedTables: [context.scopedAllowedTable],
  });

  await seedRecord(request, allowedTablesOAuthToken, context.scopedAllowedTable, {
    name: "allowed-table-row",
  });
  const allowedItems = await listRecords(
    request,
    allowedTablesOAuthToken,
    context.scopedAllowedTable,
  );
  expect(allowedItems).toHaveLength(1);
  expect(allowedItems[0]?.["name"]).toBe("allowed-table-row");

  await expect(
    listRecords(request, allowedTablesOAuthToken, context.scopedDeniedTable),
  ).rejects.toThrow(/status 403|insufficient permissions|allowed/i);
  await expect(
    seedRecord(request, allowedTablesOAuthToken, context.scopedDeniedTable, {
      name: "denied-table-row",
    }),
  ).rejects.toThrow(/status 403|insufficient permissions|allowed/i);
}

async function mintOwnerScopedOAuthToken(
  request: APIRequestContext,
  context: OAuthRestrictionContext,
  userToken: string,
  tokenLabel: string,
): Promise<string> {
  return mintOAuthAuthCodeToken(request, {
    clientID: context.oauthClientID,
    clientSecret: context.oauthClientSecret,
    redirectURI: context.redirectURI,
    userToken,
    scope: "readwrite",
    state: `${tokenLabel}-${context.runID}`,
    codeVerifier: `${tokenLabel}-verifier-${context.runID}`,
    allowedTables: [context.ownerTable],
  });
}

async function assertOwnerMatchLane(
  request: APIRequestContext,
  context: OAuthRestrictionContext,
): Promise<void> {
  const ownerMatchOAuthToken = await mintOwnerScopedOAuthToken(
    request,
    context,
    context.linkedUserTokenA,
    "owner",
  );
  const otherUserOwnerOAuthToken = await mintOwnerScopedOAuthToken(
    request,
    context,
    context.linkedUserTokenB,
    "owner-other",
  );

  await seedRecord(request, context.linkedUserTokenA, context.ownerTable, {
    name: "owned-by-a",
    user_id: context.userAID,
  });

  const ownerItemsFromEmailToken = await listRecords(
    request,
    context.linkedUserTokenA,
    context.ownerTable,
  );
  expect(ownerItemsFromEmailToken).toHaveLength(1);
  expect(ownerItemsFromEmailToken[0]?.["name"]).toBe("owned-by-a");

  const ownerItemsFromOAuthToken = await listRecords(
    request,
    ownerMatchOAuthToken,
    context.ownerTable,
  );
  expect(ownerItemsFromOAuthToken).toHaveLength(1);
  expect(ownerItemsFromOAuthToken[0]?.["name"]).toBe("owned-by-a");

  const userBVisibleItems = await listRecords(request, context.linkedUserTokenB, context.ownerTable);
  expect(userBVisibleItems).toHaveLength(0);
  const otherUserOAuthVisibleItems = await listRecords(
    request,
    otherUserOwnerOAuthToken,
    context.ownerTable,
  );
  expect(otherUserOAuthVisibleItems).toHaveLength(0);
  await expect(
    seedRecord(request, context.linkedUserTokenB, context.ownerTable, {
      name: "stolen",
      user_id: context.userAID,
    }),
  ).rejects.toThrow(/status 403|insufficient permissions/i);
  await expect(
    seedRecord(request, otherUserOwnerOAuthToken, context.ownerTable, {
      name: "oauth-stolen",
      user_id: context.userAID,
    }),
  ).rejects.toThrow(/status 403|insufficient permissions/i);
}

test.describe("OAuth Auth Restrictions (Full E2E)", () => {
  const cleanupByTestID = new Map<string, CleanupState>();

  test.afterEach(async ({ request, adminToken }, testInfo: TestInfo) => {
    const cleanup = cleanupByTestID.get(testInfo.testId);
    if (!cleanup) {
      return;
    }

    if (cleanup.clientName) {
      await cleanupOAuthClientByName(request, adminToken, cleanup.clientName).catch(() => {});
    }
    if (cleanup.appName) {
      await cleanupAdminAppByName(request, adminToken, cleanup.appName).catch(() => {});
    }
    if (cleanup.emailA) {
      await cleanupAuthUser(request, adminToken, cleanup.emailA).catch(() => {});
    }
    if (cleanup.emailB) {
      await cleanupAuthUser(request, adminToken, cleanup.emailB).catch(() => {});
    }

    for (const [tableName, label] of [
      [cleanup.ownerTable, "OAuth cleanup owner table"],
      [cleanup.scopedDeniedTable, "OAuth cleanup denied table"],
      [cleanup.scopedAllowedTable, "OAuth cleanup allowed table"],
      [cleanup.readonlyTable, "OAuth cleanup readonly table"],
    ] as const) {
      if (!tableName) {
        continue;
      }
      await dropTableIfExists(request, adminToken, tableName, label).catch(() => {});
    }
    if (typeof cleanup.anonymousAuthEnabled === "boolean") {
      await ensureAuthSettings(request, adminToken, {
        anonymous_auth_enabled: cleanup.anonymousAuthEnabled,
      }).catch(() => {});
    }
    cleanupByTestID.delete(testInfo.testId);
  });

  test("enforces readonly, allowed_tables, and owner-match on OAuth auth-code tokens", async (
    { request, adminToken },
    testInfo: TestInfo,
  ) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/oauth/clients");
    test.skip(
      probeStatus === 404 || probeStatus === 501,
      `OAuth clients service not configured (status ${probeStatus})`,
    );

    const authSettingsSkipReason = await getAuthSettingsUnavailableSkipReason(request, adminToken);
    test.skip(Boolean(authSettingsSkipReason), authSettingsSkipReason ?? "");

    const context = await setupOAuthRestrictionContext(request, adminToken, testInfo, cleanupByTestID);
    await createRestrictionTables(request, adminToken, context);

    await assertReadonlyLane(request, context);
    await assertAllowedTablesLane(request, context);
    await assertOwnerMatchLane(request, context);

    await execSQL(
      request,
      adminToken,
      `DROP POLICY IF EXISTS ${context.ownerPolicy} ON ${context.ownerTable}`,
    );
  });
});
