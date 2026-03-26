/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_3_webhook_and_trigger_proof/allyourbase_dev/ui/browser-tests-unmocked/fixtures/edge-functions.ts.
 */
import type { APIRequestContext, APIResponse } from "@playwright/test";
import { validateResponse } from "./core";

function adminAuthHeaders(token: string): Record<string, string> {
  return { Authorization: `Bearer ${token}` };
}

function adminJSONHeaders(token: string): Record<string, string> {
  return {
    ...adminAuthHeaders(token),
    "Content-Type": "application/json",
  };
}

async function deleteAdminResource(
  request: APIRequestContext,
  token: string,
  path: string,
  description: string,
): Promise<void> {
  const res = await request.delete(path, {
    headers: adminAuthHeaders(token),
  });
  if (res.status() !== 404) {
    await validateResponse(res, description);
  }
}

/**
 * TODO: Document seedEdgeFunction.
 */
export async function seedEdgeFunction(
  request: APIRequestContext,
  token: string,
  overrides: {
    name?: string;
    source?: string;
    entry_point?: string;
    timeout_ms?: number;
    public?: boolean;
    env_vars?: Record<string, string>;
  } = {},
): Promise<{ id: string; name: string }> {
  const name = overrides.name || `test-fn-${Date.now()}`;
  const source =
    overrides.source ||
    `export default function handler(req) {
  return {
    statusCode: 200,
    body: JSON.stringify({ message: "Hello from ${name}" }),
    headers: { "Content-Type": "application/json" },
  };
}`;
  const res = await request.post("/api/admin/functions", {
    headers: adminJSONHeaders(token),
    data: {
      name,
      source,
      entry_point: overrides.entry_point || "handler",
      timeout_ms: overrides.timeout_ms || 5000,
      public: overrides.public ?? false,
      env_vars: overrides.env_vars || {},
    },
  });
  await validateResponse(res, `Deploy edge function ${name}`);
  const body = await res.json();
  return { id: body.id, name: body.name };
}

export async function deleteEdgeFunction(
  request: APIRequestContext,
  token: string,
  id: string,
): Promise<void> {
  await deleteAdminResource(request, token, `/api/admin/functions/${id}`, `Delete edge function ${id}`);
}

/**
 * TODO: Document getEdgeFunctionIDByName.
 */
export async function getEdgeFunctionIDByName(
  request: APIRequestContext,
  token: string,
  functionName: string,
): Promise<string> {
  const res = await request.get(`/api/admin/functions/by-name/${encodeURIComponent(functionName)}`, {
    headers: adminAuthHeaders(token),
  });
  await validateResponse(res, `Resolve edge function ${functionName} by name`);
  const body = await res.json();
  const functionID = body?.id;
  if (typeof functionID !== "string" || functionID.length === 0) {
    throw new Error(`Expected function id for ${functionName}`);
  }
  return functionID;
}

/**
 * Invokes the shipped public edge-function route using GET.
 */
export async function invokePublicEdgeFunctionGET(
  request: APIRequestContext,
  functionName: string,
  bearerToken?: string,
): Promise<APIResponse> {
  const headers =
    typeof bearerToken === "string" && bearerToken.length > 0
      ? { Authorization: `Bearer ${bearerToken}` }
      : undefined;
  return request.get(`/functions/v1/${encodeURIComponent(functionName)}`, headers ? { headers } : undefined);
}

export async function createDBTrigger(
  request: APIRequestContext,
  token: string,
  functionId: string,
  data: { table_name: string; schema?: string; events: string[]; filter_columns?: string[] },
): Promise<{ id: string }> {
  const res = await request.post(`/api/admin/functions/${functionId}/triggers/db`, {
    headers: adminJSONHeaders(token),
    data: { schema: "public", ...data },
  });
  await validateResponse(res, `Create DB trigger for function ${functionId}`);
  const body = await res.json();
  return { id: body.id };
}

/**
 * TODO: Document deleteDBTrigger.
 */
export async function deleteDBTrigger(
  request: APIRequestContext,
  token: string,
  functionId: string,
  triggerId: string,
): Promise<void> {
  await deleteAdminResource(
    request,
    token,
    `/api/admin/functions/${functionId}/triggers/db/${triggerId}`,
    `Delete DB trigger ${triggerId}`,
  );
}

export async function createCronTrigger(
  request: APIRequestContext,
  token: string,
  functionId: string,
  data: { cron_expr: string; timezone?: string; payload?: Record<string, unknown> },
): Promise<{ id: string }> {
  const res = await request.post(`/api/admin/functions/${functionId}/triggers/cron`, {
    headers: adminJSONHeaders(token),
    data: { timezone: "UTC", ...data },
  });
  await validateResponse(res, `Create cron trigger for function ${functionId}`);
  const body = await res.json();
  return { id: body.id };
}

