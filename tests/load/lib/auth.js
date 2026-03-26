import http from 'k6/http';

import { assertResponseChecks, parseJSONResponse } from './checks.js';
import { loadScenarioOptions, parsePositiveInt, readEnv, trimTrailingSlashes } from './env.js';

const AUTH_REGISTER_PATH = '/api/auth/register';
const AUTH_LOGIN_PATH = '/api/auth/login';
const AUTH_REFRESH_PATH = '/api/auth/refresh';
const TENANT_ADMIN_PATH = '/api/admin/tenants';
const TENANT_LIST_QUERY = 'page=1&perPage=100';
const RUN_NONCE = Date.now().toString(36);
const DEFAULT_AUTH_PASSWORD = `load-auth-password-${RUN_NONCE}-${Math.random().toString(36).slice(2, 12)}`;
const DEFAULT_WS_USER_POOL_SIZE = 0;
const DEFAULT_TENANT_PLAN_TIER = 'free';
const DEFAULT_TENANT_ISOLATION_MODE = 'shared';
const tenantByIdentitySlot = new Map();

function authJSONRequestOptions(endpointTag, headers, expectedStatuses, method = 'POST') {
  return {
    headers: {
      'Content-Type': 'application/json',
      ...headers,
    },
    responseCallback: http.expectedStatuses(...expectedStatuses),
    tags: {
      endpoint: endpointTag,
      method,
    },
  };
}

function normalizeVU(vu) {
  const parsedVU = Number.parseInt(String(vu), 10);
  return Number.isFinite(parsedVU) && parsedVU > 0 ? parsedVU : 1;
}

function tenantAdminURL(baseURL) {
  return `${trimTrailingSlashes(baseURL)}${TENANT_ADMIN_PATH}`;
}

function tenantListURL(baseURL) {
  return `${tenantAdminURL(baseURL)}?${TENANT_LIST_QUERY}`;
}

function resolveIdentitySlot(identity) {
  if (identity === null || typeof identity !== 'object') {
    return 1;
  }
  return normalizeVU(identity.identitySlot);
}

function tenantSlug(identitySlot) {
  return `load-tenant-${RUN_NONCE}-slot${identitySlot}`;
}

function buildCredentialBody(email, password) {
  return {
    email,
    password,
  };
}

export function buildAuthHeaders(token, tenantID = '') {
  const tenantHeaders = tenantID === '' ? {} : { 'X-Tenant-ID': tenantID };
  return {
    Authorization: `Bearer ${token}`,
    ...tenantHeaders,
  };
}

export function authSessionHeaders(authSession) {
  const tenantID = authSession.tenantID || '';
  return buildAuthHeaders(authSession.token, tenantID);
}

function parseTenantBootstrapResponse(response, endpointTag) {
  const payload = parseJSONResponse(response);
  if (payload === null || typeof payload !== 'object') {
    throw new Error(`${endpointTag} returned malformed tenant payload: ${response.body}`);
  }
  const tenantID = typeof payload.id === 'string' ? payload.id : '';
  if (tenantID === '') {
    throw new Error(`${endpointTag} returned tenant payload without id: ${response.body}`);
  }
  return tenantID;
}

function parseTenantListResponse(response, endpointTag) {
  const payload = parseJSONResponse(response);
  if (payload === null || typeof payload !== 'object' || !Array.isArray(payload.items)) {
    throw new Error(`${endpointTag} returned malformed tenant list payload: ${response.body}`);
  }
  return payload.items;
}

function findTenantIDBySlug(baseURL, adminToken, slug, endpointTag) {
  const listTenantResponse = http.get(
    tenantListURL(baseURL),
    authJSONRequestOptions(endpointTag, buildAuthHeaders(adminToken), [200], 'GET'),
  );
  if (listTenantResponse.status !== 200) {
    throw new Error(`${endpointTag} failed with status ${listTenantResponse.status}: ${listTenantResponse.body}`);
  }
  const tenantItems = parseTenantListResponse(listTenantResponse, endpointTag);
  const matchingTenant = tenantItems.find(
    (tenant) =>
      tenant !== null &&
      typeof tenant === 'object' &&
      tenant.slug === slug &&
      typeof tenant.id === 'string' &&
      tenant.id !== '',
  );
  if (matchingTenant === undefined) {
    throw new Error(`${endpointTag} could not recover tenant id for slug ${slug}`);
  }
  return matchingTenant.id;
}

