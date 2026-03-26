import http from 'k6/http';

import { adminAuthHeaders } from './admin.js';
import { allocateLoadUserIdentity, authSessionHeaders, bootstrapTenantScopedSession } from './auth.js';
import { assertResponseChecks } from './checks.js';
import { readEnv, trimTrailingSlashes } from './env.js';

const ADMIN_SQL_PATH = '/api/admin/sql/';
const COLLECTION_PATH = '/api/collections/';
const DEFAULT_DATA_TABLE_PREFIX = 'load_stage4_items';
const TABLE_IDENTIFIER_MAX_LENGTH = 63;
const RUN_NONCE = Date.now().toString(36);

function sanitizeIdentifier(rawIdentifier) {
  const normalized = rawIdentifier.toLowerCase().replace(/[^a-z0-9_]/g, '_').replace(/^_+|_+$/g, '');
  return normalized === '' ? DEFAULT_DATA_TABLE_PREFIX : normalized;
}

function quoteIdentifier(identifier) {
  return `"${identifier.replace(/"/g, '""')}"`;
}

function buildLoadFixtureTableQuery(tableName) {
  const fixtureTable = quoteIdentifier(tableName);
  return `
CREATE TABLE IF NOT EXISTS ${fixtureTable} (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`;
}

function buildLoadFixtureAccessQuery(tableName) {
  const fixtureTable = quoteIdentifier(tableName);
  return `
DO $$ BEGIN
  CREATE ROLE ayb_authenticated NOLOGIN;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
GRANT USAGE ON SCHEMA public TO ayb_authenticated;
ALTER TABLE ${fixtureTable} ENABLE ROW LEVEL SECURITY;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE ${fixtureTable} TO ayb_authenticated;
DROP POLICY IF EXISTS load_fixture_select ON ${fixtureTable};
CREATE POLICY load_fixture_select ON ${fixtureTable} FOR SELECT
  TO ayb_authenticated
  USING (true);
DROP POLICY IF EXISTS load_fixture_insert ON ${fixtureTable};
CREATE POLICY load_fixture_insert ON ${fixtureTable} FOR INSERT
  TO ayb_authenticated
  WITH CHECK (true);
DROP POLICY IF EXISTS load_fixture_update ON ${fixtureTable};
CREATE POLICY load_fixture_update ON ${fixtureTable} FOR UPDATE
  TO ayb_authenticated
  USING (true)
  WITH CHECK (true);
DROP POLICY IF EXISTS load_fixture_delete ON ${fixtureTable};
CREATE POLICY load_fixture_delete ON ${fixtureTable} FOR DELETE
  TO ayb_authenticated
  USING (true)`;
}

function dataJSONHeaders(headers = {}) {
  return {
    'Content-Type': 'application/json',
    ...headers,
  };
}

function dataJSONRequestOptions(endpointTag, method, headers, ...expectedStatuses) {
  return {
    headers: dataJSONHeaders(headers),
    responseCallback: http.expectedStatuses(...expectedStatuses),
    tags: {
      endpoint: endpointTag,
      method,
    },
  };
}

function readRequestOptions(endpointTag, authHeaders, ...expectedStatuses) {
  return {
    headers: {
      ...authHeaders,
    },
    responseCallback: http.expectedStatuses(...expectedStatuses),
    tags: {
      endpoint: endpointTag,
      method: 'GET',
    },
  };
}

function deleteRequestOptions(endpointTag, authHeaders, ...expectedStatuses) {
  return {
    headers: {
      ...authHeaders,
    },
    responseCallback: http.expectedStatuses(...expectedStatuses),
    tags: {
      endpoint: endpointTag,
      method: 'DELETE',
    },
  };
}

function requireStatus(response, expectedStatus, contextLabel) {
  if (response.status !== expectedStatus) {
    throw new Error(`${contextLabel} failed with status ${response.status}: ${response.body}`);
  }
}

function buildLoadDataTableName(tablePrefix) {
  const suffix = `_${RUN_NONCE}`;
  const maxPrefixLength = Math.max(1, TABLE_IDENTIFIER_MAX_LENGTH - suffix.length);
  const boundedPrefix = tablePrefix.slice(0, maxPrefixLength).replace(/_+$/g, '');
  const fallbackPrefix = DEFAULT_DATA_TABLE_PREFIX.slice(0, maxPrefixLength).replace(/_+$/g, '');
  const resolvedPrefix = boundedPrefix === '' ? fallbackPrefix : boundedPrefix;
  return `${resolvedPrefix}${suffix}`;
}

