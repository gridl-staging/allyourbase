import { test, expect, execSQL, seedWebhook, deleteWebhook } from "../fixtures";

/**
 * FULL E2E TEST: Webhooks Lifecycle
 *
 * Tests complete webhook management:
 * - Create webhook with URL and events
 * - Toggle enabled/disabled
 * - Edit webhook URL
 * - Test webhook delivery
 * - View delivery history
 * - Delete webhook
 */

test.describe("Webhooks Lifecycle (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded webhook renders in list view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const webhookUrl = `https://example.com/lifecycle-verify-${runId}`;

    // Register cleanup early (by URL pattern) so afterEach runs it even on failure
    pendingCleanup.push(`DELETE FROM _ayb_webhooks WHERE url = '${webhookUrl}';`);

    // Arrange: seed a webhook via API
    await seedWebhook(request, adminToken, webhookUrl);

    // Act: navigate to Webhooks page
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    const webhooksButton = page.locator("aside").getByRole("button", { name: /^Webhooks$/i });
    await webhooksButton.click();
    await expect(page.getByRole("heading", { name: /Webhooks/i })).toBeVisible({ timeout: 5000 });

    // Assert: seeded webhook URL appears in the table
    await expect(page.getByText(webhookUrl).first()).toBeVisible({ timeout: 5000 });

    // Cleanup handled by afterEach
  });

  test("complete webhook management lifecycle", async ({ page }) => {
    const runId = Date.now();
    const webhookUrl = `https://httpbin.org/post?test=${runId}`;
    const updatedUrl = `https://httpbin.org/anything?test=${runId}`;

    // Register cleanup early — URL may change mid-test (edit step), so clean up both
    pendingCleanup.push(
      `DELETE FROM _ayb_webhooks WHERE url LIKE '%test=${runId}%';`,
    );

    // ============================================================
    // Setup: Navigate to Webhooks
    // ============================================================
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    const webhooksButton = page.locator("aside").getByRole("button", { name: /^Webhooks$/i });
    await expect(webhooksButton).toBeVisible({ timeout: 5000 });
    await webhooksButton.click();

    // Wait for webhooks view to load
    await expect(page.getByRole("heading", { name: /Webhooks/i })).toBeVisible({ timeout: 5000 });

    // ============================================================
    // CREATE: Add new webhook
    // ============================================================
    // Click "Add Webhook" button
    const addButton = page.getByRole("button", { name: /add webhook/i });
    await expect(addButton).toBeVisible({ timeout: 5000 });
    await addButton.click();

    // Fill webhook URL (scope to modal to avoid matching "Copy URL" buttons in the table)
    const urlInput = page.getByRole("textbox", { name: /^URL/ });
    await expect(urlInput).toBeVisible({ timeout: 3000 });
    await urlInput.fill(webhookUrl);

    // Submit
    const createBtn = page.getByRole("button", { name: /^create$|^save$/i });
    await expect(createBtn).toBeVisible();
    await createBtn.click();

    // Verify webhook in list
    await expect(page.getByText(webhookUrl).first()).toBeVisible({ timeout: 5000 });

    // ============================================================
    // TOGGLE: Enable/disable webhook (verify direction, not just toast)
    // ============================================================
    // Find the switch/toggle in the webhook row
    const webhookRow = page.locator("tr").filter({ hasText: "httpbin.org" }).first();
    const toggleSwitch = webhookRow.getByRole("switch");

    await expect(toggleSwitch).toBeVisible({ timeout: 2000 });

    // Newly created webhooks default to enabled (aria-checked="true")
    await expect(toggleSwitch).toHaveAttribute("aria-checked", "true");

    // First toggle: must flip to disabled
    await toggleSwitch.click();
    const firstToast = page.getByText(/webhook (enabled|disabled)/i);
    await expect(firstToast.first()).toBeVisible({ timeout: 3000 });
    await expect(toggleSwitch).toHaveAttribute("aria-checked", "false");

    // Wait for first toast to disappear before toggling again
    await expect(firstToast).not.toBeVisible({ timeout: 5000 });

    // Second toggle: must restore to enabled
    await toggleSwitch.click();
    const secondToast = page.getByText(/webhook (enabled|disabled)/i);
    await expect(secondToast.first()).toBeVisible({ timeout: 3000 });
    await expect(toggleSwitch).toHaveAttribute("aria-checked", "true");

    // ============================================================
    // EDIT: Update webhook URL
    // ============================================================
    const editButton = webhookRow.getByRole("button", { name: "Edit" });

    await expect(editButton).toBeVisible({ timeout: 2000 });
    await editButton.click();

    // Update URL in edit form (scope to textbox to avoid "Copy URL" button matches)
    const editUrlInput = page.getByRole("textbox", { name: /^URL/ });
    await expect(editUrlInput).toBeVisible({ timeout: 3000 });
    await editUrlInput.clear();
    await editUrlInput.fill(updatedUrl);

    // Save
    const saveBtn = page.getByRole("button", { name: /^save$|^update$/i });
    await saveBtn.click();

    // Verify updated URL in list
    await expect(page.getByText("httpbin.org/anything").first()).toBeVisible({ timeout: 5000 });

    // ============================================================
    // TEST: Send test delivery
    // ============================================================
    const updatedRow = page.locator("tr").filter({ hasText: "httpbin.org" }).first();
    const testButton = updatedRow.getByRole("button", { name: "Test" });

    await expect(testButton).toBeVisible({ timeout: 2000 });
    await testButton.click();

    // Verify test delivery produced a result toast
    // Both pass/fail are valid outcomes (httpbin may be unreachable), but the feature must respond
    const testToast = page.getByText(/test (passed|failed)|test request failed/i);
    await expect(testToast.first()).toBeVisible({ timeout: 10000 });

    // ============================================================
    // HISTORY: View delivery history
    // ============================================================
    const historyButton = updatedRow.getByRole("button", { name: "Delivery History" });

    await expect(historyButton).toBeVisible({ timeout: 2000 });
    await historyButton.click();

    // Verify history modal/view opens
    const historyModal = page.getByText(/delivery history|deliveries/i);
    await expect(historyModal.first()).toBeVisible({ timeout: 3000 });

    // Close the modal
    const closeBtn = page.getByRole("button", { name: "Close" });
    await expect(closeBtn.first()).toBeVisible({ timeout: 1000 });
    await closeBtn.first().click();

    // ============================================================
    // DELETE: Remove webhook
    // ============================================================
    const deleteRow = page.locator("tr").filter({ hasText: "httpbin.org" }).first();
    const deleteButton = deleteRow.getByRole("button", { name: "Delete" });
    await expect(deleteButton).toBeVisible({ timeout: 3000 });
    await deleteButton.click();

    // Wait for confirmation dialog and confirm
    await expect(page.getByText("Are you sure")).toBeVisible({ timeout: 3000 });
    await page.getByRole("button", { name: "Delete", exact: true }).last().click();

    // Verify deleted
    await expect(page.getByText("httpbin.org").first()).not.toBeVisible({ timeout: 5000 });

  });
});