function bootstrapIdentityTenant(baseURL, adminToken, ownerUserID, identitySlot, endpointTag) {
  const slug = tenantSlug(identitySlot);
  const tenantRequestBody = JSON.stringify({
    name: slug,
    slug,
    ownerUserId: ownerUserID,
    isolationMode: DEFAULT_TENANT_ISOLATION_MODE,
    planTier: DEFAULT_TENANT_PLAN_TIER,
    idempotencyKey: slug,
  });
  const createTenantResponse = http.post(
    tenantAdminURL(baseURL),
    tenantRequestBody,
    authJSONRequestOptions(endpointTag, buildAuthHeaders(adminToken), [201, 409]),
  );
  if (createTenantResponse.status === 201) {
    return parseTenantBootstrapResponse(createTenantResponse, endpointTag);
  }
  if (
    createTenantResponse.status === 409 &&
    String(createTenantResponse.body).includes('tenant slug is already taken')
  ) {
    return findTenantIDBySlug(baseURL, adminToken, slug, `${endpointTag}_list`);
  }
  throw new Error(`${endpointTag} failed with status ${createTenantResponse.status}: ${createTenantResponse.body}`);
}

export function authPassword() {
  const configuredPassword = readEnv('AYB_LOAD_AUTH_PASSWORD');
  return configuredPassword === '' ? DEFAULT_AUTH_PASSWORD : configuredPassword;
}

export function authRegisterURL(baseURL) {
  return `${baseURL}${AUTH_REGISTER_PATH}`;
}

export function authLoginURL(baseURL) {
  return `${baseURL}${AUTH_LOGIN_PATH}`;
}

export function authRefreshURL(baseURL) {
  return `${baseURL}${AUTH_REFRESH_PATH}`;
}

export function loadWSUserPoolSize() {
  return parsePositiveInt(readEnv('AYB_WS_USER_POOL_SIZE'), DEFAULT_WS_USER_POOL_SIZE);
}

export function allocateLoadUserIdentity(vu) {
  const normalizedVU = normalizeVU(vu);
  const configuredPoolSize = loadWSUserPoolSize();
  const poolSize = configuredPoolSize > 0 ? configuredPoolSize : normalizedVU;
  const identitySlot = ((normalizedVU - 1) % poolSize) + 1;
  return {
    email: `load-auth-${RUN_NONCE}-slot${identitySlot}@example.test`,
    password: authPassword(),
    identitySlot,
    poolSize,
  };
}

export function uniqueAuthIdentity(vu, iteration) {
  return {
    email: `load-auth-${RUN_NONCE}-vu${vu}-iter${iteration}@example.test`,
    password: authPassword(),
  };
}

export function buildRegisterBody(email, password) {
  return buildCredentialBody(email, password);
}

export function buildLoginBody(email, password) {
  return buildCredentialBody(email, password);
}

export function buildRefreshBody(refreshToken) {
  return {
    refreshToken,
  };
}

function jsonRequestOptions(endpointTag, ...expectedStatuses) {
  return authJSONRequestOptions(endpointTag, {}, expectedStatuses);
}

function failMalformedAuthPayload(stageName, payload) {
  throw new Error(`${stageName} returned malformed auth payload: ${JSON.stringify(payload)}`);
}

export function parseAuthSuccessResponse(response, stageName) {
  const payload = parseJSONResponse(response);
  if (payload === null || typeof payload !== "object") {
    failMalformedAuthPayload(stageName, payload);
  }

  if (payload.mfa_pending === true) {
    throw new Error(`${stageName} entered MFA pending flow; Stage 3 scenario expects non-MFA success path`);
  }

  const token = typeof payload.token === "string" ? payload.token : "";
  const refreshToken = typeof payload.refreshToken === "string" ? payload.refreshToken : "";
  const user = payload.user;
  if (token === "" || refreshToken === "" || user === null || typeof user !== "object") {
    failMalformedAuthPayload(stageName, payload);
  }

  return {
    token,
    refreshToken,
    user,
  };
}