export function loadDataRunTableName() {
  const configuredPrefix = readEnv('AYB_LOAD_DATA_TABLE_PREFIX');
  const tablePrefix = sanitizeIdentifier(configuredPrefix === '' ? DEFAULT_DATA_TABLE_PREFIX : configuredPrefix);
  return buildLoadDataTableName(tablePrefix);
}

export function dataAdminSQLURL(baseURL) {
  return `${trimTrailingSlashes(baseURL)}${ADMIN_SQL_PATH}`;
}

export function dataCollectionListURL(baseURL, tableName) {
  return `${trimTrailingSlashes(baseURL)}${COLLECTION_PATH}${encodeURIComponent(tableName)}/`;
}

export function dataCollectionReadURL(baseURL, tableName, rowKey) {
  return `${dataCollectionListURL(baseURL, tableName)}${encodeURIComponent(rowKey)}`;
}

export function dataCollectionBatchURL(baseURL, tableName) {
  return `${trimTrailingSlashes(baseURL)}${COLLECTION_PATH}${encodeURIComponent(tableName)}/batch`;
}

export function dataAdminRequestHeaders() {
  return dataJSONHeaders(adminAuthHeaders());
}

export function runAdminSQL(baseURL, headers, query, config = {}) {
  const requestConfig = typeof config === 'string' ? { endpointTag: config } : config;
  const {
    endpointTag = 'admin_sql_fixture',
    expectedStatuses = [200],
    requireStatus: requiredStatus = 200,
  } = requestConfig;
  const response = http.post(
    dataAdminSQLURL(baseURL),
    JSON.stringify({ query }),
    dataJSONRequestOptions(endpointTag, 'POST', headers, ...expectedStatuses),
  );
  if (typeof requiredStatus === 'number') {
    requireStatus(response, requiredStatus, endpointTag);
  }
  return response;
}

export function createDataFixture(baseURL, headers, tableName) {
  runAdminSQL(baseURL, headers, buildLoadFixtureTableQuery(tableName), 'admin_sql_fixture_create');
  return runAdminSQL(baseURL, headers, buildLoadFixtureAccessQuery(tableName), 'admin_sql_fixture_access');
}

export function dropDataFixture(baseURL, headers, tableName) {
  const fixtureTable = quoteIdentifier(tableName);
  const teardownQuery = `DROP TABLE IF EXISTS ${fixtureTable}`;
  return runAdminSQL(baseURL, headers, teardownQuery, 'admin_sql_fixture_drop');
}

function resolveDataFlowEndpointTags(endpointTags = {}) {
  return {
    listEndpointTag: endpointTags.listEndpointTag || 'data_list',
    createEndpointTag: endpointTags.createEndpointTag || 'data_create',
    readEndpointTag: endpointTags.readEndpointTag || 'data_read',
    updateEndpointTag: endpointTags.updateEndpointTag || 'data_update',
    batchEndpointTag: endpointTags.batchEndpointTag || 'data_batch',
    rollbackEndpointTag: endpointTags.rollbackEndpointTag || 'data_batch_rollback_probe',
    deleteEndpointTag: endpointTags.deleteEndpointTag || 'data_delete',
  };
}

function runDataListStep(baseURL, tableName, userHeaders, listEndpointTag) {
  const listResponse = http.get(
    dataCollectionListURL(baseURL, tableName),
    readRequestOptions(listEndpointTag, userHeaders, 200),
  );
  assertResponseChecks(listResponse, {
    'collection list returns HTTP 200': (res) => res.status === 200,
  });
}

function runDataCreateStep(baseURL, tableName, userHeaders, rowKey, createEndpointTag) {
  const createResponse = http.post(
    dataCollectionListURL(baseURL, tableName),
    JSON.stringify({ id: rowKey, name: `row-${rowKey}`, status: 'active' }),
    dataJSONRequestOptions(createEndpointTag, 'POST', userHeaders, 201),
  );
  assertResponseChecks(createResponse, {
    'collection create returns HTTP 201': (res) => res.status === 201,
  });
}

function runDataReadStep(baseURL, tableName, userHeaders, rowKey, readEndpointTag) {
  const readResponse = http.get(
    dataCollectionReadURL(baseURL, tableName, rowKey),
    readRequestOptions(readEndpointTag, userHeaders, 200),
  );
  assertResponseChecks(readResponse, {
    'collection read returns HTTP 200': (res) => res.status === 200,
  });
}

function runDataUpdateStep(baseURL, tableName, userHeaders, rowKey, updateEndpointTag) {
  const updateResponse = http.patch(
    dataCollectionReadURL(baseURL, tableName, rowKey),
    JSON.stringify({ status: 'updated' }),
    dataJSONRequestOptions(updateEndpointTag, 'PATCH', userHeaders, 200),
  );
  assertResponseChecks(updateResponse, {
    'collection update returns HTTP 200': (res) => res.status === 200,
  });
}

