import ws from 'k6/ws';
import exec from 'k6/execution';

import { allocateLoadUserIdentity, authSessionHeaders, bootstrapTenantScopedSession } from '../lib/auth.js';
import {
  createDataFixture,
  dataAdminRequestHeaders,
  dropDataFixture,
  loadDataRunTableName,
} from '../lib/data.js';
import { loadBaseURL, loadScenarioOptions } from '../lib/env.js';
import { runRealtimeSubscribeCreateEventUnsubscribeFlow } from '../lib/realtime.js';

const BASE_URL = loadBaseURL();
export const options = loadScenarioOptions({
  scenarioName: 'realtime_ws_subscribe',
  endpointThresholds: {
    realtime_ws_connect: ['p(95)<1200'],
    realtime_data_create: ['p(95)<1200'],
  },
});

function abortRealtimeScenario(message) {
  exec.test.abort(message);
}

export function setup() {
  const tableName = loadDataRunTableName();
  createDataFixture(BASE_URL, dataAdminRequestHeaders(), tableName);
  return { tableName };
}

export function teardown(setupData) {
  dropDataFixture(BASE_URL, dataAdminRequestHeaders(), setupData.tableName);
}

export default function runRealtimeWSSubscribe(setupData) {
  const tableName = setupData.tableName;
  const identity = allocateLoadUserIdentity(__VU);
  const authSession = bootstrapTenantScopedSession(BASE_URL, identity, {
    registerEndpointTag: 'realtime_auth_register',
    loginEndpointTag: 'realtime_auth_login',
    tenantEndpointTag: 'realtime_auth_tenant',
  });
  const userHeaders = authSessionHeaders(authSession);
  const rowKey = `realtime-vu${__VU}-iter${__ITER}`;

  runRealtimeSubscribeCreateEventUnsubscribeFlow({
    ws,
    baseURL: BASE_URL,
    token: authSession.token,
    tenantID: authSession.tenantID,
    tableName,
    rowKey,
    userHeaders,
    adminHeaders: dataAdminRequestHeaders(),
    abort: abortRealtimeScenario,
  });
}
