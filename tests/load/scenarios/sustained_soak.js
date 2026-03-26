import { sleep } from 'k6';
import ws from 'k6/ws';
import exec from 'k6/execution';

import {
  allocateLoadUserIdentity,
  authSessionHeaders,
  bootstrapTenantScopedSession,
  runAuthRegisterLoginRefreshFlow,
  uniqueAuthIdentity,
} from '../lib/auth.js';
import {
  createDataFixture,
  dataAdminRequestHeaders,
  dropDataFixture,
  loadDataRunTableName,
  runDataPathCRUDAndBatchFlow,
} from '../lib/data.js';
import { loadBaseURL, loadSustainedSoakOptions, readEnv } from '../lib/env.js';
import { runRealtimeSubscribeCreateEventUnsubscribeFlow } from '../lib/realtime.js';

const BASE_URL = loadBaseURL();
const sessionByIdentitySlot = new Map();
const DEFAULT_SOAK_LOOP_SLEEP_SECONDS = 0.25;
const DEFAULT_POOLED_SESSION_MAX_AGE_MS = 10 * 60 * 1000;

function loadSoakLoopSleepSeconds() {
  const configured = Number.parseFloat(readEnv('AYB_SOAK_LOOP_SLEEP_SECONDS'));
  if (!Number.isFinite(configured) || configured < 0) {
    return DEFAULT_SOAK_LOOP_SLEEP_SECONDS;
  }
  return configured;
}

const SOAK_LOOP_SLEEP_SECONDS = loadSoakLoopSleepSeconds();

export const options = loadSustainedSoakOptions({
  scenarioName: 'sustained_soak',
  endpointThresholds: {
    soak_auth_register: ['p(95)<1200'],
    soak_auth_login: ['p(95)<1200'],
    soak_auth_login_invalid: ['p(95)<1200'],
    soak_auth_refresh: ['p(95)<1200'],
    soak_auth_refresh_reuse: ['p(95)<1200'],
    soak_data_list: ['p(95)<1500'],
    soak_data_create: ['p(95)<1500'],
    soak_data_read: ['p(95)<1500'],
    soak_data_update: ['p(95)<1500'],
    soak_data_batch: ['p(95)<1500'],
    soak_data_batch_rollback_probe: ['p(95)<1500'],
    soak_data_delete: ['p(95)<1500'],
    soak_realtime_ws_connect: ['p(95)<1500'],
    soak_realtime_data_create: ['p(95)<1500'],
  },
});

function abortSustainedSoak(message) {
  exec.test.abort(message);
}

function isReusablePooledSession(cachedSessionEntry, nowMillis) {
  if (cachedSessionEntry === undefined) {
    return false;
  }
  return nowMillis - cachedSessionEntry.resolvedAtMillis < DEFAULT_POOLED_SESSION_MAX_AGE_MS;
}

function resolvePooledSession(vuIdentity) {
  const nowMillis = Date.now();
  const cachedSessionEntry = sessionByIdentitySlot.get(vuIdentity.identitySlot);
  if (isReusablePooledSession(cachedSessionEntry, nowMillis)) {
    return cachedSessionEntry.session;
  }

  const resolvedSession = bootstrapTenantScopedSession(BASE_URL, vuIdentity, {
    registerEndpointTag: 'soak_session_register',
    loginEndpointTag: 'soak_session_login',
    tenantEndpointTag: 'soak_session_tenant',
  });
  // Long soaks can outlive the default 15-minute JWT; periodically rebootstrap to avoid stale tokens.
  sessionByIdentitySlot.set(vuIdentity.identitySlot, {
    session: resolvedSession,
    resolvedAtMillis: nowMillis,
  });
  return resolvedSession;
}

export function setup() {
  const tableName = loadDataRunTableName();
  createDataFixture(BASE_URL, dataAdminRequestHeaders(), tableName);
  return { tableName };
}

export function teardown(setupData) {
  dropDataFixture(BASE_URL, dataAdminRequestHeaders(), setupData.tableName);
}

export default function runSustainedSoak(setupData) {
  const tableName = setupData.tableName;
  const pooledIdentity = allocateLoadUserIdentity(__VU);
  const pooledSession = resolvePooledSession(pooledIdentity);
  const userHeaders = authSessionHeaders(pooledSession);
  const rowSeed = `soak-vu${__VU}-iter${__ITER}`;

  runAuthRegisterLoginRefreshFlow(BASE_URL, uniqueAuthIdentity(__VU, __ITER), {
    registerEndpointTag: 'soak_auth_register',
    loginEndpointTag: 'soak_auth_login',
    invalidLoginEndpointTag: 'soak_auth_login_invalid',
    refreshEndpointTag: 'soak_auth_refresh',
    refreshReuseEndpointTag: 'soak_auth_refresh_reuse',
  }, {
    includeNegativeChecks: false,
    allowNonSuccessStatuses: true,
    extraExpectedStatuses: [429],
  });

  runDataPathCRUDAndBatchFlow({
    baseURL: BASE_URL,
    tableName,
    userHeaders,
    rowKey: `${rowSeed}-data`,
    endpointTags: {
      listEndpointTag: 'soak_data_list',
      createEndpointTag: 'soak_data_create',
      readEndpointTag: 'soak_data_read',
      updateEndpointTag: 'soak_data_update',
      batchEndpointTag: 'soak_data_batch',
      rollbackEndpointTag: 'soak_data_batch_rollback_probe',
      deleteEndpointTag: 'soak_data_delete',
    },
  });

  runRealtimeSubscribeCreateEventUnsubscribeFlow({
    ws,
    baseURL: BASE_URL,
    token: pooledSession.token,
    tenantID: pooledSession.tenantID,
    tableName,
    rowKey: `${rowSeed}-realtime`,
    userHeaders,
    adminHeaders: dataAdminRequestHeaders(),
    abort: abortSustainedSoak,
    readProbeEndpointTag: 'soak_realtime_auth_probe',
    readProbeExpectedStatuses: [200, 401, 403, 429],
    allowReadProbeFailure: true,
    createEndpointTag: 'soak_realtime_data_create',
    createExpectedStatuses: [201, 401, 403, 429],
    allowCreateFailure: true,
    connectEndpointTag: 'soak_realtime_ws_connect',
  });

  sleep(SOAK_LOOP_SLEEP_SECONDS);
}
