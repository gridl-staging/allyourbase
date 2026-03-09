import { test, expect } from "../fixtures";

/**
 * FULL E2E TEST: API Explorer
 *
 * Tests the interactive API explorer:
 * - Navigate to API Explorer
 * - Send GET request to /api/schema
 * - Verify response display (status, body)
 * - Check cURL code generation
 * - Check JS SDK code generation
 */

test.describe("API Explorer (Full E2E)", () => {
  test("send requests and verify response display", async ({ page }) => {
    // ============================================================
    // Navigate to API Explorer
    // ============================================================
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    const sidebar = page.locator("aside");
    const explorerButton = sidebar.getByRole("button", { name: /^API Explorer$/i });
    await expect(explorerButton).toBeVisible({ timeout: 5000 });
    await explorerButton.click();

    // Verify API Explorer loaded
    await expect(page.getByRole("heading", { name: /API Explorer/i })).toBeVisible({ timeout: 5000 });

    // ============================================================
    // SEND GET REQUEST: /api/schema
    // ============================================================
    // Select GET method via combobox
    const methodSelector = page.getByRole("combobox").first();
    await expect(methodSelector).toBeVisible({ timeout: 2000 });
    await methodSelector.selectOption("GET");

    // Enter path
    const pathInput = page.getByLabel("Request path");
    await expect(pathInput).toBeVisible({ timeout: 3000 });
    await pathInput.clear();
    await pathInput.fill("/api/schema");

    // Click execute
    const executeButton = page.getByRole("button", { name: /send|execute|run/i });
    await expect(executeButton.first()).toBeVisible({ timeout: 2000 });
    await executeButton.first().click();

    // ============================================================
    // VERIFY RESPONSE (scoped to main to avoid matching sidebar text)
    // ============================================================
    const mainContent = page.locator("main");
    const statusCode = mainContent.getByText("200").or(mainContent.getByText(/2\d\d/));
    await expect(statusCode.first()).toBeVisible({ timeout: 10000 });

    // The /api/schema response JSON contains "tables" — scope to main to avoid sidebar match
    const responseBody = mainContent.getByText(/"tables"/i);
    await expect(responseBody.first()).toBeVisible({ timeout: 3000 });

    // ============================================================
    // CODE GENERATION: Verify cURL tab
    // ============================================================
    const curlTab = page.getByRole("button", { name: /curl/i }).or(page.getByText(/cURL/i));
    await expect(curlTab.first()).toBeVisible({ timeout: 2000 });
    await curlTab.first().click();

    // Assert on generated code content, not the tab label (which was already visible)
    await expect(page.getByText(/curl -X/i).first()).toBeVisible({ timeout: 3000 });

    // ============================================================
    // CODE GENERATION: Verify JS SDK tab
    // ============================================================
    const jsTab = page.getByRole("button", { name: /javascript|js|sdk/i }).or(page.getByText(/JavaScript|SDK/i));
    await expect(jsTab.first()).toBeVisible({ timeout: 2000 });
    await jsTab.first().click();

    // /api/schema falls back to raw fetch code (SDK only covers /api/collections/* and /api/rpc/*)
    const sdkCode = page.getByText(/fetch\(|ayb\./i);
    await expect(sdkCode.first()).toBeVisible({ timeout: 3000 });

    // ============================================================
    // SEND another GET REQUEST
    // ============================================================
    await pathInput.clear();
    await pathInput.fill("/api/admin/status");
    await executeButton.first().click();

    const statusResponse = mainContent.getByText("200").or(mainContent.getByText(/2\d\d/));
    await expect(statusResponse.first()).toBeVisible({ timeout: 10000 });

    // The /api/admin/status response contains "auth" — scope to main to avoid matching nav text
    const authField = mainContent.getByText(/"auth"/i);
    await expect(authField.first()).toBeVisible({ timeout: 3000 });
  });
});
