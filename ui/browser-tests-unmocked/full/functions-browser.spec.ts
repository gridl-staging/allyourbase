import { test, expect } from "../fixtures";

/**
 * FULL E2E TEST: Functions Browser
 *
 * Tests PostgreSQL function browsing and execution:
 * - Setup: Create a test function via SQL
 * - Navigate to Functions section
 * - Verify function appears in list
 * - Expand function to see parameters
 * - Execute function with arguments
 * - Verify results
 * - Cleanup: Drop test function
 */

test.describe("Functions Browser (Full E2E)", () => {
  test("browse, execute, and verify function results", async ({ page }) => {
    const runId = Date.now();
    const funcName = `test_add_${runId}`;

    // ============================================================
    // Setup: Create test function via SQL
    // ============================================================
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    await sqlInput.fill(
      `CREATE OR REPLACE FUNCTION ${funcName}(a integer, b integer) RETURNS integer AS $$ SELECT a + b; $$ LANGUAGE SQL;`
    );
    await page.getByRole("button", { name: /run|execute/i }).click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // Reload page and refresh schema to pick up the new function.
    // Retry up to 3 times in case the schema cache is still rebuilding.
    for (let attempt = 0; attempt < 3; attempt++) {
      await page.reload();
      await expect(page.getByText("Allyourbase").first()).toBeVisible();

      // Click refresh schema button
      const refreshButton = page.getByRole("button", { name: "Refresh schema" });
      if (await refreshButton.isVisible({ timeout: 2000 })) {
        await refreshButton.click();
      }

      // Navigate to Functions
      const functionsButton = sidebar.getByRole("button", { name: /^Functions$/i });
      await expect(functionsButton).toBeVisible({ timeout: 5000 });
      await functionsButton.click();
      await expect(page.getByRole("heading", { name: /Functions/i })).toBeVisible({ timeout: 5000 });

      // Check if function appeared
      if (await page.getByText(funcName).first().isVisible({ timeout: 3000 })) {
        break;
      }
    }

    // Final assertion
    await expect(page.getByText(funcName).first()).toBeVisible({ timeout: 5000 });

    // ============================================================
    // EXPAND: Click function to see parameters
    // ============================================================
    const funcButton = page.getByRole("button", { name: new RegExp(funcName) });
    await funcButton.click();

    const paramInputs = page.getByPlaceholder("NULL");
    await expect(paramInputs.first()).toBeVisible({ timeout: 3000 });

    // ============================================================
    // EXECUTE: Fill params and run function
    // ============================================================
    await paramInputs.nth(0).fill("3");
    await paramInputs.nth(1).fill("5");

    const executeButton = page.getByRole("button", { name: /execute|run/i });
    await expect(executeButton.first()).toBeVisible({ timeout: 2000 });
    await executeButton.first().click();

    // ============================================================
    // VERIFY: Check results show 8 (3 + 5)
    // ============================================================
    // Verify the Result label appeared (execution completed)
    await expect(page.getByText("Result").first()).toBeVisible({ timeout: 5000 });
    // Verify the result value — exact match avoids matching durations like "8ms"
    await expect(page.getByText("8", { exact: true }).first()).toBeVisible();

    // ============================================================
    // Cleanup: Drop test function via SQL
    // ============================================================
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();
    const cleanupSql = page.getByLabel("SQL query");
    await expect(cleanupSql).toBeVisible({ timeout: 5000 });
    await cleanupSql.fill(`DROP FUNCTION IF EXISTS ${funcName}(integer, integer);`);
    await page.getByRole("button", { name: /run|execute/i }).click();
  });
});
