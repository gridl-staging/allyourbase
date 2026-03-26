/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/edge-function-mock-state.ts.
 */
/**
 * TODO: Document EdgeFunctionMockState.
 */
export interface EdgeFunctionMockState {
  listCalls: number;
  deployCalls: number;
  lastDeployBody: Record<string, unknown> | null;
  updateCalls: number;
  lastUpdateBody: Record<string, unknown> | null;
  deleteCalls: number;
  lastDeletedId: string | null;
  invokeCalls: number;
  lastInvokeBody: Record<string, unknown> | null;
  logCalls: number;

  dbListCalls: number;
  dbCreateCalls: number;
  lastDBCreateBody: Record<string, unknown> | null;
  lastCreatedDBTriggerId: string | null;
  dbEnableCalls: number;
  lastDBEnabledId: string | null;
  dbDisableCalls: number;
  lastDBDisabledId: string | null;
  dbDeleteCalls: number;
  lastDBDeletedId: string | null;

  cronListCalls: number;
  cronCreateCalls: number;
  lastCronCreateBody: Record<string, unknown> | null;
  lastCreatedCronTriggerId: string | null;
  cronEnableCalls: number;
  lastCronEnabledId: string | null;
  cronDisableCalls: number;
  lastCronDisabledId: string | null;
  cronDeleteCalls: number;
  lastCronDeletedId: string | null;
  cronManualRunCalls: number;
  lastCronManualRunId: string | null;

  storageListCalls: number;
  storageCreateCalls: number;
  lastStorageCreateBody: Record<string, unknown> | null;
  lastCreatedStorageTriggerId: string | null;
  storageEnableCalls: number;
  lastStorageEnabledId: string | null;
  storageDisableCalls: number;
  lastStorageDisabledId: string | null;
  storageDeleteCalls: number;
  lastStorageDeletedId: string | null;
}

import type { MockApiResponse } from "./core";

export interface EdgeFunctionMockOptions {
  deployResponder?: (body: Record<string, unknown>) => MockApiResponse;
  updateResponder?: (fnId: string, body: Record<string, unknown>) => MockApiResponse;
  invokeResponder?: (fnId: string, body: Record<string, unknown>) => MockApiResponse;
  listResponder?: () => MockApiResponse;
}

const defaultEdgeFunctions = [
  {
    id: "ef-001",
    name: "hello-world",
    entryPoint: "handler",
    timeout: 5000000000,
    public: true,
    source: 'export default function handler(req) { return { statusCode: 200, body: JSON.stringify({ message: "Hello!" }), headers: { "Content-Type": "application/json" } }; }',
    compiledJs: "",
    lastInvokedAt: "2026-02-20T10:00:00Z",
    envVars: { API_KEY: "test-key-123" },
    createdAt: "2026-02-01T08:00:00Z",
    updatedAt: "2026-02-20T10:00:00Z",
  },
  {
    id: "ef-002",
    name: "auth-check",
    entryPoint: "handler",
    timeout: 3000000000,
    public: false,
    source: 'export default function handler(req) { return { statusCode: 401, body: "Unauthorized" }; }',
    compiledJs: "",
    lastInvokedAt: null,
    envVars: {},
    createdAt: "2026-02-10T12:00:00Z",
    updatedAt: "2026-02-10T12:00:00Z",
  },
];

const defaultEdgeFunctionLogs = [
  {
    id: "log-001",
    functionId: "ef-001",
    invocationId: "inv-001",
    status: "success",
    durationMs: 42,
    stdout: "console output here",
    error: null,
    requestMethod: "GET",
    requestPath: "/hello-world",
    triggerType: "http",
    triggerId: null,
    parentInvocationId: null,
    createdAt: "2026-02-20T10:00:00Z",
  },
  {
    id: "log-002",
    functionId: "ef-001",
    invocationId: "inv-002",
    status: "error",
    durationMs: 5001,
    stdout: "",
    error: "execution timeout: 5s exceeded",
    requestMethod: "POST",
    requestPath: "/hello-world",
    triggerType: "http",
    triggerId: null,
    parentInvocationId: null,
    createdAt: "2026-02-20T09:00:00Z",
  },
  {
    id: "log-003",
    functionId: "ef-001",
    invocationId: "inv-003",
    status: "success",
    durationMs: 18,
    stdout: "db event processed",
    error: null,
    requestMethod: "POST",
    requestPath: "/db-event",
    triggerType: "db",
    triggerId: "dbt-001",
    parentInvocationId: null,
    createdAt: "2026-02-20T08:30:00Z",
  },
];

const defaultDBTriggers = [
  {
    id: "dbt-001",
    functionId: "ef-001",
    tableName: "users",
    schema: "public",
    events: ["INSERT", "UPDATE"],
    filterColumns: ["email"],
    enabled: true,
    createdAt: "2026-02-20T08:00:00Z",
    updatedAt: "2026-02-20T08:00:00Z",
  },
];

