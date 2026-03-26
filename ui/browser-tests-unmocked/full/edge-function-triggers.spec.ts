import type { APIRequestContext, Locator, Page } from "@playwright/test";
import {
  test,
  expect,
  probeEndpoint,
  seedEdgeFunction,
  deleteEdgeFunction,
  execSQL,
  seedFile,
  deleteFile,
  waitForFunctionLog,
  waitForDashboard,
} from "../fixtures";

type TriggerKind = "db" | "cron" | "storage";
type TriggerPath = "/db-event" | "/cron" | "/storage";
type TriggerLogEntry = {
  requestMethod?: string;
  requestPath?: string;
  status: string;
  triggerType?: string;
  triggerId?: string;
  parentInvocationId?: string;
  createdAt?: string | number | Date;
  stdout?: string;
};
type TriggerRowControls = {
  triggerId: string;
  statusBadge: Locator;
  toggleButton: Locator;
  runButton: Locator;
};

const FUNCTION_LOG_TIMEOUT_MS = 20000;
const NO_LOG_TIMEOUT_MS = 4000;
const INVOCATION_MARKER_PREFIX = "invocation-marker:";

function buildTriggerLifecycleFunctionSource(functionName: string): string {
  return `export default function handler(req) {
  const invocationMarker = "${INVOCATION_MARKER_PREFIX}" + Date.now() + "-" + Math.random().toString(36).slice(2, 10);
  let bodyText = "";
  if (req && typeof req.body === "string") {
    bodyText = req.body;
  } else if (req && req.body !== undefined) {
    try {
      bodyText = JSON.stringify(req.body);
    } catch (e) {
      bodyText = String(e);
    }
  }
  console.log(invocationMarker);
  if (bodyText.length > 0) {
    console.log("event-body:" + bodyText);
  }
  return {
    statusCode: 200,
    body: JSON.stringify({ ok: true, functionName: "${functionName}" }),
    headers: { "Content-Type": "application/json" },
  };
}`;
}

function marker(label: string, runId: number): string {
  return `${label}-${runId}-${Date.now()}`;
}

function escapeSQLLiteral(value: string): string {
  return value.replace(/'/g, "''");
}

function extractInvocationMarker(stdout: string | undefined, context: string): string {
  if (!stdout) {
    throw new Error(`Expected stdout in ${context} log`);
  }
  const match = stdout.match(/invocation-marker:[^\s]+/);
  if (!match) {
    throw new Error(`Expected invocation marker in ${context} stdout`);
  }
  return match[0];
}

function expectDistinctInvocationMarkers(
  initialLog: TriggerLogEntry,
  reenabledLog: TriggerLogEntry,
  context: string,
): void {
  const initialInvocationMarker = extractInvocationMarker(initialLog.stdout, `initial ${context}`);
  const reenabledInvocationMarker = extractInvocationMarker(
    reenabledLog.stdout,
    `re-enabled ${context}`,
  );
  expect(reenabledInvocationMarker).not.toBe(initialInvocationMarker);
}

async function openFunctionTriggers(page: Page, functionName: string, tab: TriggerKind): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
  await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible();
  await page.getByText(functionName).click();
  await expect(page.getByRole("heading", { name: functionName })).toBeVisible({ timeout: 5000 });
  await page.getByRole("button", { name: "Triggers" }).click();
  await expect(page.getByTestId("trigger-tab-db")).toBeVisible();
  if (tab !== "db") {
    await page.getByTestId(`trigger-tab-${tab}`).click();
  }
}

async function getTriggerRowControls(triggerRow: Locator): Promise<TriggerRowControls> {
  const statusBadge = triggerRow.getByTestId(/^trigger-enabled-/).first();
  await expect(statusBadge).toBeVisible({ timeout: 10000 });
  const statusTestID = await statusBadge.getAttribute("data-testid");
  if (!statusTestID || !statusTestID.startsWith("trigger-enabled-")) {
    throw new Error("Could not resolve trigger id from status badge");
  }
  const triggerId = statusTestID.slice("trigger-enabled-".length);
  return {
    triggerId,
    statusBadge,
    toggleButton: triggerRow.getByTestId(`trigger-toggle-${triggerId}`),
    runButton: triggerRow.getByTestId(`trigger-run-${triggerId}`),
  };
}

async function expectTriggerStatus(statusBadge: Locator, enabled: boolean): Promise<void> {
  await expect(statusBadge).toHaveText(enabled ? "Enabled" : "Disabled", { timeout: 10000 });
}