export function runAuthRegisterLoginRefreshFlow(baseURL, identity, endpointTags = {}, flowConfig = {}) {
  const includeNegativeChecks = flowConfig.includeNegativeChecks !== false;
  const allowNonSuccessStatuses = flowConfig.allowNonSuccessStatuses === true;
  const configuredExtraStatuses = Array.isArray(flowConfig.extraExpectedStatuses) ? flowConfig.extraExpectedStatuses : [];
  const extraExpectedStatuses = configuredExtraStatuses
    .map((statusCode) => Number.parseInt(String(statusCode), 10))
    .filter((statusCode) => Number.isFinite(statusCode) && statusCode >= 100 && statusCode <= 599);
  const withExtraExpectedStatuses = (...baseStatuses) => Array.from(new Set([...baseStatuses, ...extraExpectedStatuses]));
  const registerEndpointTag = endpointTags.registerEndpointTag || 'auth_register';
  const loginEndpointTag = endpointTags.loginEndpointTag || 'auth_login';
  const invalidLoginEndpointTag = endpointTags.invalidLoginEndpointTag || 'auth_login_invalid';
  const refreshEndpointTag = endpointTags.refreshEndpointTag || 'auth_refresh';
  const refreshReuseEndpointTag = endpointTags.refreshReuseEndpointTag || 'auth_refresh_reuse';
  const registerStatusCheck = allowNonSuccessStatuses
    ? (res) => res.status === 201 || extraExpectedStatuses.includes(res.status)
    : (res) => res.status === 201;
  const loginOrRefreshStatusCheck = allowNonSuccessStatuses
    ? (res) => res.status === 200 || extraExpectedStatuses.includes(res.status)
    : (res) => res.status === 200;
  const unauthorizedStatusCheck = allowNonSuccessStatuses
    ? (res) => res.status === 401 || extraExpectedStatuses.includes(res.status)
    : (res) => res.status === 401;

  const registerResponse = http.post(
    authRegisterURL(baseURL),
    JSON.stringify(buildRegisterBody(identity.email, identity.password)),
    jsonRequestOptions(registerEndpointTag, ...withExtraExpectedStatuses(201)),
  );
  assertResponseChecks(registerResponse, {
    'register responds with HTTP 201': registerStatusCheck,
  });
  if (registerResponse.status !== 201 && allowNonSuccessStatuses) {
    return null;
  }
  parseAuthSuccessResponse(registerResponse, registerEndpointTag);

  const loginResponse = http.post(
    authLoginURL(baseURL),
    JSON.stringify(buildLoginBody(identity.email, identity.password)),
    jsonRequestOptions(loginEndpointTag, ...withExtraExpectedStatuses(200)),
  );
  assertResponseChecks(loginResponse, {
    'login responds with HTTP 200': loginOrRefreshStatusCheck,
  });
  if (loginResponse.status !== 200 && allowNonSuccessStatuses) {
    return null;
  }
  const loginAuth = parseAuthSuccessResponse(loginResponse, loginEndpointTag);

  if (includeNegativeChecks) {
    const invalidLoginResponse = http.post(
      authLoginURL(baseURL),
      JSON.stringify(buildLoginBody(identity.email, `${identity.password}-invalid`)),
      jsonRequestOptions(invalidLoginEndpointTag, ...withExtraExpectedStatuses(401)),
    );
    assertResponseChecks(invalidLoginResponse, {
      'invalid credentials respond with HTTP 401': unauthorizedStatusCheck,
    });
    if (invalidLoginResponse.status !== 401 && allowNonSuccessStatuses) {
      return null;
    }
  }

  const refreshResponse = http.post(
    authRefreshURL(baseURL),
    JSON.stringify(buildRefreshBody(loginAuth.refreshToken)),
    jsonRequestOptions(refreshEndpointTag, ...withExtraExpectedStatuses(200)),
  );
  assertResponseChecks(refreshResponse, {
    'refresh responds with HTTP 200': loginOrRefreshStatusCheck,
  });
  if (refreshResponse.status !== 200 && allowNonSuccessStatuses) {
    return null;
  }
  const refreshAuth = parseAuthSuccessResponse(refreshResponse, refreshEndpointTag);

  assertResponseChecks(refreshResponse, {
    'rotated refresh token differs from consumed refresh token': () => refreshAuth.refreshToken !== loginAuth.refreshToken,
  });

  if (includeNegativeChecks) {
    const refreshReuseResponse = http.post(
      authRefreshURL(baseURL),
      JSON.stringify(buildRefreshBody(loginAuth.refreshToken)),
      jsonRequestOptions(refreshReuseEndpointTag, ...withExtraExpectedStatuses(401)),
    );
    assertResponseChecks(refreshReuseResponse, {
      'reused refresh token responds with HTTP 401': unauthorizedStatusCheck,
    });
    if (refreshReuseResponse.status !== 401 && allowNonSuccessStatuses) {
      return null;
    }
  }

  return {
    loginAuth,
    refreshAuth,
  };
}

