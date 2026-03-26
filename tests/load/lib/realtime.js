import http from 'k6/http';

import { buildAuthHeaders } from './auth.js';
import { assertResponseChecks } from './checks.js';
import { dataCollectionListURL } from './data.js';
import { readEnv, trimTrailingSlashes } from './env.js';

const REALTIME_WS_PATH = '/api/realtime/ws';
const DEFAULT_REALTIME_TIMEOUT_MS = 5000;

function toWebSocketBaseURL(baseURL) {
  const normalized = trimTrailingSlashes(baseURL);
  if (normalized.startsWith('https://')) {
    return `wss://${normalized.slice('https://'.length)}`;
  }
  if (normalized.startsWith('http://')) {
    return `ws://${normalized.slice('http://'.length)}`;
  }
  throw new Error(`unsupported base URL for realtime websocket: ${baseURL}`);
}

export function realtimeWSURL(baseURL) {
  return `${toWebSocketBaseURL(baseURL)}${REALTIME_WS_PATH}`;
}

export function realtimeConnectParams(token, tenantIDOrTags = '', tags = {}) {
  const tenantID = typeof tenantIDOrTags === 'string' ? tenantIDOrTags : '';
  const resolvedTags = typeof tenantIDOrTags === 'string' ? tags : tenantIDOrTags;
  return {
    headers: buildAuthHeaders(token, tenantID),
    tags: {
      endpoint: 'realtime_ws_connect',
      method: 'GET',
      ...resolvedTags,
    },
  };
}

function jsonRequestOptions(endpointTag, method, authHeaders, ...expectedStatuses) {
  return {
    headers: {
      'Content-Type': 'application/json',
      ...authHeaders,
    },
    responseCallback: http.expectedStatuses(...expectedStatuses),
    tags: {
      endpoint: endpointTag,
      method,
    },
  };
}

function requireRealtimeReadableSubscription(
  baseURL,
  tableName,
  userHeaders,
  endpointTag,
  expectedStatuses,
  allowReadProbeFailure,
  abort,
) {
  const authProbeResponse = http.get(
    dataCollectionListURL(baseURL, tableName),
    jsonRequestOptions(endpointTag, 'GET', userHeaders, ...expectedStatuses),
  );
  if (authProbeResponse.status !== 200) {
    if (allowReadProbeFailure) {
      return false;
    }
    abort(
      `realtime auth probe requires the subscribed user to read ${tableName}; received HTTP ${authProbeResponse.status}`,
    );
  }
  return true;
}

function createRealtimeEventRow(
  baseURL,
  tableName,
  rowKey,
  userHeaders,
  adminHeaders,
  endpointTag,
  expectedStatuses,
  allowCreateFailure,
) {
  const requestBody = JSON.stringify({ id: rowKey, name: `realtime-${rowKey}`, status: 'active' });
  const createResponse = http.post(
    dataCollectionListURL(baseURL, tableName),
    requestBody,
    jsonRequestOptions(endpointTag, 'POST', userHeaders, ...expectedStatuses),
  );
  if (createResponse.status !== 201 && createResponse.status !== 401 && createResponse.status !== 403) {
    if (allowCreateFailure) {
      return false;
    }
    throw new Error(
      `realtime collection create failed with unexpected status ${createResponse.status}: ${createResponse.body}`,
    );
  }
  if (createResponse.status === 401 || createResponse.status === 403) {
    const adminRetryResponse = http.post(
      dataCollectionListURL(baseURL, tableName),
      requestBody,
      jsonRequestOptions(endpointTag, 'POST', adminHeaders, ...expectedStatuses),
    );
    if (adminRetryResponse.status !== 201) {
      if (allowCreateFailure) {
        return false;
      }
      throw new Error(
        `realtime collection create admin retry failed with status ${adminRetryResponse.status}: ${adminRetryResponse.body}`,
      );
    }
    assertResponseChecks(adminRetryResponse, {
      'realtime collection write returns HTTP 201': (res) => res.status === 201,
    });
    return true;
  }

  assertResponseChecks(createResponse, {
    'realtime collection write returns HTTP 201': (res) => res.status === 201,
  });
  return true;
}