async function assertTriggerToggleLifecycle(params: {
  request: APIRequestContext;
  adminToken: string;
  functionID: string;
  controls: TriggerRowControls;
  path: TriggerPath;
  triggerType: TriggerKind;
  initialMarker: string;
  disabledMarker: string;
  reenabledMarker: string;
  performAction: (markerText: string) => Promise<number>;
  assertDisabledAction?: () => Promise<void>;
}): Promise<{ initialLog: TriggerLogEntry; reenabledLog: TriggerLogEntry }> {
  const {
    request,
    adminToken,
    functionID,
    controls,
    path,
    triggerType,
    initialMarker,
    disabledMarker,
    reenabledMarker,
    performAction,
    assertDisabledAction,
  } = params;

  await expectTriggerStatus(controls.statusBadge, true);
  await expect(controls.toggleButton).toHaveText("Disable");

  const initialActionAt = await performAction(initialMarker);
  const initialLog = await waitForMarkerLog(
    request,
    adminToken,
    functionID,
    path,
    triggerType,
    initialMarker,
    initialActionAt,
  );

  await controls.toggleButton.click();
  await expectTriggerStatus(controls.statusBadge, false);
  await expect(controls.toggleButton).toHaveText("Enable");

  const disabledActionAt = await performAction(disabledMarker);
  if (assertDisabledAction) {
    await assertDisabledAction();
  }
  await expectNoMarkerLog(
    request,
    adminToken,
    functionID,
    path,
    triggerType,
    disabledMarker,
    disabledActionAt,
  );

  await controls.toggleButton.click();
  await expectTriggerStatus(controls.statusBadge, true);
  await expect(controls.toggleButton).toHaveText("Disable");

  const reenabledActionAt = await performAction(reenabledMarker);
  const reenabledLog = await waitForMarkerLog(
    request,
    adminToken,
    functionID,
    path,
    triggerType,
    reenabledMarker,
    reenabledActionAt,
  );
  return { initialLog, reenabledLog };
}

function matchesTriggerMarkerLog(
  log: TriggerLogEntry,
  path: TriggerPath,
  triggerType: TriggerKind,
  markerText: string,
): boolean {
  return (
    log.status === "success" &&
    log.requestPath === path &&
    log.triggerType === triggerType &&
    typeof log.stdout === "string" &&
    log.stdout.includes(markerText)
  );
}

async function waitForMarkerLog(
  request: APIRequestContext,
  adminToken: string,
  functionID: string,
  path: TriggerPath,
  triggerType: TriggerKind,
  markerText: string,
  minCreatedAt: number,
  timeoutMs: number = FUNCTION_LOG_TIMEOUT_MS,
): Promise<TriggerLogEntry> {
  const matched = await waitForFunctionLog(
    request,
    adminToken,
    functionID,
    (rawLog) => {
      const log = rawLog as TriggerLogEntry;
      return matchesTriggerMarkerLog(log, path, triggerType, markerText);
    },
    { timeoutMs, pollIntervalMs: 250, minCreatedAt },
  );
  return matched as TriggerLogEntry;
}

async function expectNoMarkerLog(
  request: APIRequestContext,
  adminToken: string,
  functionID: string,
  path: TriggerPath,
  triggerType: TriggerKind,
  markerText: string,
  minCreatedAt: number,
): Promise<void> {
  await expect(
    waitForFunctionLog(
      request,
      adminToken,
      functionID,
      (rawLog) => {
        const log = rawLog as TriggerLogEntry;
        return matchesTriggerMarkerLog(log, path, triggerType, markerText);
      },
      { timeoutMs: NO_LOG_TIMEOUT_MS, pollIntervalMs: 250, minCreatedAt },
    ),
  ).rejects.toThrow(`No matching log entry found for function ${functionID}`);
}

async function expectVisibleTriggerLogRow(
  page: Page,
  path: TriggerPath,
  triggerType: TriggerKind,
): Promise<void> {
  await page.getByRole("button", { name: "Logs", exact: true }).click();
  const logRow = page.getByTestId(/log-row-/).filter({ hasText: path }).first();
  await expect(logRow).toBeVisible({ timeout: 10000 });
  await expect(logRow).toContainText("POST");
  await expect(logRow).toContainText(path);
  await expect(logRow.getByText(new RegExp(`^${triggerType}$`))).toBeVisible({ timeout: 5000 });
}

/**
 * FULL E2E TEST: Edge Function Triggers
 *
 * Tests the three trigger types end-to-end against a real server:
 * 1. DB trigger: create via dashboard → insert row → verify function log appears
 * 2. Cron trigger: create via dashboard → manual run → verify log
 * 3. Storage trigger: create via dashboard → upload file → verify log
 */

