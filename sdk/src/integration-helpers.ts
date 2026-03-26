/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar22_03_sdk_e2e_real_server/allyourbase_dev/sdk/src/integration-helpers.ts.
 */
/**
 * Shared helpers, types, and constants for SDK integration tests.
 * All three integration suites (smoke/auth/records/storage, realtime, RPC)
 * import from this module — it is the single source of truth for admin auth,
 * SQL execution, user cleanup, and condition polling against a real AYB server.
 */
import { readFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect } from "vitest";
import { AYBClient } from "./client";
import { AYBError } from "./errors";
import type { AuthSession, AuthStateEvent, RealtimeEvent } from "./types";

// ── Types ────────────────────────────────────────────────────────────────────

export type AdminSQLResponse = {
  columns: string[];
  rows: unknown[][];
  rowCount: number;
  durationMs: number;
};

export type CapturedAuthEvent = {
  event: AuthStateEvent;
  session: AuthSession | null;
};

export type WaitForConditionOptions = {
  description: string;
  check: () => Promise<boolean>;
  timeoutMs?: number;
  intervalMs?: number;
};

export type RecordsFixture = {
  id: number;
  title: string;
  priority: number;
  created_at: string;
};

export type SeedInput = {
  title: string;
  priority: number;
};

// ── Constants ────────────────────────────────────────────────────────────────

export const BASE_URL =
  process.env.AYB_TEST_BASE_URL ||
  process.env.AYB_BASE_URL ||
  "http://localhost:8090";
export const ADMIN_TOKEN_PATH =
  process.env.AYB_ADMIN_TOKEN_PATH ||
  path.join(process.env.HOME || os.homedir(), ".ayb", "admin-token");
export const INTEGRATION_RUN_ID = new Date()
  .toISOString()
  .replace(/[-:.TZ]/g, "");
export const DEFAULT_WAIT_TIMEOUT_MS = 30_000;
export const DEFAULT_WAIT_INTERVAL_MS = 200;
export const SCHEMA_CACHE_TIMEOUT_MS = 30_000;
export const SCHEMA_CACHE_INTERVAL_MS = 250;
export const AUTH_TEST_PASSWORD = "password123";

// ── Module-level state ───────────────────────────────────────────────────────

export const trackedAuthUserIDs = new Set<string>();
let uniqueAuthEmailCounter = 0;
let cachedAdminToken: string | null = null;

/** Expose cached admin token for read access in tests. */
export function getCachedAdminToken(): string | null {
  return cachedAdminToken;
}

/** Set the cached admin token (used in beforeAll). */
export function setCachedAdminToken(token: string): void {
  cachedAdminToken = token;
}

// ── Helper functions ─────────────────────────────────────────────────────────

export function makeUniqueAuthEmail(testCaseName: string): string {
  uniqueAuthEmailCounter += 1;
  const normalizedTestCaseName = testCaseName
    .toLowerCase()
    .replace(/[^a-z0-9]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
  return `sdk-${INTEGRATION_RUN_ID}-${normalizedTestCaseName}-${uniqueAuthEmailCounter}@example.com`;
}

export function createTestClient(): AYBClient {
  return new AYBClient(BASE_URL);
}

export function trackAuthUser(userID: unknown): void {
  if (typeof userID === "string" && userID.trim() !== "") {
    trackedAuthUserIDs.add(userID);
  }
}

export async function primeIntegrationSuite(): Promise<void> {
  await waitForHealth();
  setCachedAdminToken(await getAdminToken());
}

export function captureAuthEvents(client: AYBClient): {
  events: CapturedAuthEvent[];
  unsubscribe: () => void;
} {
  const events: CapturedAuthEvent[] = [];
  const unsubscribe = client.onAuthStateChange((event, session) => {
    events.push({
      event,
      session: session
        ? { token: session.token, refreshToken: session.refreshToken }
        : null,
    });
  });
  return { events, unsubscribe };
}

export function toCount(value: unknown): number {
  if (typeof value === "number") {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  throw new Error(
    `Expected a numeric SQL count cell, received: ${String(value)}`,
  );
}

export function sqlStringLiteral(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

export function trackedUserIDSQLList(): string {
  if (trackedAuthUserIDs.size === 0) {
    throw new Error("Expected at least one tracked auth user before cleanup");
  }
  return Array.from(trackedAuthUserIDs).map(sqlStringLiteral).join(", ");
}

/**
 * Assert that an async operation throws an AYBError with the given HTTP status
 * and message. Fails the test if the operation succeeds or throws a non-AYB error.
 */
export async function expectAYBError(
  operation: () => Promise<unknown>,
  status: number,
  message: string,
): Promise<void> {
  try {
    await operation();
  } catch (error) {
    expect(error).toBeInstanceOf(AYBError);
    const aybError = error as AYBError;
    expect(aybError.status).toBe(status);
    expect(aybError.message).toBe(message);
    return;
  }
  throw new Error(
    `Expected AYBError(${status}, "${message}") but operation succeeded`,
  );
}

/**
 * TODO: Document getAdminToken.
 */
export async function getAdminToken(): Promise<string> {
  if (cachedAdminToken && cachedAdminToken.trim() !== "") {
    return cachedAdminToken;
  }

  const envAdminToken = (process.env.AYB_ADMIN_TOKEN || "").trim();
  if (envAdminToken !== "") {
    cachedAdminToken = envAdminToken;
    return cachedAdminToken;
  }

  const password = (process.env.AYB_ADMIN_PASSWORD || "").trim();
  if (password === "") {
    try {
      const fileAdminToken = readFileSync(ADMIN_TOKEN_PATH, "utf8").trim();
      if (fileAdminToken !== "") {
        cachedAdminToken = fileAdminToken;
        return cachedAdminToken;
      }
    } catch (error) {
      const fileReadError = error as NodeJS.ErrnoException;
      if (fileReadError.code !== "ENOENT") {
        throw error;
      }
    }
  }

  if (password === "") {
    throw new Error(
      `Admin credentials not configured. Set AYB_ADMIN_PASSWORD or AYB_ADMIN_TOKEN, or ensure ${ADMIN_TOKEN_PATH} exists.`,
    );
  }

  const response = await fetch(`${BASE_URL}/api/admin/auth`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ password }),
  });

  if (!response.ok) {
    const msg = await response.text().catch(() => "");
    throw new Error(
      `Failed to resolve admin token: ${response.status}${msg ? ` - ${msg}` : ""}`,
    );
  }

  const body = (await response.json()) as { token?: unknown };
  if (typeof body.token !== "string" || body.token.trim() === "") {
    throw new Error("Admin auth response missing token");
  }

  cachedAdminToken = body.token.trim();
  return cachedAdminToken;
}