function runDataBatchStep(baseURL, tableName, userHeaders, rowKey, batchRowKey, batchEndpointTag) {
  const batchResponse = http.post(
    dataCollectionBatchURL(baseURL, tableName),
    JSON.stringify({
      operations: [
        { method: 'create', body: { id: batchRowKey, name: `batch-${rowKey}`, status: 'active' } },
        { method: 'update', id: rowKey, body: { status: 'batch-updated' } },
        { method: 'delete', id: batchRowKey },
      ],
    }),
    dataJSONRequestOptions(batchEndpointTag, 'POST', userHeaders, 200),
  );
  assertResponseChecks(batchResponse, {
    'collection batch returns HTTP 200': (res) => res.status === 200,
  });
}

function runDataBatchRollbackProbeStep(baseURL, tableName, userHeaders, rowKey, rollbackRowKey, rollbackEndpointTag) {
  const failedBatchResponse = http.post(
    dataCollectionBatchURL(baseURL, tableName),
    JSON.stringify({
      operations: [
        { method: 'create', body: { id: rollbackRowKey, name: `rollback-${rowKey}`, status: 'active' } },
        { method: 'create', body: { id: rollbackRowKey, name: `rollback-duplicate-${rowKey}`, status: 'active' } },
      ],
    }),
    dataJSONRequestOptions(rollbackEndpointTag, 'POST', userHeaders, 409),
  );
  assertResponseChecks(failedBatchResponse, {
    'failed batch mutation responds with HTTP 409': (res) => res.status === 409,
  });

  const rollbackReadResponse = http.get(
    dataCollectionReadURL(baseURL, tableName, rollbackRowKey),
    readRequestOptions(rollbackEndpointTag, userHeaders, 404),
  );
  assertResponseChecks(rollbackReadResponse, {
    'rollback probe rejects partial commit': (res) => res.status === 404,
  });
}

function runDataDeleteStep(baseURL, tableName, userHeaders, rowKey, deleteEndpointTag) {
  const deleteResponse = http.del(
    dataCollectionReadURL(baseURL, tableName, rowKey),
    null,
    deleteRequestOptions(deleteEndpointTag, userHeaders, 204),
  );
  assertResponseChecks(deleteResponse, {
    'collection delete returns HTTP 204': (res) => res.status === 204,
  });
}

function runDataCRUDBatchFlowSteps({
  baseURL,
  tableName,
  userHeaders,
  rowKey,
  batchRowKey,
  rollbackRowKey,
  resolvedTags,
}) {
  runDataListStep(baseURL, tableName, userHeaders, resolvedTags.listEndpointTag);
  runDataCreateStep(baseURL, tableName, userHeaders, rowKey, resolvedTags.createEndpointTag);
  runDataReadStep(baseURL, tableName, userHeaders, rowKey, resolvedTags.readEndpointTag);
  runDataUpdateStep(baseURL, tableName, userHeaders, rowKey, resolvedTags.updateEndpointTag);
  runDataBatchStep(baseURL, tableName, userHeaders, rowKey, batchRowKey, resolvedTags.batchEndpointTag);
  runDataBatchRollbackProbeStep(
    baseURL,
    tableName,
    userHeaders,
    rowKey,
    rollbackRowKey,
    resolvedTags.rollbackEndpointTag,
  );
  runDataDeleteStep(baseURL, tableName, userHeaders, rowKey, resolvedTags.deleteEndpointTag);
}

export function runDataPathCRUDAndBatchFlow({
  baseURL,
  tableName,
  userHeaders,
  rowKey = `vu${__VU}-iter${__ITER}`,
  endpointTags = {},
}) {
  const resolvedTags = resolveDataFlowEndpointTags(endpointTags);
  const batchRowKey = `${rowKey}-batch`;
  const rollbackRowKey = `${rowKey}-rollback`;
  runDataCRUDBatchFlowSteps({
    baseURL,
    tableName,
    userHeaders,
    rowKey,
    batchRowKey,
    rollbackRowKey,
    resolvedTags,
  });

  return {
    rowKey,
    batchRowKey,
    rollbackRowKey,
  };
}

export function bootstrapDataUserHeaders(baseURL) {
  const bootstrapIdentity = allocateLoadUserIdentity(1);
  const loginAuth = bootstrapTenantScopedSession(baseURL, bootstrapIdentity, {
    registerEndpointTag: 'data_auth_register',
    loginEndpointTag: 'data_auth_login',
    tenantEndpointTag: 'data_auth_tenant',
  });
  return authSessionHeaders(loginAuth);
}
