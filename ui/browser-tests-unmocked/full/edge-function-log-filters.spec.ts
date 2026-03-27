import {
  test,
  expect,
  buildParallelSafeRunID,
  seedEdgeFunction,
  deleteEdgeFunction,
  invokePublicEdgeFunctionGET,
  invokePublicEdgeFunctionGETWithQuery,
  waitForFunctionLog,
  waitForDashboard,
  probeEndpoint,
} from "../fixtures";

type EdgeFunctionLogRecord = {
  id: string;
  status: "success" | "error";
  stdout?: string;
  triggerType?: string;
};

function buildLogFilterFunctionSource(): string {
  return `export default function handler(req) {
  // Keep success/error behavior controllable so the filter test can
  // generate both statuses against the same real function.
  if (req.query && req.query.includes("fail=1")) {
    console.log("filter-log-error");
    throw new Error("intentional error for filter test");
  }

  console.log("filter-log-success");
  return {
    statusCode: 200,
    body: JSON.stringify({ ok: true }),
    headers: { "Content-Type": "application/json" },
  };
}`;
}

async function openFunctionLogs(page: Parameters<typeof test>[0]["page"], functionName: string): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
  await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible();
  await page.getByRole("cell", { name: functionName }).click();
  await expect(page.getByRole("heading", { name: functionName })).toBeVisible({ timeout: 10_000 });
  await page.getByRole("button", { name: "Logs", exact: true }).click();
  await expect(page.getByTestId("logs-table")).toBeVisible({ timeout: 10_000 });
}

async function expectRowsVisible(
  page: Parameters<typeof test>[0]["page"],
  expectedLogIDs: string[],
  absentLogIDs: string[] = [],
): Promise<void> {
  for (const logID of expectedLogIDs) {
    await expect(page.getByTestId(`log-row-${logID}`)).toBeVisible({ timeout: 10_000 });
  }
  for (const logID of absentLogIDs) {
    await expect(page.getByTestId(`log-row-${logID}`)).toHaveCount(0);
  }
}

test.describe("Edge Function Log Filters (Full E2E)", () => {
  const functionIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (functionIDs.length > 0) {
      const functionID = functionIDs.pop();
      if (!functionID) continue;
      await deleteEdgeFunction(request, adminToken, functionID).catch(() => {});
    }
  });

  test("filters function logs by status and trigger type", async ({ page, request, adminToken }, testInfo) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/functions");
    // This full E2E flow depends on the optional edge-functions service.
    // Skip cleanly when the environment does not expose that surface.
    // eslint-disable-next-line playwright/no-skipped-test
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501 || probeStatus === 500,
      `Edge functions service unavailable (status ${probeStatus})`,
    );

    const runID = buildParallelSafeRunID(testInfo);
    const functionName = `log-filters-${runID}`;
    const fn = await seedEdgeFunction(request, adminToken, {
      name: functionName,
      public: true,
      source: buildLogFilterFunctionSource(),
    });
    functionIDs.push(fn.id);

    // Generate one success log and one error log against the same function
    // so the UI filters must separate real mixed-status server data.
    const successStartedAt = Date.now();
    const successInvoke = await invokePublicEdgeFunctionGET(request, functionName);
    expect(successInvoke.status()).toBe(200);

    const errorStartedAt = Date.now();
    const errorInvoke = await invokePublicEdgeFunctionGETWithQuery(request, functionName, "fail=1");
    expect(errorInvoke.status()).toBe(500);

    const successLog = (await waitForFunctionLog(
      request,
      adminToken,
      fn.id,
      (log) =>
        log.status === "success" &&
        log.triggerType === "http" &&
        typeof log.stdout === "string" &&
        log.stdout.includes("filter-log-success"),
      { timeoutMs: 15_000, pollIntervalMs: 250, minCreatedAt: successStartedAt },
    )) as EdgeFunctionLogRecord;

    const errorLog = (await waitForFunctionLog(
      request,
      adminToken,
      fn.id,
      (log) =>
        log.status === "error" &&
        log.triggerType === "http" &&
        typeof log.stdout === "string" &&
        log.stdout.includes("filter-log-error"),
      { timeoutMs: 15_000, pollIntervalMs: 250, minCreatedAt: errorStartedAt },
    )) as EdgeFunctionLogRecord;

    await openFunctionLogs(page, functionName);

    // Confirm the baseline first so later filter assertions are comparing
    // against the exact two real log rows created in this test.
    await expectRowsVisible(page, [successLog.id, errorLog.id]);
    await expect(page.getByTestId("logs-no-match")).toHaveCount(0);

    const statusFilter = page.getByTestId("log-status-filter");
    const triggerFilter = page.getByTestId("log-trigger-filter");

    // Success-only proves the status dropdown removes error rows rather than
    // just visually decorating them.
    await statusFilter.selectOption("success");
    await expectRowsVisible(page, [successLog.id], [errorLog.id]);

    // Error-only proves the inverse path and catches asymmetric server-side
    // filtering bugs.
    await statusFilter.selectOption("error");
    await expectRowsVisible(page, [errorLog.id], [successLog.id]);

    await statusFilter.selectOption("");
    await expectRowsVisible(page, [successLog.id, errorLog.id]);

    // Both generated logs came from public HTTP invocations, so the HTTP
    // trigger filter should keep both rows visible.
    await triggerFilter.selectOption("http");
    await expectRowsVisible(page, [successLog.id, errorLog.id]);
    await expect(page.getByTestId("logs-no-match")).toHaveCount(0);

    // Switching to a trigger type we never generated should produce the
    // explicit empty-filter state, proving the server is honoring the filter.
    await triggerFilter.selectOption("db");
    await expect(page.getByTestId("logs-no-match")).toBeVisible({ timeout: 10_000 });
    await expectRowsVisible(page, [], [successLog.id, errorLog.id]);

    // Combining status=error with trigger=http should narrow the result set to
    // only the real error row we created above.
    await triggerFilter.selectOption("http");
    await statusFilter.selectOption("error");
    await expectRowsVisible(page, [errorLog.id], [successLog.id]);

    // Combining success with an unused trigger should stay empty, which proves
    // the filters are composed together rather than one filter overriding the other.
    await statusFilter.selectOption("success");
    await triggerFilter.selectOption("db");
    await expect(page.getByTestId("logs-no-match")).toBeVisible({ timeout: 10_000 });
    await expectRowsVisible(page, [], [successLog.id, errorLog.id]);

    await statusFilter.selectOption("");
    await triggerFilter.selectOption("");
    await expectRowsVisible(page, [successLog.id, errorLog.id]);
    await expect(page.getByTestId("logs-no-match")).toHaveCount(0);
  });
});
