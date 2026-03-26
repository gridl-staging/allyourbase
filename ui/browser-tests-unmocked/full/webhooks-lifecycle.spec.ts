import { createHmac } from "node:crypto";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";

import { test, expect, execSQL, seedRecord, seedWebhook, sqlLiteral, waitForDashboard } from "../fixtures";

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
  const pendingServerCloses: Array<() => Promise<void>> = [];

  async function startWebhookTargetServer(): Promise<{
    baseURL: string;
    getRequestCount: () => number;
    getLatestRequest: () => {
      body: Buffer;
      headers: Record<string, string | string[] | undefined>;
      method: string | undefined;
      url: string | undefined;
    } | null;
    close: () => Promise<void>;
  }> {
    return new Promise((resolve, reject) => {
      const receivedRequests: Array<{
        body: Buffer;
        headers: Record<string, string | string[] | undefined>;
        method: string | undefined;
        url: string | undefined;
      }> = [];
      const server = createServer((req, res) => {
        const chunks: Buffer[] = [];
        req.on("data", (chunk) => {
          chunks.push(typeof chunk === "string" ? Buffer.from(chunk) : chunk);
        });
        req.on("end", () => {
          receivedRequests.push({
            body: Buffer.concat(chunks),
            headers: { ...req.headers },
            method: req.method,
            url: req.url,
          });
          res.statusCode = 204;
          res.end();
        });
      });
      server.once("error", reject);
      server.listen(0, "127.0.0.1", () => {
        const address = server.address();
        if (!address || typeof address === "string") {
          server.close();
          reject(new Error("webhook target server did not expose a TCP address"));
          return;
        }
        resolve({
          baseURL: `http://127.0.0.1:${(address as AddressInfo).port}`,
          getRequestCount: () => receivedRequests.length,
          getLatestRequest: () => receivedRequests.at(-1) ?? null,
          close: () =>
            new Promise<void>((resolveClose, rejectClose) => {
              server.close((err) => {
                if (err) {
                  rejectClose(err);
                  return;
                }
                resolveClose();
              });
            }),
        });
      });
    });
  }

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup.splice(0)) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    for (const close of pendingServerCloses.splice(0).reverse()) {
      await close().catch(() => {});
    }
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
    await waitForDashboard(page);
    const webhooksButton = page.locator("aside").getByRole("button", { name: /^Webhooks$/i });
    await webhooksButton.click();
    await expect(page.getByRole("heading", { name: /Webhooks/i })).toBeVisible({ timeout: 5000 });

    // Assert: seeded webhook URL appears in the table
    await expect(page.getByText(webhookUrl).first()).toBeVisible({ timeout: 5000 });

    // Cleanup handled by afterEach
  });

  test("complete webhook management lifecycle", async ({ page, request, adminToken }, testInfo) => {
    const runId = `${Date.now()}_${testInfo.workerIndex}`;
    const tableName = `stage2_webhooks_lifecycle_${runId}`;
    const hmacSecret = `stage2-secret-${runId}`;
    const webhookTarget = await startWebhookTargetServer();
    const webhookUrl = `${webhookTarget.baseURL}/${runId}`;
    const updatedUrl = `${webhookTarget.baseURL}/updated-${runId}`;
    pendingServerCloses.push(webhookTarget.close);

    // Register cleanup early — URL may change mid-test (edit step), so clean up both
    pendingCleanup.push(
      `DROP TABLE IF EXISTS ${tableName};`,
      `DELETE FROM _ayb_webhooks WHERE url = '${sqlLiteral(webhookUrl)}' OR url = '${sqlLiteral(updatedUrl)}';`,
    );
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE IF NOT EXISTS ${tableName} (id BIGSERIAL PRIMARY KEY, title TEXT NOT NULL);`,
    );

    // ============================================================
    // Setup: Navigate to Webhooks
    // ============================================================
    await page.goto("/admin/");
    await waitForDashboard(page);

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
    await page.getByRole("textbox", { name: /^HMAC Secret$/i }).fill(hmacSecret);
    await page.getByRole("textbox", { name: /^Tables$/i }).fill(tableName);

    // Submit
    const createBtn = page.getByRole("button", { name: /^create$|^save$/i });
    await expect(createBtn).toBeVisible();
    await createBtn.click();

    // Verify webhook in list
    const webhookRow = page.locator("tr").filter({ hasText: webhookUrl }).first();
    await expect(webhookRow).toBeVisible({ timeout: 5000 });
    await expect(webhookRow).toContainText(tableName, { timeout: 5000 });

    await seedRecord(request, adminToken, tableName, {
      title: `real-event-${runId}`,
    });
    await expect.poll(() => webhookTarget.getRequestCount(), { timeout: 10000 }).toBe(1);

    const collectionEventRequest = webhookTarget.getLatestRequest();
    expect(collectionEventRequest).not.toBeNull();
    if (!collectionEventRequest) {
      throw new Error("expected webhook target to capture one collection-triggered request");
    }
    expect(collectionEventRequest.method).toBe("POST");
    expect(collectionEventRequest.url).toBe(`/${runId}`);
    const expectedSignature = createHmac("sha256", hmacSecret)
      .update(collectionEventRequest.body)
      .digest("hex");
    expect(collectionEventRequest.headers["x-ayb-signature"]).toBe(expectedSignature);

    // ============================================================
    // TOGGLE: Enable/disable webhook (verify direction, not just toast)
    // ============================================================
    // Find the switch/toggle in the webhook row
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
    await expect(page.locator("tr").filter({ hasText: updatedUrl }).first()).toBeVisible({ timeout: 5000 });

    // ============================================================
    // TEST: Send test delivery
    // ============================================================
    const updatedRow = page.locator("tr").filter({ hasText: updatedUrl }).first();
    const testButton = updatedRow.getByRole("button", { name: "Test" });

    await expect(testButton).toBeVisible({ timeout: 2000 });
    await testButton.click();

    // Verify test delivery succeeded against the deterministic local target.
    const testToast = page.getByText(/test passed/i);
    await expect(testToast.first()).toBeVisible({ timeout: 10000 });
    await expect.poll(() => webhookTarget.getRequestCount(), { timeout: 10000 }).toBe(2);

    const testDeliveryRequest = webhookTarget.getLatestRequest();
    expect(testDeliveryRequest).not.toBeNull();
    if (!testDeliveryRequest) {
      throw new Error("expected webhook test delivery to reach the updated local target");
    }
    expect(testDeliveryRequest.method).toBe("POST");
    expect(testDeliveryRequest.url).toBe(`/updated-${runId}`);
    const expectedTestSignature = createHmac("sha256", hmacSecret)
      .update(testDeliveryRequest.body)
      .digest("hex");
    expect(testDeliveryRequest.headers["x-ayb-signature"]).toBe(expectedTestSignature);

    // ============================================================
    // HISTORY: View delivery history
    // ============================================================
    const historyButton = updatedRow.getByRole("button", { name: "Delivery History" });

    await expect(historyButton).toBeVisible({ timeout: 2000 });
    await historyButton.click();

    // Verify history modal/view opens
    const historyModal = page.getByText(/delivery history|deliveries/i);
    await expect(historyModal.first()).toBeVisible({ timeout: 3000 });
    await expect(page.getByText(updatedUrl).first()).toBeVisible({ timeout: 3000 });

    // Close the modal
    const closeBtn = page.getByRole("button", { name: "Close" });
    await expect(closeBtn.first()).toBeVisible({ timeout: 1000 });
    await closeBtn.first().click();

    // ============================================================
    // DELETE: Remove webhook
    // ============================================================
    const deleteRow = page.locator("tr").filter({ hasText: updatedUrl }).first();
    const deleteButton = deleteRow.getByRole("button", { name: "Delete" });
    await expect(deleteButton).toBeVisible({ timeout: 3000 });
    await deleteButton.click();

    // Wait for confirmation dialog and confirm
    await expect(page.getByText("Are you sure")).toBeVisible({ timeout: 3000 });
    await page.getByRole("button", { name: "Delete", exact: true }).last().click();

    // Verify deleted
    await expect(
      page.locator("tr").filter({ hasText: String(runId) })
    ).not.toBeVisible({ timeout: 5000 });

  });
});
