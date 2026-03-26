/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-unmocked/fixtures/jobs.ts.
 */
import type { APIRequestContext } from "@playwright/test";
import { execSQL, sqlLiteral, validateResponse } from "./core";

/**
 * TODO: Document seedJob.
 */
export async function seedJob(
  request: APIRequestContext,
  token: string,
  options: {
    type: string;
    state?: "queued" | "running" | "completed" | "failed" | "canceled";
    payload?: Record<string, unknown>;
    attempts?: number;
    maxAttempts?: number;
    lastError?: string;
  },
): Promise<{ id: string; type: string; state: string }> {
  const state = options.state || "failed";
  const payload = JSON.stringify(options.payload || { source: "browser-smoke" });
  const attempts = options.attempts ?? 1;
  const maxAttempts = options.maxAttempts ?? 3;
  const lastErrorSQL = options.lastError ? `'${sqlLiteral(options.lastError)}'` : "NULL";
  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_jobs (type, payload, state, attempts, max_attempts, last_error)
     VALUES ('${sqlLiteral(options.type)}', '${sqlLiteral(payload)}'::jsonb, '${state}', ${attempts}, ${maxAttempts}, ${lastErrorSQL})
     RETURNING id, type, state`,
  );
  const id = result.rows[0]?.[0];
  const type = result.rows[0]?.[1];
  const returnedState = result.rows[0]?.[2];
  if (typeof id !== "string" || typeof type !== "string" || typeof returnedState !== "string") {
    throw new Error(`Expected seeded job id/type/state for ${options.type}`);
  }
  return { id, type, state: returnedState };
}

export async function cleanupJobsByType(
  request: APIRequestContext,
  token: string,
  type: string,
): Promise<void> {
  await execSQL(request, token, `DELETE FROM _ayb_jobs WHERE type = '${sqlLiteral(type)}'`);
}

/**
 * TODO: Document seedSchedule.
 */
export async function seedSchedule(
  request: APIRequestContext,
  token: string,
  options: {
    name: string;
    jobType: string;
    cronExpr?: string;
    timezone?: string;
    payload?: Record<string, unknown>;
    enabled?: boolean;
    maxAttempts?: number;
  },
): Promise<{ id: string; name: string; jobType: string; cronExpr: string; timezone: string }> {
  const res = await request.post("/api/admin/schedules", {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: {
      name: options.name,
      jobType: options.jobType,
      cronExpr: options.cronExpr || "*/15 * * * *",
      timezone: options.timezone || "UTC",
      payload: options.payload || { source: "browser-smoke" },
      enabled: options.enabled ?? true,
      maxAttempts: options.maxAttempts ?? 3,
    },
  });
  await validateResponse(res, `Create schedule ${options.name}`);
  const body = await res.json();
  const id = body?.id;
  const name = body?.name;
  const jobType = body?.jobType;
  const cronExpr = body?.cronExpr;
  const timezone = body?.timezone;
  if (
    typeof id !== "string" ||
    typeof name !== "string" ||
    typeof jobType !== "string" ||
    typeof cronExpr !== "string" ||
    typeof timezone !== "string"
  ) {
    throw new Error(
      `Expected seeded schedule id/name/jobType/cronExpr/timezone for ${options.name}`,
    );
  }
  return { id, name, jobType, cronExpr, timezone };
}

export async function cleanupScheduleByID(
  request: APIRequestContext,
  token: string,
  scheduleID: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/schedules/${scheduleID}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete schedule ${scheduleID}`);
  }
}