export function buildRealtimeSubscribeMessage(tableName, ref = 'realtime-subscribe') {
  return {
    type: 'subscribe',
    ref,
    tables: [tableName],
  };
}

export function buildRealtimeUnsubscribeMessage(tableName, ref = 'realtime-unsubscribe') {
  return {
    type: 'unsubscribe',
    ref,
    tables: [tableName],
  };
}

export function parseRealtimeMessage(rawMessage) {
  try {
    return JSON.parse(rawMessage);
  } catch (_) {
    throw new Error(`realtime websocket message is not valid JSON: ${rawMessage}`);
  }
}

export function assertRealtimeConnectedMessage(message) {
  if (message === null || typeof message !== 'object' || Array.isArray(message)) {
    throw new Error(`connected message payload is malformed: ${JSON.stringify(message)}`);
  }
  if (message.type !== 'connected') {
    throw new Error(`expected connected message type, got: ${message.type}`);
  }
  if (typeof message.client_id !== 'string' || message.client_id === '') {
    throw new Error(`connected message missing client_id: ${JSON.stringify(message)}`);
  }
}

export function assertRealtimeReplyOK(message, expectedRef) {
  if (message === null || typeof message !== 'object' || Array.isArray(message)) {
    throw new Error(`reply message payload is malformed: ${JSON.stringify(message)}`);
  }
  if (message.type !== 'reply') {
    throw new Error(`expected reply message type, got: ${message.type}`);
  }
  if (message.status !== 'ok') {
    throw new Error(`expected reply status ok, got: ${message.status}`);
  }
  if (expectedRef !== '' && message.ref !== expectedRef) {
    throw new Error(`reply ref mismatch: expected ${expectedRef}, got ${message.ref}`);
  }
}

export function assertRealtimeEventMessage(message, expectedAction, expectedTable, expectedRecordID) {
  if (message === null || typeof message !== 'object' || Array.isArray(message)) {
    throw new Error(`event message payload is malformed: ${JSON.stringify(message)}`);
  }
  if (message.type !== 'event') {
    throw new Error(`expected event message type, got: ${message.type}`);
  }
  if (message.action !== expectedAction) {
    throw new Error(`event action mismatch: expected ${expectedAction}, got ${message.action}`);
  }
  if (message.table !== expectedTable) {
    throw new Error(`event table mismatch: expected ${expectedTable}, got ${message.table}`);
  }
  if (message.record === null || typeof message.record !== 'object' || Array.isArray(message.record)) {
    throw new Error(`event record payload is malformed: ${JSON.stringify(message)}`);
  }
  if (message.record.id !== expectedRecordID) {
    throw new Error(`event record.id mismatch: expected ${expectedRecordID}, got ${message.record.id}`);
  }
}

export function realtimeMessageTimeoutMillis() {
  const configured = Number.parseInt(readEnv('AYB_REALTIME_MESSAGE_TIMEOUT_MS'), 10);
  if (!Number.isFinite(configured) || configured < 1) {
    return DEFAULT_REALTIME_TIMEOUT_MS;
  }
  return configured;
}

function createRealtimeFlowState() {
  return {
    subscribeAcked: false,
    unsubscribeAcked: false,
    eventReceived: false,
    connectedSeen: false,
  };
}