test.describe("Edge Function Triggers (Full E2E)", () => {
  const functionIDs: string[] = [];
  const tableNames: string[] = [];
  const seededStorageFiles: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededStorageFiles.length > 0) {
      const fileName = seededStorageFiles.pop();
      if (!fileName) continue;
      await deleteFile(request, adminToken, "default", fileName).catch(() => {});
    }
    while (tableNames.length > 0) {
      const tableName = tableNames.pop();
      if (!tableName) continue;
      await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${tableName}`).catch(() => {});
    }
    while (functionIDs.length > 0) {
      const functionID = functionIDs.pop();
      if (!functionID) continue;
      await deleteEdgeFunction(request, adminToken, functionID).catch(() => {});
    }
  });

  // ============================================================
  // DB Trigger: create via UI → insert row → verify log
  // ============================================================
  test("DB trigger: create trigger, insert row, verify function log", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/functions");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501 || probeStatus === 500,
      `Edge functions service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const fnName = `db-trig-test-${runId}`;
    const tableName = `_test_db_trig_${runId}`;

    // Arrange: deploy a function that logs trigger payloads
    const fn = await seedEdgeFunction(request, adminToken, {
      name: fnName,
      source: buildTriggerLifecycleFunctionSource(fnName),
    });
    functionIDs.push(fn.id);
    tableNames.push(tableName);

    // Create a test table for the DB trigger to watch
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE IF NOT EXISTS ${tableName} (id serial PRIMARY KEY, name text)`,
    );

    await openFunctionTriggers(page, fnName, "db");

    await page.getByTestId("add-db-trigger-btn").click();
    await page.getByTestId("db-trigger-table").fill(tableName);
    await page.getByTestId("db-event-INSERT").check();
    await page.getByTestId("db-trigger-submit").click();

    const dbRow = page.locator("main").locator("tr").filter({ hasText: tableName }).first();
    await expect(dbRow).toBeVisible({ timeout: 10000 });
    const dbControls = await getTriggerRowControls(dbRow);
    const insertRowForMarker = async (rowMarker: string): Promise<number> => {
      const actionStartedAt = Date.now();
      await execSQL(
        request,
        adminToken,
        `INSERT INTO ${tableName} (name) VALUES ('${escapeSQLLiteral(rowMarker)}')`,
      );
      return actionStartedAt;
    };

    const initialDBMarker = marker("db-initial", runId);
    const disabledDBMarker = marker("db-disabled", runId);
    const reenabledDBMarker = marker("db-reenabled", runId);
    const { initialLog: initialDBLog, reenabledLog: reenabledDBLog } =
      await assertTriggerToggleLifecycle({
        request,
        adminToken,
        functionID: fn.id,
        controls: dbControls,
        path: "/db-event",
        triggerType: "db",
        initialMarker: initialDBMarker,
        disabledMarker: disabledDBMarker,
        reenabledMarker: reenabledDBMarker,
        performAction: insertRowForMarker,
      });
    expect(initialDBLog.triggerType).toBe("db");
    expect(reenabledDBLog.triggerType).toBe("db");

    expectDistinctInvocationMarkers(initialDBLog, reenabledDBLog, "DB");
    await expectVisibleTriggerLogRow(page, "/db-event", "db");
  });

  // ============================================================
  // Cron Trigger: create via UI → manual run → verify log
  // ============================================================
  test("Cron trigger: create trigger, manual run, verify function log", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/functions");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501 || probeStatus === 500,
      `Edge functions service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const fnName = `cron-trig-test-${runId}`;

    // Arrange: deploy a function that logs trigger payloads
    const fn = await seedEdgeFunction(request, adminToken, {
      name: fnName,
      source: buildTriggerLifecycleFunctionSource(fnName),
    });
    functionIDs.push(fn.id);

    await openFunctionTriggers(page, fnName, "cron");

    await page.getByTestId("add-cron-trigger-btn").click();
    await page.getByTestId("cron-trigger-expr").fill("0 0 * * *");
    await page.getByTestId("cron-trigger-payload").fill(JSON.stringify({ testRunId: runId }));
    await page.getByTestId("cron-trigger-submit").click();

    const cronRow = page.locator("tr").filter({ hasText: "0 0 * * *" }).first();
    await expect(cronRow).toBeVisible({ timeout: 10000 });
    const cronControls = await getTriggerRowControls(cronRow);
    await expect(cronControls.runButton).toBeVisible({ timeout: 3000 });
    const clickRunNow = async (runMarker: string): Promise<number> => {
      const payloadJSON = JSON.stringify({ testRunId: runId, marker: runMarker });
      await execSQL(
        request,
        adminToken,
        `UPDATE _ayb_edge_cron_triggers
         SET payload = '${escapeSQLLiteral(payloadJSON)}'::jsonb,
             updated_at = NOW()
         WHERE id = '${escapeSQLLiteral(cronControls.triggerId)}'`,
      );
      const actionStartedAt = Date.now();
      await cronControls.runButton.click();
      return actionStartedAt;
    };
    const initialCronMarker = marker("cron-initial", runId);
    const disabledCronMarker = marker("cron-disabled", runId);
    const reenabledCronMarker = marker("cron-reenabled", runId);
    const { initialLog: initialCronLog, reenabledLog: reenabledCronLog } =
      await assertTriggerToggleLifecycle({
        request,
        adminToken,
        functionID: fn.id,
        controls: cronControls,
        path: "/cron",
        triggerType: "cron",
        initialMarker: initialCronMarker,
        disabledMarker: disabledCronMarker,
        reenabledMarker: reenabledCronMarker,
        performAction: clickRunNow,
        assertDisabledAction: async () => {
          await expect(
            page.getByTestId("toast").filter({ hasText: "cron trigger is disabled" }).last(),
          ).toBeVisible({ timeout: 5000 });
        },
      });
    expect(initialCronLog.requestPath).toBe("/cron");
    expect(initialCronLog.triggerType).toBe("cron");
    expect(reenabledCronLog.requestPath).toBe("/cron");
    expect(reenabledCronLog.triggerType).toBe("cron");

    expectDistinctInvocationMarkers(initialCronLog, reenabledCronLog, "cron");
    await expectVisibleTriggerLogRow(page, "/cron", "cron");
  });

  // ============================================================
  // Storage Trigger: create via UI → upload file → verify log
  // ============================================================
  test("Storage trigger: create trigger, upload file, verify function log", async ({
    page,
    request,
    adminToken,
  }) => {
    const functionsProbeStatus = await probeEndpoint(request, adminToken, "/api/admin/functions");
    test.skip(
      functionsProbeStatus === 503 || functionsProbeStatus === 404 || functionsProbeStatus === 501 || functionsProbeStatus === 500,
      `Edge functions service unavailable (status ${functionsProbeStatus})`,
    );
    const storageProbeStatus = await probeEndpoint(request, adminToken, "/api/storage/default");
    test.skip(
      storageProbeStatus === 503 || storageProbeStatus === 404 || storageProbeStatus === 501 || storageProbeStatus === 500,
      `Storage service unavailable for storage trigger test (status ${storageProbeStatus})`,
    );

    const runId = Date.now();
    const fnName = `stor-trig-test-${runId}`;

    // Arrange: deploy a function that logs trigger payloads
    const fn = await seedEdgeFunction(request, adminToken, {
      name: fnName,
      source: buildTriggerLifecycleFunctionSource(fnName),
    });
    functionIDs.push(fn.id);

    await openFunctionTriggers(page, fnName, "storage");

    await page.getByTestId("add-storage-trigger-btn").click();
    await page.getByTestId("storage-trigger-bucket").fill("default");
    await page.getByTestId("storage-event-upload").check();
    await page.getByTestId("storage-trigger-submit").click();

    const storageRow = page.locator("tr").filter({ hasText: "default" }).first();
    await expect(storageRow).toBeVisible({ timeout: 10000 });
    const storageControls = await getTriggerRowControls(storageRow);
    const uploadFileForMarker = async (fileMarker: string): Promise<number> => {
      const fileName = fileMarker;
      seededStorageFiles.push(fileName);
      const actionStartedAt = Date.now();
      await seedFile(
        request,
        adminToken,
        "default",
        fileName,
        `storage trigger payload ${fileMarker}`,
      );
      return actionStartedAt;
    };

    const initialStorageMarker = marker("storage-initial", runId);
    const disabledStorageMarker = marker("storage-disabled", runId);
    const reenabledStorageMarker = marker("storage-reenabled", runId);
    const {
      initialLog: initialStorageLog,
      reenabledLog: reenabledStorageLog,
    } = await assertTriggerToggleLifecycle({
      request,
      adminToken,
      functionID: fn.id,
      controls: storageControls,
      path: "/storage",
      triggerType: "storage",
      initialMarker: `${initialStorageMarker}.txt`,
      disabledMarker: `${disabledStorageMarker}.txt`,
      reenabledMarker: `${reenabledStorageMarker}.txt`,
      performAction: uploadFileForMarker,
    });
    expect(initialStorageLog.triggerType).toBe("storage");
    expect(reenabledStorageLog.triggerType).toBe("storage");

    expectDistinctInvocationMarkers(initialStorageLog, reenabledStorageLog, "storage");
    await expectVisibleTriggerLogRow(page, "/storage", "storage");
  });
});
