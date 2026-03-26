/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/edge-function-routes.ts.
 */
import type { Route } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute } from "./core";
import type {
  CronTriggerRecord,
  DBTriggerRecord,
  EdgeFunctionMockOptions,
  EdgeFunctionMockState,
  EdgeFunctionMockStore,
  EdgeFunctionRecord,
  EdgeFunctionLogRecord,
  StorageTriggerRecord,
} from "./edge-function-mock-state";

interface EdgeFunctionRouteContext {
  route: Route;
  request: ReturnType<Route["request"]>;
  method: string;
  path: string;
  url: URL;
}

/**
 * TODO: Document handleEdgeFunctionApiRoute.
 */
export async function handleEdgeFunctionApiRoute(
  route: Route,
  options: EdgeFunctionMockOptions | undefined,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<void> {
  const context = createRouteContext(route);

  if (await handleCommonAdminRoutes(context.route, context.method, context.path)) return;
  if (await handleEdgeFunctionCollectionRoute(context, options, state, store)) return;
  if (await handleEdgeFunctionDetailRoute(context, store)) return;
  if (await handleEdgeFunctionUpdateRoute(context, options, state, store)) return;
  if (await handleEdgeFunctionDeleteRoute(context, state, store)) return;
  if (await handleEdgeFunctionLogsRoute(context, state, store)) return;
  if (await handleDBTriggerRoute(context, state, store)) return;
  if (await handleCronTriggerRoute(context, state, store)) return;
  if (await handleStorageTriggerRoute(context, state, store)) return;
  if (await handleInvokeRoute(context, options, state, store)) return;

  await unhandledMockedApiRoute(context.route, context.method, context.path);
}

function createRouteContext(route: Route): EdgeFunctionRouteContext {
  const request = route.request();
  const url = new URL(request.url());
  return {
    route,
    request,
    method: request.method(),
    path: url.pathname,
    url,
  };
}

/**
 * TODO: Document handleEdgeFunctionCollectionRoute.
 */
async function handleEdgeFunctionCollectionRoute(
  context: EdgeFunctionRouteContext,
  options: EdgeFunctionMockOptions | undefined,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  if (context.path !== "/api/admin/functions") {
    return false;
  }

  if (context.method === "GET") {
    state.listCalls += 1;
    const response = options?.listResponder?.();
    await json(context.route, response?.status ?? 200, response?.body ?? store.edgeFunctions);
    return true;
  }

  if (context.method === "POST") {
    state.deployCalls += 1;
    const body = context.request.postDataJSON() as Record<string, unknown>;
    state.lastDeployBody = body;

    if (options?.deployResponder) {
      const response = options.deployResponder(body);
      await json(context.route, response.status, response.body);
      return true;
    }

    const created = createEdgeFunctionRecord(body, store.nextFunctionNumber);
    store.nextFunctionNumber += 1;
    store.edgeFunctions.unshift(created);
    await json(context.route, 201, created);
    return true;
  }

  return false;
}

/**
 * TODO: Document createEdgeFunctionRecord.
 */
function createEdgeFunctionRecord(
  body: Record<string, unknown>,
  nextFunctionNumber: number,
): EdgeFunctionRecord {
  return {
    id: `ef-${String(nextFunctionNumber).padStart(3, "0")}`,
    name: String(body.name ?? ""),
    entryPoint: String(body.entry_point ?? "handler"),
    timeout: Number(body.timeout_ms ?? 5000) * 1_000_000,
    public: body.public ?? true,
    source: String(body.source ?? ""),
    compiledJs: "",
    lastInvokedAt: null,
    envVars: (body.env_vars as Record<string, string> | undefined) ?? {},
    createdAt: "2026-02-24T00:00:00Z",
    updatedAt: "2026-02-24T00:00:00Z",
  };
}

async function handleEdgeFunctionDetailRoute(
  context: EdgeFunctionRouteContext,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const detailMatch = context.path.match(/^\/api\/admin\/functions\/([^/]+)$/);
  if (context.method !== "GET" || !detailMatch) {
    return false;
  }

  const fn = store.edgeFunctions.find((item) => item.id === detailMatch[1]);
  await json(context.route, fn ? 200 : 404, fn ?? { message: "not found" });
  return true;
}

/**
 * TODO: Document handleEdgeFunctionUpdateRoute.
 */
async function handleEdgeFunctionUpdateRoute(
  context: EdgeFunctionRouteContext,
  options: EdgeFunctionMockOptions | undefined,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const updateMatch = context.path.match(/^\/api\/admin\/functions\/([^/]+)$/);
  if (context.method !== "PUT" || !updateMatch) {
    return false;
  }

  state.updateCalls += 1;
  const body = context.request.postDataJSON() as Record<string, unknown>;
  state.lastUpdateBody = body;

  if (options?.updateResponder) {
    const response = options.updateResponder(updateMatch[1], body);
    await json(context.route, response.status, response.body);
    return true;
  }

  const existingIndex = store.edgeFunctions.findIndex((fn) => fn.id === updateMatch[1]);
  if (existingIndex < 0) {
    await json(context.route, 404, { message: "not found" });
    return true;
  }

  const updated = buildUpdatedEdgeFunction(store.edgeFunctions[existingIndex], body);
  store.edgeFunctions[existingIndex] = updated;
  await json(context.route, 200, updated);
  return true;
}

/**
 * TODO: Document buildUpdatedEdgeFunction.
 */
function buildUpdatedEdgeFunction(
  existing: EdgeFunctionRecord,
  body: Record<string, unknown>,
): EdgeFunctionRecord {
  return {
    ...existing,
    source: typeof body.source === "string" ? body.source : existing.source,
    entryPoint:
      typeof body.entry_point === "string" ? body.entry_point : existing.entryPoint,
    timeout:
      Number(body.timeout_ms ?? Math.round(existing.timeout / 1_000_000)) * 1_000_000,
    public: typeof body.public === "boolean" ? body.public : existing.public,
    envVars: (body.env_vars as Record<string, string> | undefined) ?? existing.envVars,
    updatedAt: "2026-02-24T00:00:00Z",
  };
}

/**
 * TODO: Document handleEdgeFunctionDeleteRoute.
 */
async function handleEdgeFunctionDeleteRoute(
  context: EdgeFunctionRouteContext,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const deleteMatch = context.path.match(/^\/api\/admin\/functions\/([^/]+)$/);
  if (context.method !== "DELETE" || !deleteMatch) {
    return false;
  }

  state.deleteCalls += 1;
  state.lastDeletedId = deleteMatch[1];

  const existingIndex = store.edgeFunctions.findIndex((fn) => fn.id === deleteMatch[1]);
  if (existingIndex < 0) {
    await json(context.route, 404, { message: "not found" });
    return true;
  }

  store.edgeFunctions.splice(existingIndex, 1);
  removeFunctionArtifacts(deleteMatch[1], store);
  await json(context.route, 204, null);
  return true;
}

function removeFunctionArtifacts(functionId: string, store: EdgeFunctionMockStore): void {
  removeMatchingItems(store.edgeFunctionLogs, (log) => log.functionId === functionId);
  removeMatchingItems(store.dbTriggers, (trigger) => trigger.functionId === functionId);
  removeMatchingItems(store.cronTriggers, (trigger) => trigger.functionId === functionId);
  removeMatchingItems(store.storageTriggers, (trigger) => trigger.functionId === functionId);
}

function removeMatchingItems<T>(items: T[], predicate: (item: T) => boolean): void {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (predicate(items[index])) {
      items.splice(index, 1);
    }
  }
}