/**
 * TODO: Document deleteCronTrigger.
 */
export async function deleteCronTrigger(
  request: APIRequestContext,
  token: string,
  functionId: string,
  triggerId: string,
): Promise<void> {
  await deleteAdminResource(
    request,
    token,
    `/api/admin/functions/${functionId}/triggers/cron/${triggerId}`,
    `Delete cron trigger ${triggerId}`,
  );
}

export async function manualRunCronTrigger(
  request: APIRequestContext,
  token: string,
  functionId: string,
  triggerId: string,
): Promise<{ statusCode: number; body: string }> {
  const res = await request.post(
    `/api/admin/functions/${functionId}/triggers/cron/${triggerId}/run`,
    {
      headers: adminAuthHeaders(token),
    },
  );
  await validateResponse(res, `Manual run cron trigger ${triggerId}`);
  return res.json();
}

/**
 * TODO: Document createStorageTrigger.
 */
export async function createStorageTrigger(
  request: APIRequestContext,
  token: string,
  functionId: string,
  data: {
    bucket: string;
    event_types: string[];
    prefix_filter?: string;
    suffix_filter?: string;
  },
): Promise<{ id: string }> {
  const res = await request.post(`/api/admin/functions/${functionId}/triggers/storage`, {
    headers: adminJSONHeaders(token),
    data,
  });
  await validateResponse(res, `Create storage trigger for function ${functionId}`);
  const body = await res.json();
  return { id: body.id };
}

/**
 * TODO: Document deleteStorageTrigger.
 */
export async function deleteStorageTrigger(
  request: APIRequestContext,
  token: string,
  functionId: string,
  triggerId: string,
): Promise<void> {
  await deleteAdminResource(
    request,
    token,
    `/api/admin/functions/${functionId}/triggers/storage/${triggerId}`,
    `Delete storage trigger ${triggerId}`,
  );
}

/**
 * TODO: Document waitForFunctionLog.
 */
type FunctionLogEntry = {
  requestMethod?: string;
  requestPath?: string;
  status: string;
  stdout?: string;
  triggerType?: string;
  triggerId?: string;
  parentInvocationId?: string;
  createdAt?: string | number | Date;
};

/**
 * TODO: Document waitForFunctionLog.
 */
export async function waitForFunctionLog(
  request: APIRequestContext,
  token: string,
  functionId: string,
  predicate: (log: FunctionLogEntry) => boolean,
  timeoutOrOptions:
    | number
    | {
        timeoutMs?: number;
        pollIntervalMs?: number;
        minCreatedAt?: number | string | Date;
      } = 15000,
): Promise<FunctionLogEntry> {
  const toEpochMillis = (value: number | string | Date | undefined): number | null => {
    if (typeof value === "number") {
      return Number.isFinite(value) ? value : null;
    }
    if (value instanceof Date) {
      const time = value.getTime();
      return Number.isFinite(time) ? time : null;
    }
    if (typeof value === "string") {
      const parsed = Date.parse(value);
      return Number.isFinite(parsed) ? parsed : null;
    }
    return null;
  };

  const timeoutMs =
    typeof timeoutOrOptions === "number" ? timeoutOrOptions : (timeoutOrOptions.timeoutMs ?? 15000);
  const pollIntervalMsRaw =
    typeof timeoutOrOptions === "number" ? 500 : (timeoutOrOptions.pollIntervalMs ?? 500);
  const pollIntervalMs = Math.max(0, pollIntervalMsRaw);
  const minCreatedAtMs =
    typeof timeoutOrOptions === "number" ? null : toEpochMillis(timeoutOrOptions.minCreatedAt);

  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const res = await request.get(`/api/admin/functions/${functionId}/logs`, {
      headers: adminAuthHeaders(token),
    });
    if (!res.ok()) {
      let body = "";
      try {
        body = (await res.text()).trim();
      } catch {
        // Ignore body parse errors; status still provides useful context.
      }
      const detail = body.length > 0 ? `: ${body}` : "";
      throw new Error(
        `Failed to fetch function logs for ${functionId}: status ${res.status()}${detail}`,
      );
    }

    const logs = await res.json();
    if (Array.isArray(logs)) {
      for (const rawLog of logs) {
        if (!rawLog || typeof rawLog !== "object") {
          continue;
        }
        const log = rawLog as FunctionLogEntry;

        if (minCreatedAtMs !== null) {
          const logCreatedAtMs = toEpochMillis(log.createdAt);
          if (logCreatedAtMs === null || logCreatedAtMs < minCreatedAtMs) {
            continue;
          }
        }

        if (predicate(log)) {
          return log;
        }
      }
    }
    await new Promise((resolve) => setTimeout(resolve, pollIntervalMs));
  }
  throw new Error(`No matching log entry found for function ${functionId} within ${timeoutMs}ms`);
}
