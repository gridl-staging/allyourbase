import type { TestInfo } from "@playwright/test";
import { test, expect, execSQL, waitForDashboard } from "../fixtures";

/**
 * FULL E2E TEST: First Table Journey (Onboarding Proof)
 *
 * User Story: A brand-new AYB dashboard user goes from empty schema to visible
 * first data without leaving the UI.
 *
 * This test exercises:
 * 1. Empty-state sidebar guidance ("No tables yet" + "Open SQL Editor" CTA)
 *    — only when the database has no user tables; skipped if pre-existing tables
 *    are present (e.g., from prior test runs in a shared environment)
 * 2. Empty-state main content hint ("Select a table from the sidebar")
 * 3. Navigation to SQL Editor via onboarding CTA or sidebar nav
 * 4. DDL execution (CREATE TABLE) and automatic schema refresh
 * 5. New table appearing in sidebar after schema refresh
 * 6. Table browser showing column headers for the new table
 * 7. DML execution (INSERT) and data visibility in table browser
 *
 * Scope boundary: does NOT cover multi-table CRUD, filtering, sorting,
 * FK relationships, or DDL lifecycle (DROP). Those are covered by
 * blog-platform-journey.spec.ts and sql-editor-lifecycle.spec.ts.
 */

test.describe("First Table Journey (Full E2E)", () => {
  // Cleanup queue: SQL statements pushed during tests, drained in afterEach
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("empty dashboard to first table with data", async ({ page }, testInfo: TestInfo) => {
    const runId = `${Date.now()}_${testInfo.parallelIndex}_${testInfo.repeatEachIndex}_${testInfo.retry}`;
    const tableName = `first_test_${runId}`;

    // Register cleanup before creating resources
    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName}`);

    // Navigate to the admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");

    // --- Empty-state assertions (when database has no user tables) ---
    // In a clean environment, the sidebar shows "No tables yet" with an
    // onboarding CTA. In a shared test env, prior runs may leave tables
    // behind, so we verify empty state when present and skip when not.
    const emptyStateVisible = await sidebar
      .getByText("No tables yet")
      .isVisible()
      .catch(() => false);

    if (emptyStateVisible) {
      await expect(
        sidebar.getByRole("button", { name: "Open SQL Editor" }),
      ).toBeVisible();

      // Main content should show the empty-selection hint
      await expect(
        page.locator("main").getByText("Select a table from the sidebar"),
      ).toBeVisible();

      // Navigate via the onboarding CTA
      await sidebar.getByRole("button", { name: "Open SQL Editor" }).click();
    } else {
      // Tables exist — navigate via the Database section's SQL Editor button
      await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();
    }

    // --- SQL Editor should be visible ---
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // --- CREATE TABLE ---
    await sqlInput.fill(
      `CREATE TABLE ${tableName} (id serial PRIMARY KEY, name text NOT NULL, value int)`,
    );
    await page.getByRole("button", { name: /Execute/i }).click();
    await expect(
      page.getByText(/Statement executed successfully/i),
    ).toBeVisible({ timeout: 5000 });

    // --- Verify table appears in sidebar via schema refresh (onSchemaChange) ---
    // Schema refresh is triggered automatically by SqlEditor.execute() after DDL.
    // Generous timeout to tolerate slow schema fetches under load.
    await expect(
      sidebar.getByText(tableName, { exact: true }),
    ).toBeVisible({ timeout: 10000 });

    // --- Click the table to open the table browser ---
    await sidebar.getByText(tableName, { exact: true }).click();

    // Table browser should display column headers for the new table
    await expect(
      page.getByRole("columnheader", { name: "id" }),
    ).toBeVisible({ timeout: 5000 });

    // --- Return to SQL Editor and INSERT a row ---
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    await sqlInput.fill(
      `INSERT INTO ${tableName} (name, value) VALUES ('hello', 42)`,
    );
    await page.getByRole("button", { name: /Execute/i }).click();
    await expect(page.getByText(/1 row affected/i)).toBeVisible({
      timeout: 5000,
    });

    // --- Navigate back to table browser and verify the row renders ---
    await sidebar.getByText(tableName, { exact: true }).click();
    await expect(page.getByRole("cell", { name: "hello" })).toBeVisible({
      timeout: 5000,
    });
    await expect(page.getByRole("cell", { name: "42" })).toBeVisible();
  });
});