/**
 * TODO: Document handleEdgeFunctionLogsRoute.
 */
async function handleEdgeFunctionLogsRoute(
  context: EdgeFunctionRouteContext,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const logsMatch = context.path.match(/^\/api\/admin\/functions\/([^/]+)\/logs$/);
  if (context.method !== "GET" || !logsMatch) {
    return false;
  }

  state.logCalls += 1;
  let functionLogs = store.edgeFunctionLogs.filter((log) => log.functionId === logsMatch[1]);
  const statusFilter = context.url.searchParams.get("status");
  if (statusFilter) {
    functionLogs = functionLogs.filter((log) => log.status === statusFilter);
  }
  const triggerFilter = context.url.searchParams.get("trigger_type");
  if (triggerFilter) {
    functionLogs = functionLogs.filter((log) => log.triggerType === triggerFilter);
  }

  await json(context.route, 200, functionLogs);
  return true;
}

/**
 * TODO: Document handleDBTriggerRoute.
 */
async function handleDBTriggerRoute(
  context: EdgeFunctionRouteContext,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const collectionMatch = context.path.match(/^\/api\/admin\/functions\/([^/]+)\/triggers\/db$/);
  if (collectionMatch && context.method === "GET") {
    state.dbListCalls += 1;
    await json(
      context.route,
      200,
      store.dbTriggers.filter((trigger) => trigger.functionId === collectionMatch[1]),
    );
    return true;
  }

  if (collectionMatch && context.method === "POST") {
    state.dbCreateCalls += 1;
    const body = context.request.postDataJSON() as Record<string, unknown>;
    state.lastDBCreateBody = body;
    const created = createDBTriggerRecord(collectionMatch[1], body, store.nextDBTriggerNumber);
    store.nextDBTriggerNumber += 1;
    state.lastCreatedDBTriggerId = created.id;
    store.dbTriggers.unshift(created);
    await json(context.route, 201, created);
    return true;
  }

  const toggleMatch = context.path.match(
    /^\/api\/admin\/functions\/([^/]+)\/triggers\/db\/([^/]+)\/(enable|disable)$/,
  );
  if (toggleMatch && context.method === "POST") {
    return handleDBTriggerToggle(context.route, toggleMatch, state, store);
  }

  const deleteMatch = context.path.match(/^\/api\/admin\/functions\/([^/]+)\/triggers\/db\/([^/]+)$/);
  if (deleteMatch && context.method === "DELETE") {
    return handleDBTriggerDelete(context.route, deleteMatch, state, store);
  }

  return false;
}

