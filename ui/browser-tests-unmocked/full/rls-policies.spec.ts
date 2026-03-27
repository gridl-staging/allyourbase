import { test, expect, execSQL, waitForDashboard, expectRlsPolicyCard } from "../fixtures";

/**
 * FULL E2E TEST: RLS Policy Management
 *
 * Tests Row-Level Security policy management:
 * - Setup: Create test table via SQL
 * - Enable RLS on table
 * - Create a policy
 * - Verify policy in list
 * - Delete policy
 * - Disable RLS
 * - Cleanup: Drop test table
 */

test.describe("RLS Policies (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded RLS policy renders in policy list", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `rls_seed_${runId}`;
    const policyName = `seed_policy_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Arrange: create table, enable RLS, create policy via SQL
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (id SERIAL PRIMARY KEY, name TEXT NOT NULL, user_id UUID);`,
    );
    await execSQL(request, adminToken, `ALTER TABLE ${tableName} ENABLE ROW LEVEL SECURITY;`);
    await execSQL(
      request,
      adminToken,
      `CREATE POLICY ${policyName} ON ${tableName} FOR ALL USING (true);`,
    );

    // Act: navigate to RLS Policies page and select the table
    await page.goto("/admin/");
    await waitForDashboard(page);
    const sidebar = page.locator("aside");
    const rlsButton = sidebar.getByRole("button", { name: /^RLS Policies$/i });
    await rlsButton.click();
    await expect(page.getByText("Tables").first()).toBeVisible({ timeout: 5000 });

    const rlsTableButton = page.locator("main").getByRole("button", { name: tableName });
    await expect(rlsTableButton).toBeVisible({ timeout: 5000 });
    await rlsTableButton.click();

    // Assert: seeded policy name appears in the list
    await expect(page.getByText(policyName).first()).toBeVisible({ timeout: 5000 });

    // Cleanup handled by afterEach
  });

  test("enable RLS and create policy shows enabled badge and new policy row", async ({ page }) => {
    const runId = Date.now();
    const tableName = `rls_test_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // ============================================================
    // Setup: Create test table via SQL
    // ============================================================
    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");

    // Navigate to SQL Editor via sidebar
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // Create test table with user_id column for RLS testing
    await sqlInput.fill(`CREATE TABLE ${tableName} (
      id SERIAL PRIMARY KEY,
      name TEXT NOT NULL,
      user_id UUID
    );`);
    await page.getByRole("button", { name: /^Execute$/i }).click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // Reload to see new table
    await page.reload();
    await waitForDashboard(page);

    // ============================================================
    // Navigate to RLS Policies
    // ============================================================
    const rlsButton = sidebar.getByRole("button", { name: /^RLS Policies$/i });
    await expect(rlsButton).toBeVisible({ timeout: 5000 });
    await rlsButton.click();

    // Verify RLS view loaded — the RLS component has a "Tables" sidebar header
    await expect(page.getByText("Tables").first()).toBeVisible({ timeout: 5000 });

    // ============================================================
    // SELECT TABLE: Choose test table from RLS component's internal sidebar
    // ============================================================
    // The RLS component renders its own table list as buttons inside main.
    // Scope to main to avoid matching the global aside sidebar.
    const rlsTableButton = page.locator("main").getByRole("button", { name: tableName });
    await expect(rlsTableButton).toBeVisible({ timeout: 5000 });
    await rlsTableButton.click();

    // ============================================================
    // ENABLE RLS
    // ============================================================
    const enableButton = page.getByRole("button", { name: /enable rls/i });
    await expect(enableButton).toBeVisible({ timeout: 3000 });
    await enableButton.click();

    // Verify enabled — toast shows "RLS enabled on <table>"
    await expect(page.getByText(/RLS enabled on/i).first()).toBeVisible({ timeout: 3000 });

    // ============================================================
    // CREATE POLICY
    // ============================================================
    const createPolicyBtn = page.getByRole("button", { name: /create policy|new policy|add/i });

    await expect(createPolicyBtn.first()).toBeVisible({ timeout: 3000 });
    await createPolicyBtn.first().click();

    // Fill policy form
    const policyName = `test_policy_${runId}`;

    const nameInput = page.getByLabel("Policy name");
    await expect(nameInput).toBeVisible({ timeout: 3000 });
    await nameInput.fill(policyName);

    // Command select (ALL, SELECT, INSERT, etc.)
    const commandSelect = page.getByLabel("Command");
    await expect(commandSelect).toBeVisible({ timeout: 2000 });
    await commandSelect.selectOption("ALL");

    // USING expression
    const usingInput = page.getByLabel("USING expression");
    await expect(usingInput).toBeVisible({ timeout: 2000 });
    await usingInput.fill("true");

    // Submit policy — button text is "Create Policy" (not just "Create")
    const submitBtn = page.getByRole("button", { name: /^create policy$|^create$|^save$/i });
    await expect(submitBtn).toBeVisible({ timeout: 5000 });
    await submitBtn.click();

    // Assert post-submit UI state owned by RlsPolicies:
    // - modal closes
    // - table shows RLS enabled badge
    // - newly created policy row is rendered
    await expect(page.getByRole("heading", { name: "Create RLS Policy" })).toHaveCount(0);
    await expect(page.locator("main").getByText("RLS Enabled", { exact: true })).toBeVisible({
      timeout: 5000,
    });

    await expectRlsPolicyCard(page, {
      policyName,
      command: "ALL",
      usingExpression: "true",
    });

    // Cleanup handled by afterEach
  });
});
