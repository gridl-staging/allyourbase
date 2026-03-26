import type { TestInfo } from "@playwright/test";
import {
  cleanupAuthUser,
  createTableViaSQLEditor,
  createLinkedEmailAuthSessionToken,
  ensureAuthSettings,
  execSQL,
  expect,
  fetchAuthSettings,
  seedRecord,
  startSSECapture,
  test,
} from "../fixtures";
import type { SSECaptureHandle } from "../fixtures";

interface CleanupState {
  capture?: SSECaptureHandle;
  email?: string;
  tableName?: string;
  anonymousAuthEnabled?: boolean;
}

function extractCreateEventShape(
  event: Record<string, unknown> | undefined,
): { action: string | null; table: string | null; name: string | null } | null {
  if (!event) {
    return null;
  }
  const record = event["record"];
  let name: string | null = null;
  if (record && typeof record === "object" && !Array.isArray(record)) {
    const candidate = (record as Record<string, unknown>)["name"];
    if (typeof candidate === "string") {
      name = candidate;
    }
  }
  return {
    action: typeof event["action"] === "string" ? event["action"] : null,
    table: typeof event["table"] === "string" ? event["table"] : null,
    name,
  };
}

test.describe("Dashboard Client Realtime Journey (Full E2E)", () => {
  const cleanupByTestID = new Map<string, CleanupState>();

  test.afterEach(async ({ request, adminToken }, testInfo: TestInfo) => {
    const cleanup = cleanupByTestID.get(testInfo.testId);
    if (!cleanup) {
      return;
    }

    // Keep teardown deterministic: close SSE subscription before dropping data.
    if (cleanup.capture) {
      await cleanup.capture.close().catch(() => {});
    }
    if (cleanup.email) {
      await cleanupAuthUser(request, adminToken, cleanup.email).catch(() => {});
    }
    if (cleanup.tableName) {
      await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${cleanup.tableName}`).catch(
        () => {},
      );
    }
    if (typeof cleanup.anonymousAuthEnabled === "boolean") {
      await ensureAuthSettings(request, adminToken, {
        anonymous_auth_enabled: cleanup.anonymousAuthEnabled,
      }).catch(() => {});
    }
    cleanupByTestID.delete(testInfo.testId);
  });

  test("dashboard SQL table + client insert emits realtime create event", async (
    { page, request, adminToken },
    testInfo: TestInfo,
  ) => {
    const runID = `${Date.now()}_${testInfo.parallelIndex}_${testInfo.repeatEachIndex}_${testInfo.retry}`;
    const tableName = `dashboard_rt_${runID}`;
    const email = `dashboard-rt-${runID}@example.com`;
    const password = `TestPass!${runID}`;

    const originalAuthSettings = await fetchAuthSettings(request, adminToken);
    cleanupByTestID.set(testInfo.testId, {
      email,
      tableName,
      anonymousAuthEnabled: originalAuthSettings.anonymous_auth_enabled,
    });

    // Linked email signup depends on the anonymous session bootstrap route.
    await ensureAuthSettings(request, adminToken, {
      anonymous_auth_enabled: true,
    });

    await createTableViaSQLEditor(page, tableName);

    const clientToken = await createLinkedEmailAuthSessionToken(request, email, password);
    const baseURL = new URL(page.url()).origin;
    const capture = await startSSECapture(page, baseURL, clientToken, [tableName]);
    const existingCleanup = cleanupByTestID.get(testInfo.testId);
    cleanupByTestID.set(testInfo.testId, { ...existingCleanup, capture });

    await seedRecord(request, clientToken, tableName, { name: "hello" });

    await expect
      .poll(
        async () => {
          const events = await capture.getEvents();
          return extractCreateEventShape(events[0]);
        },
        { timeout: 10000 },
      )
      .toEqual({
        action: "create",
        table: tableName,
        name: "hello",
      });
  });
});