/**
 * TODO: Document createDBTriggerRecord.
 */
function createDBTriggerRecord(
  functionId: string,
  body: Record<string, unknown>,
  nextTriggerNumber: number,
): DBTriggerRecord {
  const createdAt = new Date().toISOString();
  return {
    id: `dbt-${String(nextTriggerNumber).padStart(3, "0")}`,
    functionId,
    tableName: typeof body.table_name === "string" ? body.table_name : "",
    schema: typeof body.schema === "string" && body.schema.trim() !== "" ? body.schema : "public",
    events: Array.isArray(body.events) ? body.events.map((event) => String(event)) : [],
    filterColumns: Array.isArray(body.filter_columns)
      ? body.filter_columns.map((column) => String(column))
      : [],
    enabled: true,
    createdAt,
    updatedAt: createdAt,
  };
}

/**
 * TODO: Document handleDBTriggerToggle.
 */
async function handleDBTriggerToggle(
  route: Route,
  toggleMatch: RegExpMatchArray,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const trigger = store.dbTriggers.find(
    (item) => item.functionId === toggleMatch[1] && item.id === toggleMatch[2],
  );
  if (!trigger) {
    await json(route, 404, { message: "not found" });
    return true;
  }

  const enabled = toggleMatch[3] === "enable";
  if (enabled) {
    state.dbEnableCalls += 1;
    state.lastDBEnabledId = trigger.id;
  } else {
    state.dbDisableCalls += 1;
    state.lastDBDisabledId = trigger.id;
  }

  trigger.enabled = enabled;
  trigger.updatedAt = new Date().toISOString();
  await json(route, 200, trigger);
  return true;
}

