import { test, expect, bootstrapMockedAdminApp, mockAdminEdgeFunctionApis, type EdgeFunctionMockOptions } from "./fixtures";

test.describe("Edge Functions (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("load-and-verify: seeded functions render in list view", async ({ page }) => {
    await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();

    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible();

    // Verify both seeded functions appear with correct details
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("auth-check")).toBeVisible();

    // Verify access badges
    await expect(page.getByTestId("fn-public-ef-001")).toHaveText("Public");
    await expect(page.getByTestId("fn-public-ef-002")).toHaveText("Private");

    // Verify "Never" fallback for auth-check (no lastInvokedAt) — scope to table to avoid sidebar matches
    const table = page.getByRole("table");
    await expect(table.getByText("Never")).toBeVisible();
  });

  test("create function: fills form, deploys, and shows new row in list", async ({ page }) => {
    const apis = await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible();

    // Click New Function
    await page.getByRole("button", { name: /New Function/i }).click();
    await expect(page.getByRole("heading", { name: "Deploy New Function" })).toBeVisible();

    // Fill name
    await page.getByLabel("Name").fill("my-new-func");

    // Deploy
    await page.getByRole("button", { name: /Deploy/i }).click();

    // Verify deploy was called
    await expect.poll(() => apis.deployCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastDeployBody?.name).toBe("my-new-func");

    // Verify we returned to the list and can see the new function
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("cell", { name: "my-new-func" })).toBeVisible({ timeout: 5000 });
  });

  test("detail view: navigates to function detail, shows source editor", async ({ page }) => {
    await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible();

    // Click on hello-world function
    await page.getByText("hello-world").click();

    // Verify detail view loads with function name heading
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Verify editor tab is active (default)
    await expect(page.getByText("Source")).toBeVisible();

    // Verify env vars are populated
    await expect(page.getByRole("textbox", { name: "KEY" })).toHaveValue("API_KEY");
    await expect(page.getByRole("textbox", { name: "value" }).first()).toHaveValue("test-key-123");
  });

  test("update function: edits source and saves", async ({ page }) => {
    const apis = await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Click Save
    await page.getByRole("button", { name: /Save/i }).click();

    // Verify update was called
    await expect.poll(() => apis.updateCalls, { timeout: 5000 }).toBe(1);
  });

  test("delete function: clicks delete, confirms, and removes row from list", async ({ page }) => {
    const apis = await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Click delete
    await page.getByRole("button", { name: /Delete/i }).click();

    // Confirm deletion
    await expect(page.getByText("Are you sure")).toBeVisible();
    await page.getByRole("button", { name: /Confirm/i }).click();

    // Verify delete was called
    await expect.poll(() => apis.deleteCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastDeletedId).toBe("ef-001");

    // Verify we returned to the list and deleted function no longer appears
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("hello-world")).not.toBeVisible({ timeout: 5000 });
  });

  test("logs tab: shows execution logs with expandable rows", async ({ page }) => {
    await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Switch to Logs tab
    await page.getByRole("button", { name: "Logs", exact: true }).click();

    // Verify log entries are visible
    await expect(page.getByText("42ms")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("5001ms")).toBeVisible();

    // Click on the error log to expand it
    await page.locator("tr").filter({ hasText: "5001ms" }).click();

    // Verify expanded error detail
    await expect(page.getByText("execution timeout: 5s exceeded")).toBeVisible({ timeout: 3000 });
  });

  test("logs tab: expand success log shows stdout", async ({ page }) => {
    await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: "Logs", exact: true }).click();
    await expect(page.getByText("42ms")).toBeVisible({ timeout: 5000 });

    // Click on the success log to expand it
    await page.locator("tr").filter({ hasText: "42ms" }).click();

    // Verify stdout content
    await expect(page.getByText("console output here")).toBeVisible({ timeout: 3000 });
  });

  test("invoke tester: sends request and shows response", async ({ page }) => {
    const apis = await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Switch to Invoke tab
    await page.getByRole("button", { name: /Invoke/i }).click();

    // Verify method selector and path input are visible
    await expect(page.getByLabel("HTTP Method")).toBeVisible();
    await expect(page.getByLabel("Request Path")).toBeVisible();

    // Click Send
    await page.getByRole("button", { name: /Send/i }).click();

    // Verify invoke was called
    await expect.poll(() => apis.invokeCalls, { timeout: 5000 }).toBe(1);

    // Verify response is displayed — scope status code to testid to avoid matching dates/other numbers
    await expect(page.getByTestId("invoke-response")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("invoke-status-code")).toHaveText("200");
    await expect(page.getByTestId("invoke-response-body")).toContainText('{"message":"Hello from mock!"}');
  });

  test("invoke tester: adds custom headers", async ({ page }) => {
    const apis = await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Switch to Invoke tab
    await page.getByRole("button", { name: /Invoke/i }).click();

    // Add a header
    await page.getByRole("button", { name: /Add Header/i }).click();
    await page.getByPlaceholder("Header-Name").fill("X-Custom-Auth");
    await page.getByPlaceholder("value").last().fill("Bearer test-token");

    // Send
    await page.getByRole("button", { name: /Send/i }).click();

    // Verify invoke was called with headers
    await expect.poll(() => apis.invokeCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastInvokeBody?.headers).toEqual({ "X-Custom-Auth": ["Bearer test-token"] });

    // Verify response panel is shown (proves invoke completed and rendered, not just sent)
    await expect(page.getByTestId("invoke-response")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("invoke-status-code")).toHaveText("200");
  });

  test("triggers: load-and-verify seeded triggers render across tabs", async ({ page }) => {
    await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: /Triggers/i }).click();
    await expect(page.getByText("Database Triggers")).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("cell", { name: "users" })).toBeVisible();
    await expect(page.getByText("INSERT, UPDATE")).toBeVisible();

    await page.getByTestId("trigger-tab-cron").click();
    await expect(page.getByText("Cron Triggers")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("*/15 * * * *")).toBeVisible();
    await expect(page.getByText("UTC")).toBeVisible();

    await page.getByTestId("trigger-tab-storage").click();
    await expect(page.getByText("Storage Triggers")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("uploads")).toBeVisible();
    await expect(page.getByText("upload", { exact: true })).toBeVisible();
  });

  test("triggers: DB create, disable, and delete flow", async ({ page }) => {
    const apis = await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: /Triggers/i }).click();
    await expect(page.getByText("Database Triggers")).toBeVisible({ timeout: 5000 });

    await page.getByTestId("add-db-trigger-btn").click();
    await page.getByTestId("db-trigger-table").fill("audit_logs");
    await page.getByTestId("db-event-INSERT").check();
    await page.getByTestId("db-event-DELETE").check();
    await page.getByTestId("db-trigger-submit").click();

    await expect.poll(() => apis.dbCreateCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastDBCreateBody).toMatchObject({
      table_name: "audit_logs",
      schema: "public",
      events: ["INSERT", "DELETE"],
      filter_columns: [],
    });
    await expect(page.getByText("audit_logs")).toBeVisible({ timeout: 5000 });

    const createdDBTriggerId = apis.lastCreatedDBTriggerId;
    expect(createdDBTriggerId).toBeTruthy();
    const dbTriggerId = createdDBTriggerId as string;

    await page.getByTestId(`trigger-toggle-${dbTriggerId}`).click();
    await expect.poll(() => apis.dbDisableCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastDBDisabledId).toBe(dbTriggerId);
    await expect(page.getByTestId(`trigger-enabled-${dbTriggerId}`)).toHaveText("Disabled");

    await page.getByTestId(`trigger-delete-${dbTriggerId}`).click();
    await page.getByTestId(`trigger-confirm-delete-${dbTriggerId}`).click();
    await expect.poll(() => apis.dbDeleteCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastDBDeletedId).toBe(dbTriggerId);
    await expect(page.getByText("audit_logs")).not.toBeVisible({ timeout: 5000 });
  });

  test("triggers: Cron create, disable, manual run, and delete flow", async ({ page }) => {
    const apis = await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: /Triggers/i }).click();
    await page.getByTestId("trigger-tab-cron").click();
    await expect(page.getByText("Cron Triggers")).toBeVisible({ timeout: 5000 });

    await page.getByTestId("add-cron-trigger-btn").click();
    await page.getByTestId("cron-trigger-expr").fill("0 * * * *");
    await page.getByTestId("cron-trigger-timezone").fill("America/New_York");
    await page.getByTestId("cron-trigger-payload").fill('{"source":"browser-mocked"}');
    await page.getByTestId("cron-trigger-submit").click();

    await expect.poll(() => apis.cronCreateCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastCronCreateBody).toMatchObject({
      cron_expr: "0 * * * *",
      timezone: "America/New_York",
      payload: { source: "browser-mocked" },
    });
    await expect(page.getByText("0 * * * *")).toBeVisible({ timeout: 5000 });

    const createdCronTriggerId = apis.lastCreatedCronTriggerId;
    expect(createdCronTriggerId).toBeTruthy();
    const cronTriggerId = createdCronTriggerId as string;

    await page.getByTestId(`trigger-toggle-${cronTriggerId}`).click();
    await expect.poll(() => apis.cronDisableCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastCronDisabledId).toBe(cronTriggerId);
    await expect(page.getByTestId(`trigger-enabled-${cronTriggerId}`)).toHaveText("Disabled");

    await page.getByTestId(`trigger-run-${cronTriggerId}`).click();
    await expect.poll(() => apis.cronManualRunCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastCronManualRunId).toBe(cronTriggerId);

    await page.getByTestId(`trigger-delete-${cronTriggerId}`).click();
    await page.getByTestId(`trigger-confirm-delete-${cronTriggerId}`).click();
    await expect.poll(() => apis.cronDeleteCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastCronDeletedId).toBe(cronTriggerId);
    await expect(page.getByText("0 * * * *")).not.toBeVisible({ timeout: 5000 });
  });

  test("triggers: Storage create, disable, and delete flow", async ({ page }) => {
    const apis = await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: /Triggers/i }).click();
    await page.getByTestId("trigger-tab-storage").click();
    await expect(page.getByText("Storage Triggers")).toBeVisible({ timeout: 5000 });

    await page.getByTestId("add-storage-trigger-btn").click();
    await page.getByTestId("storage-trigger-bucket").fill("avatars");
    await page.getByTestId("storage-event-upload").check();
    await page.getByTestId("storage-event-delete").check();
    await page.getByTestId("storage-trigger-prefix").fill("public/");
    await page.getByTestId("storage-trigger-suffix").fill(".png");
    await page.getByTestId("storage-trigger-submit").click();

    await expect.poll(() => apis.storageCreateCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastStorageCreateBody).toMatchObject({
      bucket: "avatars",
      event_types: ["upload", "delete"],
      prefix_filter: "public/",
      suffix_filter: ".png",
    });
    await expect(page.getByText("avatars")).toBeVisible({ timeout: 5000 });

    const createdStorageTriggerId = apis.lastCreatedStorageTriggerId;
    expect(createdStorageTriggerId).toBeTruthy();
    const storageTriggerId = createdStorageTriggerId as string;

    await page.getByTestId(`trigger-toggle-${storageTriggerId}`).click();
    await expect.poll(() => apis.storageDisableCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastStorageDisabledId).toBe(storageTriggerId);
    await expect(page.getByTestId(`trigger-enabled-${storageTriggerId}`)).toHaveText("Disabled");

    await page.getByTestId(`trigger-delete-${storageTriggerId}`).click();
    await page.getByTestId(`trigger-confirm-delete-${storageTriggerId}`).click();
    await expect.poll(() => apis.storageDeleteCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastStorageDeletedId).toBe(storageTriggerId);
    await expect(page.getByText("avatars")).not.toBeVisible({ timeout: 5000 });
  });

  test("back navigation: returns from detail to list", async ({ page }) => {
    await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Click back
    await page.getByRole("button", { name: "Back", exact: true }).click();

    // Verify we're back on the list
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("hello-world")).toBeVisible();
  });

  // ============================================================
  // Error States
  // ============================================================

  test("error: deploy returns compile error and shows inline banner", async ({ page }) => {
    const opts: EdgeFunctionMockOptions = {
      deployResponder: () => ({
        status: 400,
        body: { message: "compile error: Unexpected token at line 3" },
      }),
    };
    await mockAdminEdgeFunctionApis(page, opts);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible();

    // Navigate to create
    await page.getByRole("button", { name: /New Function/i }).click();
    await expect(page.getByRole("heading", { name: "Deploy New Function" })).toBeVisible();

    await page.getByLabel("Name").fill("broken-func");
    await page.getByRole("button", { name: /Deploy/i }).click();

    // Verify inline deploy error banner appears with the compile error message
    const deployBanner = page.getByTestId("deploy-error");
    await expect(deployBanner).toBeVisible({ timeout: 5000 });
    await expect(deployBanner.getByText("Unexpected token at line 3")).toBeVisible();

    // Verify we stayed on the create page (did NOT navigate back to list)
    await expect(page.getByRole("heading", { name: "Deploy New Function" })).toBeVisible();
  });

  test("error: update returns compile error and shows inline banner in editor", async ({ page }) => {
    const opts: EdgeFunctionMockOptions = {
      updateResponder: () => ({
        status: 400,
        body: { message: "compile error: syntax error near 'const'" },
      }),
    };
    await mockAdminEdgeFunctionApis(page, opts);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Click Save to trigger the update error
    await page.getByRole("button", { name: /Save/i }).click();

    // Verify deploy error banner appears in the editor
    const editorBanner = page.getByTestId("deploy-error");
    await expect(editorBanner).toBeVisible({ timeout: 5000 });
    await expect(editorBanner.getByText("syntax error near 'const'")).toBeVisible();
  });

  test("error: invoke returns 500 and shows error toast", async ({ page }) => {
    const opts: EdgeFunctionMockOptions = {
      invokeResponder: () => ({
        status: 500,
        body: { message: "Internal server error: runtime panic" },
      }),
    };
    await mockAdminEdgeFunctionApis(page, opts);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Switch to Invoke tab
    await page.getByRole("button", { name: /Invoke/i }).click();
    await expect(page.getByLabel("HTTP Method")).toBeVisible();

    // Send the request
    await page.getByRole("button", { name: /Send/i }).click();

    // Verify error toast appears (the toast contains the error message)
    await expect(page.getByText(/runtime panic|server error|Invocation failed/i)).toBeVisible({ timeout: 5000 });

    // Verify no response panel is shown (since the invoke failed at HTTP level)
    await expect(page.getByTestId("invoke-response")).not.toBeVisible({ timeout: 2000 });
  });

  test("error: invoke timeout returns 504 and shows timeout message", async ({ page }) => {
    const opts: EdgeFunctionMockOptions = {
      invokeResponder: () => ({
        status: 504,
        body: { message: "execution timeout: 5s exceeded" },
      }),
    };
    const apis = await mockAdminEdgeFunctionApis(page, opts);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: /Invoke/i }).click();
    await expect(page.getByLabel("HTTP Method")).toBeVisible();

    await page.getByRole("button", { name: /Send/i }).click();
    await expect.poll(() => apis.invokeCalls, { timeout: 5000 }).toBe(1);

    const timeoutToast = page.getByTestId("toast").filter({ hasText: "execution timeout: 5s exceeded" });
    await expect(timeoutToast).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("invoke-response")).not.toBeVisible({ timeout: 2000 });
  });

  test("error: list returns 500 and shows error state", async ({ page }) => {
    const opts: EdgeFunctionMockOptions = {
      listResponder: () => ({
        status: 500,
        body: { message: "database connection failed" },
      }),
    };
    await mockAdminEdgeFunctionApis(page, opts);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();

    // Verify error message is displayed in the list view
    await expect(page.getByText(/database connection failed|Failed to load/i)).toBeVisible({ timeout: 5000 });
  });

  // ============================================================
  // Env Var Editing
  // ============================================================

  test("env var editing: add env var, verify save includes it", async ({ page }) => {
    const apis = await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Verify existing env var is present (API_KEY)
    await expect(page.getByTestId("env-key-0")).toHaveValue("API_KEY");

    // Add a new env var
    await page.getByTestId("add-env-var").click();
    await page.getByTestId("env-key-1").fill("DB_HOST");
    await page.getByTestId("env-value-1").fill("localhost:5432");

    // Save
    await page.getByRole("button", { name: /Save/i }).click();

    // Verify update was called with both env vars
    await expect.poll(() => apis.updateCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastUpdateBody?.env_vars).toEqual({
      API_KEY: "test-key-123",
      DB_HOST: "localhost:5432",
    });
  });

  // ============================================================
  // Dirty State Indicator
  // ============================================================

  test("dirty state: editing source shows unsaved indicator, revert clears it", async ({ page }) => {
    await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Initially no dirty indicator
    await expect(page.getByTestId("dirty-indicator")).not.toBeVisible({ timeout: 2000 });

    // Change the timeout value to trigger dirty state
    await page.getByTestId("editor-timeout").fill("10000");

    // Dirty indicator should appear
    await expect(page.getByTestId("dirty-indicator")).toBeVisible({ timeout: 3000 });
    await expect(page.getByText("Unsaved changes")).toBeVisible();

    // Click Revert
    await page.getByTestId("revert-btn").click();

    // Dirty indicator should disappear
    await expect(page.getByTestId("dirty-indicator")).not.toBeVisible({ timeout: 3000 });

    // Timeout should be reverted to original value (5000)
    await expect(page.getByTestId("editor-timeout")).toHaveValue("5000");
  });

  // ============================================================
  // Log Filtering
  // ============================================================

  test("log filtering: filter by status shows only matching logs", async ({ page }) => {
    await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    // Switch to Logs tab
    await page.getByRole("button", { name: "Logs", exact: true }).click();

    // Verify all 3 logs are visible initially (success 42ms, error 5001ms, db success 18ms)
    await expect(page.getByText("42ms")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("5001ms")).toBeVisible();
    await expect(page.getByText("18ms")).toBeVisible();

    // Filter by error status
    await page.getByLabel("Filter by status").selectOption("error");

    // Only error log should remain (5001ms)
    await expect(page.getByText("5001ms")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("42ms")).toBeHidden({ timeout: 3000 });
    await expect(page.getByText("18ms")).toBeHidden();
  });

  test("log filtering: filter by trigger type shows matching logs", async ({ page }) => {
    await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: "Logs", exact: true }).click();

    // Verify db trigger badge is visible for log-003
    await expect(page.getByTestId("log-trigger-log-003")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("log-trigger-log-003")).toHaveText("db");

    // Filter by db trigger type
    await page.getByLabel("Filter by trigger type").selectOption("db");

    // Only the db log should remain (18ms)
    await expect(page.getByText("18ms")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("42ms")).toBeHidden({ timeout: 3000 });
    await expect(page.getByText("5001ms")).toBeHidden();
  });

  test("logs: trigger metadata displays in expanded row", async ({ page }) => {
    await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByText("hello-world")).toBeVisible({ timeout: 5000 });

    await page.getByText("hello-world").click();
    await expect(page.getByRole("heading", { name: "hello-world" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: "Logs", exact: true }).click();
    await expect(page.getByText("18ms")).toBeVisible({ timeout: 5000 });

    // Expand the db-triggered log (18ms, has stdout + triggerId)
    await page.locator("tr").filter({ hasText: "18ms" }).click();

    // Verify trigger metadata in expanded row
    await expect(page.getByText("db event processed")).toBeVisible({ timeout: 3000 });
    await expect(page.getByText("dbt-001")).toBeVisible();
  });

  // ============================================================
  // Full CRUD Flow (create → edit env → save → invoke → view logs → delete)
  // ============================================================

  test("full CRUD flow: create, edit env vars, save, invoke, view logs, delete", async ({ page }) => {
    const apis = await mockAdminEdgeFunctionApis(page);

    await page.goto("/admin/");
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible();

    // 1. Create
    await page.getByRole("button", { name: /New Function/i }).click();
    await page.getByLabel("Name").fill("crud-test-func");
    await page.getByRole("button", { name: /Deploy/i }).click();
    await expect.poll(() => apis.deployCalls, { timeout: 5000 }).toBe(1);

    // Verify function appears in list
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("cell", { name: "crud-test-func" })).toBeVisible({ timeout: 5000 });

    // 2. Navigate to detail and add env var
    await page.getByRole("cell", { name: "crud-test-func" }).click();
    await expect(page.getByRole("heading", { name: "crud-test-func" })).toBeVisible({ timeout: 5000 });

    await page.getByTestId("add-env-var").click();
    await page.getByTestId("env-key-0").fill("SECRET_KEY");
    await page.getByTestId("env-value-0").fill("my-secret-value");

    // 3. Save
    await page.getByRole("button", { name: /Save/i }).click();
    await expect.poll(() => apis.updateCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastUpdateBody?.env_vars).toEqual({ SECRET_KEY: "my-secret-value" });

    // 4. Invoke
    await page.getByRole("button", { name: /Invoke/i }).click();
    await page.getByRole("button", { name: /Send/i }).click();
    await expect.poll(() => apis.invokeCalls, { timeout: 5000 }).toBe(1);
    await expect(page.getByTestId("invoke-status-code")).toHaveText("200", { timeout: 5000 });

    // 5. View logs
    await page.getByRole("button", { name: "Logs", exact: true }).click();
    await expect(page.getByRole("cell", { name: "GET" })).toBeVisible({ timeout: 5000 });

    // 6. Delete - switch to Editor tab (exact match avoids sidebar "SQL Editor")
    await page.getByRole("button", { name: "Editor", exact: true }).click();
    await page.getByRole("button", { name: /Delete/i }).click();
    await expect(page.getByText("Are you sure")).toBeVisible();
    await page.getByRole("button", { name: /Confirm/i }).click();
    await expect.poll(() => apis.deleteCalls, { timeout: 5000 }).toBe(1);

    // Verify back on list, function gone
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("cell", { name: "crud-test-func" })).not.toBeVisible({ timeout: 5000 });
  });
});