/**
 * Execute a SQL query against the AYB admin SQL endpoint. Requires a valid
 * admin token (resolved via getAdminToken). Returns {columns, rows, rowCount, durationMs}.
 */
export async function adminSql(query: string): Promise<AdminSQLResponse> {
  const adminToken = await getAdminToken();
  const response = await fetch(`${BASE_URL}/api/admin/sql`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${adminToken}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ query }),
  });

  if (!response.ok) {
    const msg = await response.text().catch(() => "");
    throw new Error(
      `Admin SQL failed: ${response.status}${msg ? ` - ${msg}` : ""}`,
    );
  }

  return (await response.json()) as AdminSQLResponse;
}

export function sleep(delayMs: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, delayMs);
  });
}

export function expectRealtimeEventShape(events: RealtimeEvent[]): void {
  for (const event of events) {
    expect(Object.keys(event).sort()).toEqual(["action", "record", "table"]);
    expect(["create", "update", "delete"]).toContain(event.action);
  }
}

/**
 * TODO: Document waitForCollectionSchemaCache.
 */
export async function waitForCollectionSchemaCache(
  client: AYBClient,
  tableName: string,
  description: string,
): Promise<void> {
  await waitForCondition({
    description: `${description} schema cache for ${tableName}`,
    timeoutMs: SCHEMA_CACHE_TIMEOUT_MS,
    intervalMs: SCHEMA_CACHE_INTERVAL_MS,
    check: async () => {
      try {
        await client.records.list(tableName);
        return true;
      } catch (error) {
        if (
          error instanceof AYBError &&
          error.status === 404 &&
          error.message === `collection not found: ${tableName}`
        ) {
          return false;
        }
        throw error;
      }
    },
  });
}

export async function dropTableAndAssertRemoved(tableName: string): Promise<void> {
  await adminSql(`DROP TABLE IF EXISTS ${tableName} CASCADE`);
  const tableCount = await adminSql(
    `SELECT COUNT(*) AS count FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ${sqlStringLiteral(tableName)}`,
  );
  expect(toCount(tableCount.rows[0]?.[0])).toBe(0);
}

/**
 * Delete all auth users tracked during the test run from _ayb_users, then
 * verify deletion. Called in afterAll to prevent cross-run user accumulation.
 */
export async function cleanupTrackedAuthUsers(): Promise<void> {
  if (trackedAuthUserIDs.size === 0) {
    return;
  }

  const userIDSQLList = trackedUserIDSQLList();
  const existingUsers = await adminSql(
    `SELECT id FROM _ayb_users WHERE id IN (${userIDSQLList})`,
  );
  const existingUserIDs = new Set(
    existingUsers.rows.map((row) => String(row[0] ?? "")).filter(Boolean),
  );

  if (existingUserIDs.size > 0) {
    const deleteResult = await adminSql(
      `DELETE FROM _ayb_users WHERE id IN (${userIDSQLList}) RETURNING id`,
    );
    const deletedUserIDs = new Set(
      deleteResult.rows.map((row) => String(row[0] ?? "")).filter(Boolean),
    );
    expect(deletedUserIDs.size).toBe(existingUserIDs.size);
    for (const userID of existingUserIDs) {
      expect(deletedUserIDs.has(userID)).toBe(true);
    }
  }

  const remaining = await adminSql(
    `SELECT COUNT(*) AS count FROM _ayb_users WHERE id IN (${userIDSQLList})`,
  );
  expect(toCount(remaining.rows[0]?.[0])).toBe(0);
  trackedAuthUserIDs.clear();
}

/**
 * Poll an async check function until it returns true, or throw after timeoutMs.
 * Used to wait for schema cache propagation, health readiness, and event delivery.
 */
export async function waitForCondition({
  description,
  check,
  timeoutMs = DEFAULT_WAIT_TIMEOUT_MS,
  intervalMs = DEFAULT_WAIT_INTERVAL_MS,
}: WaitForConditionOptions): Promise<void> {
  const deadline = Date.now() + timeoutMs;

  while (Date.now() < deadline) {
    if (await check()) {
      return;
    }
    await sleep(intervalMs);
  }

  throw new Error(`Timed out waiting for ${description} after ${timeoutMs}ms`);
}

/**
 * Block until the AYB server's /health endpoint returns 200.
 * Called by primeIntegrationSuite() before any test runs.
 */
export async function waitForHealth(
  timeoutMs = DEFAULT_WAIT_TIMEOUT_MS,
): Promise<void> {
  await waitForCondition({
    description: `${BASE_URL}/health to return 200`,
    timeoutMs,
    check: async () => {
      try {
        const response = await fetch(`${BASE_URL}/health`);
        return response.status === 200;
      } catch {
        // Server may still be starting; retry until timeout.
        return false;
      }
    },
  });
}