/**
 * TODO: Document handleDBTriggerDelete.
 */
async function handleDBTriggerDelete(
  route: Route,
  deleteMatch: RegExpMatchArray,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  state.dbDeleteCalls += 1;
  state.lastDBDeletedId = deleteMatch[2];
  const triggerIndex = store.dbTriggers.findIndex(
    (item) => item.functionId === deleteMatch[1] && item.id === deleteMatch[2],
  );
  if (triggerIndex < 0) {
    await json(route, 404, { message: "not found" });
    return true;
  }

  store.dbTriggers.splice(triggerIndex, 1);
  await json(route, 204, null);
  return true;
}

/**
 * TODO: Document handleCronTriggerRoute.
 */
async function handleCronTriggerRoute(
  context: EdgeFunctionRouteContext,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const collectionMatch = context.path.match(/^\/api\/admin\/functions\/([^/]+)\/triggers\/cron$/);
  if (collectionMatch && context.method === "GET") {
    state.cronListCalls += 1;
    await json(
      context.route,
      200,
      store.cronTriggers.filter((trigger) => trigger.functionId === collectionMatch[1]),
    );
    return true;
  }

  if (collectionMatch && context.method === "POST") {
    state.cronCreateCalls += 1;
    const body = context.request.postDataJSON() as Record<string, unknown>;
    state.lastCronCreateBody = body;
    const created = createCronTriggerRecord(collectionMatch[1], body, store.nextCronTriggerNumber);
    store.nextCronTriggerNumber += 1;
    state.lastCreatedCronTriggerId = created.id;
    store.cronTriggers.unshift(created);
    await json(context.route, 201, created);
    return true;
  }

  const toggleMatch = context.path.match(
    /^\/api\/admin\/functions\/([^/]+)\/triggers\/cron\/([^/]+)\/(enable|disable)$/,
  );
  if (toggleMatch && context.method === "POST") {
    return handleCronTriggerToggle(context.route, toggleMatch, state, store);
  }

  const runMatch = context.path.match(
    /^\/api\/admin\/functions\/([^/]+)\/triggers\/cron\/([^/]+)\/run$/,
  );
  if (runMatch && context.method === "POST") {
    return handleCronTriggerManualRun(context.route, runMatch, state, store);
  }

  const deleteMatch = context.path.match(/^\/api\/admin\/functions\/([^/]+)\/triggers\/cron\/([^/]+)$/);
  if (deleteMatch && context.method === "DELETE") {
    return handleCronTriggerDelete(context.route, deleteMatch, state, store);
  }

  return false;
}

/**
 * TODO: Document createCronTriggerRecord.
 */
function createCronTriggerRecord(
  functionId: string,
  body: Record<string, unknown>,
  nextTriggerNumber: number,
): CronTriggerRecord {
  const createdAt = new Date().toISOString();
  return {
    id: `ct-${String(nextTriggerNumber).padStart(3, "0")}`,
    functionId,
    scheduleId: `sched-${String(nextTriggerNumber).padStart(3, "0")}`,
    cronExpr: typeof body.cron_expr === "string" ? body.cron_expr : "",
    timezone: typeof body.timezone === "string" && body.timezone.trim() !== "" ? body.timezone : "UTC",
    payload: body.payload ?? {},
    enabled: true,
    createdAt,
    updatedAt: createdAt,
  };
}

/**
 * TODO: Document handleCronTriggerToggle.
 */
