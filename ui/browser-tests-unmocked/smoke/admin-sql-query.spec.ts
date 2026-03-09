import { test, expect } from "../fixtures";

/**
 * SMOKE TEST: Admin Dashboard - SQL Query Execution
 *
 * Critical Path: Admin logs in → Executes SQL query → Views results
 */

test.describe("Smoke: Admin SQL Query", () => {
  test("admin can execute SQL query and view results", async ({ page }) => {
    // Step 1: Navigate to admin dashboard
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    // Step 2: Navigate to SQL Editor via sidebar
    await page.locator("aside").getByRole("button", { name: /^SQL Editor$/i }).click();

    // Step 3: Find SQL input
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // Step 4: Execute a simple query
    await sqlInput.fill("SELECT 1 AS test_column;");

    // Step 5: Click Execute button
    const runButton = page.getByRole("button", { name: /run|execute/i });
    await expect(runButton).toBeVisible();
    await runButton.click();

    // Step 6: Verify results appear
    await expect(page.getByRole("columnheader", { name: /test_column/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("cell", { name: "1" })).toBeVisible();
  });
});
