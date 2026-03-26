import {
  test,
  expect,
  seedEdgeFunction,
  deleteEdgeFunction,
  getEdgeFunctionIDByName,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Edge Functions - Load-and-verify + CRUD
 *
 * Critical Path: Navigate to Edge Functions -> Verify seeded function -> Create -> Invoke -> Delete
 */

test.describe("Smoke: Edge Functions CRUD", () => {
  const functionIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (functionIDs.length > 0) {
      const functionID = functionIDs.pop();
      if (!functionID) continue;
      await deleteEdgeFunction(request, adminToken, functionID).catch(() => {});
    }
  });

  test("seeded function renders in list view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const fnName = `seed-verify-${runId}`;

    // Arrange: deploy a function via admin API
    const fn = await seedEdgeFunction(request, adminToken, { name: fnName, public: true });
    functionIDs.push(fn.id);

    // Act: navigate to Edge Functions page
    await page.goto("/admin/");
    await waitForDashboard(page);
    const edgeFnButton = page.locator("aside").getByRole("button", { name: /^Edge Functions$/i });
    await edgeFnButton.click();
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible();

    // Assert: seeded function name appears in the table
    await expect(page.getByText(fnName).first()).toBeVisible({ timeout: 5000 });

    // Verify it shows as Public
    await expect(page.getByTestId(`fn-public-${fn.id}`)).toHaveText("Public");
  });

  test("create, invoke, and delete a function via UI", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const fnName = `ui-crud-${runId}`;

    // Navigate to Edge Functions
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible();

    // Create a new function
    await page.getByRole("button", { name: /New Function/i }).click();
    await expect(page.getByRole("heading", { name: "Deploy New Function" })).toBeVisible();

    await page.getByLabel("Name").fill(fnName);
    await page.getByRole("button", { name: /Deploy/i }).click();

    // Verify function appears in list after redirect (use cell role to avoid toast match)
    await expect(page.getByRole("cell", { name: fnName })).toBeVisible({ timeout: 10000 });
    // Poll for API consistency — function may not appear in the list immediately after UI create.
    // Use Playwright's expect.toPass() for retry instead of waitForTimeout.
    let createdFunctionID = "";
    await expect(async () => {
      createdFunctionID = await getEdgeFunctionIDByName(request, adminToken, fnName);
    }).toPass({ intervals: [500, 500, 500, 500, 500], timeout: 5000 });
    functionIDs.push(createdFunctionID);

    // Click into the function detail
    await page.getByRole("cell", { name: fnName }).click();
    await expect(page.getByRole("heading", { name: fnName })).toBeVisible({ timeout: 15_000 });

    // Switch to Invoke tab and send a test request
    await page.getByRole("button", { name: /Invoke/i }).click();
    await page.getByRole("button", { name: /Send/i }).click();

    // Verify response appears — scope status code to testid to avoid matching dates/other numbers
    await expect(page.getByTestId("invoke-response")).toBeVisible({ timeout: 10000 });
    await expect(page.getByTestId("invoke-status-code")).toHaveText("200");

    // Switch to Logs tab and verify log entry metadata
    await page.getByRole("button", { name: "Logs", exact: true }).click();

    // Verify method and path appear in the log table (use cell role to avoid sidebar "get started" match)
    await expect(page.getByRole("cell", { name: "GET" })).toBeVisible({ timeout: 10000 });
    const logRow = page.locator("tr").filter({ hasText: "GET" }).first();
    await expect(logRow).toBeVisible();

    // Verify the log row shows a duration (number followed by "ms")
    await expect(logRow.getByText(/\d+ms/)).toBeVisible();

    // Verify the log row shows the request path containing the function name
    await expect(logRow.getByText(new RegExp(fnName))).toBeVisible();

    // Verify trigger type "http" badge appears for HTTP invocations
    await expect(logRow.getByText(/^http$/)).toBeVisible({ timeout: 5000 });

    // Delete the function (exact match avoids sidebar "SQL Editor")
    await page.getByRole("button", { name: "Editor", exact: true }).click();
    await page.getByRole("button", { name: /Delete/i }).click();
    await expect(page.getByText("Are you sure")).toBeVisible();
    await page.getByRole("button", { name: /Confirm/i }).click();

    // Verify we're back on the list and the function is gone
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByRole("cell", { name: fnName })).not.toBeVisible({ timeout: 5000 });
    removeTrackedFunctionID(functionIDs, createdFunctionID);
  });
});

function removeTrackedFunctionID(functionIDs: string[], targetID: string): void {
  const trackedIndex = functionIDs.indexOf(targetID);
  if (trackedIndex >= 0) {
    functionIDs.splice(trackedIndex, 1);
  }
}