async function handleCronTriggerToggle(
  route: Route,
  toggleMatch: RegExpMatchArray,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const trigger = store.cronTriggers.find(
    (item) => item.functionId === toggleMatch[1] && item.id === toggleMatch[2],
  );
  if (!trigger) {
    await json(route, 404, { message: "not found" });
    return true;
  }

  const enabled = toggleMatch[3] === "enable";
  if (enabled) {
    state.cronEnableCalls += 1;
    state.lastCronEnabledId = trigger.id;
  } else {
    state.cronDisableCalls += 1;
    state.lastCronDisabledId = trigger.id;
  }

  trigger.enabled = enabled;
  trigger.updatedAt = new Date().toISOString();
  await json(route, 200, trigger);
  return true;
}

/**
 * TODO: Document handleCronTriggerManualRun.
 */
async function handleCronTriggerManualRun(
  route: Route,
  runMatch: RegExpMatchArray,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const trigger = store.cronTriggers.find(
    (item) => item.functionId === runMatch[1] && item.id === runMatch[2],
  );
  if (!trigger) {
    await json(route, 404, { message: "not found" });
    return true;
  }

  state.cronManualRunCalls += 1;
  state.lastCronManualRunId = trigger.id;
  await json(route, 200, { statusCode: 200, body: "manual run ok" });
  return true;
}

/**
 * TODO: Document handleCronTriggerDelete.
 */
async function handleCronTriggerDelete(
  route: Route,
  deleteMatch: RegExpMatchArray,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  state.cronDeleteCalls += 1;
  state.lastCronDeletedId = deleteMatch[2];
  const triggerIndex = store.cronTriggers.findIndex(
    (item) => item.functionId === deleteMatch[1] && item.id === deleteMatch[2],
  );
  if (triggerIndex < 0) {
    await json(route, 404, { message: "not found" });
    return true;
  }

  store.cronTriggers.splice(triggerIndex, 1);
  await json(route, 204, null);
  return true;
}

/**
 * TODO: Document handleStorageTriggerRoute.
 */
async function handleStorageTriggerRoute(
  context: EdgeFunctionRouteContext,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const collectionMatch = context.path.match(/^\/api\/admin\/functions\/([^/]+)\/triggers\/storage$/);
  if (collectionMatch && context.method === "GET") {
    state.storageListCalls += 1;
    await json(
      context.route,
      200,
      store.storageTriggers.filter((trigger) => trigger.functionId === collectionMatch[1]),
    );
    return true;
  }

  if (collectionMatch && context.method === "POST") {
    state.storageCreateCalls += 1;
    const body = context.request.postDataJSON() as Record<string, unknown>;
    state.lastStorageCreateBody = body;
    const created = createStorageTriggerRecord(
      collectionMatch[1],
      body,
      store.nextStorageTriggerNumber,
    );
    store.nextStorageTriggerNumber += 1;
    state.lastCreatedStorageTriggerId = created.id;
    store.storageTriggers.unshift(created);
    await json(context.route, 201, created);
    return true;
  }

  const toggleMatch = context.path.match(
    /^\/api\/admin\/functions\/([^/]+)\/triggers\/storage\/([^/]+)\/(enable|disable)$/,
  );
  if (toggleMatch && context.method === "POST") {
    return handleStorageTriggerToggle(context.route, toggleMatch, state, store);
  }

  const deleteMatch = context.path.match(
    /^\/api\/admin\/functions\/([^/]+)\/triggers\/storage\/([^/]+)$/,
  );
  if (deleteMatch && context.method === "DELETE") {
    return handleStorageTriggerDelete(context.route, deleteMatch, state, store);
  }

  return false;
}

/**
 * TODO: Document createStorageTriggerRecord.
 */
function createStorageTriggerRecord(
  functionId: string,
  body: Record<string, unknown>,
  nextTriggerNumber: number,
): StorageTriggerRecord {
  const createdAt = new Date().toISOString();
  return {
    id: `st-${String(nextTriggerNumber).padStart(3, "0")}`,
    functionId,
    bucket: typeof body.bucket === "string" ? body.bucket : "",
    eventTypes: Array.isArray(body.event_types)
      ? body.event_types.map((event) => String(event))
      : [],
    prefixFilter: typeof body.prefix_filter === "string" ? body.prefix_filter : "",
    suffixFilter: typeof body.suffix_filter === "string" ? body.suffix_filter : "",
    enabled: true,
    createdAt,
    updatedAt: createdAt,
  };
}