function processRealtimeSocketMessage({
  rawMessage,
  socket,
  flowState,
  baseURL,
  tableName,
  rowKey,
  userHeaders,
  adminHeaders,
  createEndpointTag,
  createExpectedStatuses,
  allowCreateFailure,
  subscribeRef,
  unsubscribeRef,
  expectedEventAction,
}) {
  const message = parseRealtimeMessage(rawMessage);
  if (!flowState.connectedSeen) {
    assertRealtimeConnectedMessage(message);
    flowState.connectedSeen = true;
    socket.send(JSON.stringify(buildRealtimeSubscribeMessage(tableName, subscribeRef)));
    return;
  }

  if (!flowState.subscribeAcked) {
    assertRealtimeReplyOK(message, subscribeRef);
    flowState.subscribeAcked = true;
    const createdRealtimeRow = createRealtimeEventRow(
      baseURL,
      tableName,
      rowKey,
      userHeaders,
      adminHeaders,
      createEndpointTag,
      createExpectedStatuses,
      allowCreateFailure,
    );
    if (!createdRealtimeRow && allowCreateFailure) {
      flowState.eventReceived = true;
      socket.send(JSON.stringify(buildRealtimeUnsubscribeMessage(tableName, unsubscribeRef)));
    }
    return;
  }

  if (!flowState.eventReceived) {
    assertRealtimeEventMessage(message, expectedEventAction, tableName, rowKey);
    flowState.eventReceived = true;
    socket.send(JSON.stringify(buildRealtimeUnsubscribeMessage(tableName, unsubscribeRef)));
    return;
  }

  if (!flowState.unsubscribeAcked) {
    assertRealtimeReplyOK(message, unsubscribeRef);
    flowState.unsubscribeAcked = true;
    socket.close();
  }
}

function runRealtimeSocketFlow({
  ws,
  baseURL,
  token,
  tenantID,
  connectEndpointTag,
  tableName,
  abort,
  flowState,
  rowKey,
  userHeaders,
  adminHeaders,
  createEndpointTag,
  createExpectedStatuses,
  allowCreateFailure,
  subscribeRef,
  unsubscribeRef,
  expectedEventAction,
}) {
  return ws.connect(
    realtimeWSURL(baseURL),
    realtimeConnectParams(token, tenantID, {
      endpoint: connectEndpointTag,
    }),
    (socket) => {
      socket.setTimeout(() => {
        if (!flowState.unsubscribeAcked) {
          abort(`realtime websocket flow timed out waiting for unsubscribe ack for ${tableName}`);
        }
      }, realtimeMessageTimeoutMillis());

      socket.on('message', (rawMessage) => {
        try {
          processRealtimeSocketMessage({
            rawMessage,
            socket,
            flowState,
            baseURL,
            tableName,
            rowKey,
            userHeaders,
            adminHeaders,
            createEndpointTag,
            createExpectedStatuses,
            allowCreateFailure,
            subscribeRef,
            unsubscribeRef,
            expectedEventAction,
          });
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          abort(`realtime websocket flow failed: ${message}`);
        }
      });
    },
  );
}

export function runRealtimeSubscribeCreateEventUnsubscribeFlow({
  ws,
  baseURL,
  token,
  tenantID = '',
  tableName,
  rowKey,
  userHeaders,
  adminHeaders,
  abort,
  readProbeEndpointTag = 'realtime_auth_probe',
  readProbeExpectedStatuses = [200, 401, 403],
  allowReadProbeFailure = false,
  createEndpointTag = 'realtime_data_create',
  createExpectedStatuses = [201, 401, 403],
  allowCreateFailure = false,
  connectEndpointTag = 'realtime_ws_connect',
  subscribeRef = 'realtime-subscribe',
  unsubscribeRef = 'realtime-unsubscribe',
  expectedEventAction = 'create',
}) {
  const readableSubscription = requireRealtimeReadableSubscription(
    baseURL,
    tableName,
    userHeaders,
    readProbeEndpointTag,
    readProbeExpectedStatuses,
    allowReadProbeFailure,
    abort,
  );
  if (!readableSubscription) {
    return null;
  }

  const flowState = createRealtimeFlowState();
  const connectResponse = runRealtimeSocketFlow({
    ws,
    baseURL,
    token,
    tenantID,
    connectEndpointTag,
    tableName,
    abort,
    flowState,
    rowKey,
    userHeaders,
    adminHeaders,
    createEndpointTag,
    createExpectedStatuses,
    allowCreateFailure,
    subscribeRef,
    unsubscribeRef,
    expectedEventAction,
  });

  if (connectResponse === null || connectResponse.status !== 101) {
    abort(
      `realtime websocket upgrade failed with status ${connectResponse === null ? 'null' : connectResponse.status}`,
    );
  }
  assertResponseChecks(connectResponse, {
    'realtime websocket upgrade returns HTTP 101': (response) => response !== null && response.status === 101,
  });
}