const defaultCronTriggers = [
  {
    id: "ct-001",
    functionId: "ef-001",
    scheduleId: "sched-001",
    cronExpr: "*/15 * * * *",
    timezone: "UTC",
    payload: { source: "seed" },
    enabled: true,
    createdAt: "2026-02-20T08:05:00Z",
    updatedAt: "2026-02-20T08:05:00Z",
  },
];

const defaultStorageTriggers = [
  {
    id: "st-001",
    functionId: "ef-001",
    bucket: "uploads",
    eventTypes: ["upload"],
    prefixFilter: "",
    suffixFilter: ".jpg",
    enabled: true,
    createdAt: "2026-02-20T08:10:00Z",
    updatedAt: "2026-02-20T08:10:00Z",
  },
];

export type EdgeFunctionRecord = (typeof defaultEdgeFunctions)[number];
export type EdgeFunctionLogRecord = (typeof defaultEdgeFunctionLogs)[number];
export type DBTriggerRecord = (typeof defaultDBTriggers)[number];
export type CronTriggerRecord = (typeof defaultCronTriggers)[number];
export type StorageTriggerRecord = (typeof defaultStorageTriggers)[number];

export interface EdgeFunctionMockStore {
  edgeFunctions: EdgeFunctionRecord[];
  edgeFunctionLogs: EdgeFunctionLogRecord[];
  dbTriggers: DBTriggerRecord[];
  cronTriggers: CronTriggerRecord[];
  storageTriggers: StorageTriggerRecord[];
  nextFunctionNumber: number;
  nextLogNumber: number;
  nextDBTriggerNumber: number;
  nextCronTriggerNumber: number;
  nextStorageTriggerNumber: number;
}

/**
 * TODO: Document createEdgeFunctionMockState.
 */
export function createEdgeFunctionMockState(): EdgeFunctionMockState {
  return {
    listCalls: 0,
    deployCalls: 0,
    lastDeployBody: null,
    updateCalls: 0,
    lastUpdateBody: null,
    deleteCalls: 0,
    lastDeletedId: null,
    invokeCalls: 0,
    lastInvokeBody: null,
    logCalls: 0,
    dbListCalls: 0,
    dbCreateCalls: 0,
    lastDBCreateBody: null,
    lastCreatedDBTriggerId: null,
    dbEnableCalls: 0,
    lastDBEnabledId: null,
    dbDisableCalls: 0,
    lastDBDisabledId: null,
    dbDeleteCalls: 0,
    lastDBDeletedId: null,
    cronListCalls: 0,
    cronCreateCalls: 0,
    lastCronCreateBody: null,
    lastCreatedCronTriggerId: null,
    cronEnableCalls: 0,
    lastCronEnabledId: null,
    cronDisableCalls: 0,
    lastCronDisabledId: null,
    cronDeleteCalls: 0,
    lastCronDeletedId: null,
    cronManualRunCalls: 0,
    lastCronManualRunId: null,
    storageListCalls: 0,
    storageCreateCalls: 0,
    lastStorageCreateBody: null,
    lastCreatedStorageTriggerId: null,
    storageEnableCalls: 0,
    lastStorageEnabledId: null,
    storageDisableCalls: 0,
    lastStorageDisabledId: null,
    storageDeleteCalls: 0,
    lastStorageDeletedId: null,
  };
}

/**
 * TODO: Document createEdgeFunctionMockStore.
 */
export function createEdgeFunctionMockStore(): EdgeFunctionMockStore {
  const edgeFunctions = defaultEdgeFunctions.map((fn) => ({ ...fn, envVars: { ...fn.envVars } }));
  const edgeFunctionLogs = defaultEdgeFunctionLogs.map((log) => ({ ...log }));
  const dbTriggers = defaultDBTriggers.map((trigger) => ({
    ...trigger,
    events: [...trigger.events],
    filterColumns: [...trigger.filterColumns],
  }));
  const cronTriggers = defaultCronTriggers.map((trigger) => ({
    ...trigger,
    payload: { ...trigger.payload },
  }));
  const storageTriggers = defaultStorageTriggers.map((trigger) => ({
    ...trigger,
    eventTypes: [...trigger.eventTypes],
  }));

  return {
    edgeFunctions,
    edgeFunctionLogs,
    dbTriggers,
    cronTriggers,
    storageTriggers,
    nextFunctionNumber: nextSequenceNumber(edgeFunctions, /^ef-(\d+)$/),
    nextLogNumber: nextSequenceNumber(edgeFunctionLogs, /^log-(\d+)$/),
    nextDBTriggerNumber: nextSequenceNumber(dbTriggers, /^dbt-(\d+)$/),
    nextCronTriggerNumber: nextSequenceNumber(cronTriggers, /^ct-(\d+)$/),
    nextStorageTriggerNumber: nextSequenceNumber(storageTriggers, /^st-(\d+)$/),
  };
}

function nextSequenceNumber(items: Array<{ id: string }>, pattern: RegExp): number {
  return (
    items.reduce((max, item) => {
      const match = item.id.match(pattern);
      if (!match) return max;
      return Math.max(max, Number(match[1]));
    }, 0) + 1
  );
}