/**
 * TODO: Document handleStorageTriggerToggle.
 */
async function handleStorageTriggerToggle(
  route: Route,
  toggleMatch: RegExpMatchArray,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const trigger = store.storageTriggers.find(
    (item) => item.functionId === toggleMatch[1] && item.id === toggleMatch[2],
  );
  if (!trigger) {
    await json(route, 404, { message: "not found" });
    return true;
  }

  const enabled = toggleMatch[3] === "enable";
  if (enabled) {
    state.storageEnableCalls += 1;
    state.lastStorageEnabledId = trigger.id;
  } else {
    state.storageDisableCalls += 1;
    state.lastStorageDisabledId = trigger.id;
  }

  trigger.enabled = enabled;
  trigger.updatedAt = new Date().toISOString();
  await json(route, 200, trigger);
  return true;
}

/**
 * TODO: Document handleStorageTriggerDelete.
 */
async function handleStorageTriggerDelete(
  route: Route,
  deleteMatch: RegExpMatchArray,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  state.storageDeleteCalls += 1;
  state.lastStorageDeletedId = deleteMatch[2];
  const triggerIndex = store.storageTriggers.findIndex(
    (item) => item.functionId === deleteMatch[1] && item.id === deleteMatch[2],
  );
  if (triggerIndex < 0) {
    await json(route, 404, { message: "not found" });
    return true;
  }

  store.storageTriggers.splice(triggerIndex, 1);
  await json(route, 204, null);
  return true;
}

/**
 * TODO: Document handleInvokeRoute.
 */
async function handleInvokeRoute(
  context: EdgeFunctionRouteContext,
  options: EdgeFunctionMockOptions | undefined,
  state: EdgeFunctionMockState,
  store: EdgeFunctionMockStore,
): Promise<boolean> {
  const invokeMatch = context.path.match(/^\/api\/admin\/functions\/([^/]+)\/invoke$/);
  if (context.method !== "POST" || !invokeMatch) {
    return false;
  }

  state.invokeCalls += 1;
  const body = context.request.postDataJSON() as Record<string, unknown>;
  state.lastInvokeBody = body;

  if (options?.invokeResponder) {
    const response = options.invokeResponder(invokeMatch[1], body);
    await json(context.route, response.status, response.body);
    return true;
  }

  const fn = store.edgeFunctions.find((item) => item.id === invokeMatch[1]);
  if (!fn) {
    await json(context.route, 404, { message: "not found" });
    return true;
  }

  const createdAt = new Date().toISOString();
  store.edgeFunctionLogs.unshift(createInvokeLog(fn, body, createdAt, store.nextLogNumber));
  store.nextLogNumber += 1;
  fn.lastInvokedAt = createdAt;
  await json(context.route, 200, {
    statusCode: 200,
    headers: { "Content-Type": ["application/json"] },
    body: '{"message":"Hello from mock!"}',
  });
  return true;
}

/**
 * TODO: Document createInvokeLog.
 */
function createInvokeLog(
  fn: EdgeFunctionRecord,
  body: Record<string, unknown>,
  createdAt: string,
  nextLogNumber: number,
): EdgeFunctionLogRecord {
  return {
    id: `log-${String(nextLogNumber).padStart(3, "0")}`,
    functionId: fn.id,
    invocationId: `inv-${Date.now()}`,
    status: "success",
    durationMs: 42,
    stdout: "console output here",
    error: null,
    requestMethod: typeof body.method === "string" ? body.method : "GET",
    requestPath: typeof body.path === "string" ? body.path : `/${fn.name}`,
    triggerType: "http",
    triggerId: null,
    parentInvocationId: null,
    createdAt,
  };
}