export function bootstrapNonMFASession(baseURL, identity, endpointTags = {}) {
  const registerEndpointTag = endpointTags.registerEndpointTag || 'auth_register_bootstrap';
  const loginEndpointTag = endpointTags.loginEndpointTag || 'auth_login_bootstrap';
  const registerResponse = http.post(
    authRegisterURL(baseURL),
    JSON.stringify(buildRegisterBody(identity.email, identity.password)),
    authJSONRequestOptions(registerEndpointTag, {}, [201, 409]),
  );

  if (registerResponse.status === 201) {
    return parseAuthSuccessResponse(registerResponse, registerEndpointTag);
  }

  const loginResponse = http.post(
    authLoginURL(baseURL),
    JSON.stringify(buildLoginBody(identity.email, identity.password)),
    authJSONRequestOptions(loginEndpointTag, {}, [200]),
  );

  if (loginResponse.status !== 200) {
    throw new Error(`${loginEndpointTag} failed with status ${loginResponse.status}: ${loginResponse.body}`);
  }

  return parseAuthSuccessResponse(loginResponse, loginEndpointTag);
}

export function bootstrapTenantScopedSession(baseURL, identity, endpointTags = {}) {
  const tenantEndpointTag = endpointTags.tenantEndpointTag || 'auth_tenant_bootstrap';
  const authSession = bootstrapNonMFASession(baseURL, identity, endpointTags);
  const adminToken = readEnv('AYB_ADMIN_TOKEN');
  if (adminToken === '') {
    return {
      ...authSession,
      tenantID: '',
    };
  }

  const identitySlot = resolveIdentitySlot(identity);
  if (!tenantByIdentitySlot.has(identitySlot)) {
    const userPayload = authSession.user;
    const ownerUserID =
      userPayload !== null && typeof userPayload === 'object' && typeof userPayload.id === 'string'
        ? userPayload.id
        : '';
    if (ownerUserID === '') {
      throw new Error(`${tenantEndpointTag} requires auth session payload with user.id`);
    }
    const tenantID = bootstrapIdentityTenant(baseURL, adminToken, ownerUserID, identitySlot, tenantEndpointTag);
    tenantByIdentitySlot.set(identitySlot, tenantID);
  }

  return {
    ...authSession,
    tenantID: tenantByIdentitySlot.get(identitySlot),
  };
}

export function buildAuthScenarioOptions() {
  return loadScenarioOptions({
    scenarioName: 'auth_register_login_refresh',
    endpointThresholds: {
      auth_register: ['p(95)<1000'],
      auth_login: ['p(95)<1000'],
      auth_login_invalid: ['p(95)<1000'],
      auth_refresh: ['p(95)<1000'],
      auth_refresh_reuse: ['p(95)<1000'],
    },
  });
}
