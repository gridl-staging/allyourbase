import { test, expect, seedWebhook, deleteWebhook } from "../fixtures";

/**
 * SMOKE TEST: Webhooks - Create and Delete
 *
 * Critical Path: Navigate to Webhooks → Create webhook → Verify in list → Delete
 */

test.describe("Smoke: Webhooks CRUD", () => {
  test("seeded webhook renders in list view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const webhookUrl = `https://example.com/seed-verify-${runId}`;

    // Arrange: seed a webhook via API
    const webhook = await seedWebhook(request, adminToken, webhookUrl);

    // Act: navigate to Webhooks page
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    const webhooksButton = page.locator("aside").getByRole("button", { name: /^Webhooks$/i });
    await webhooksButton.click();
    await expect(page.getByRole("heading", { name: "Webhooks" })).toBeVisible();

    // Assert: seeded webhook URL appears in the table
    await expect(page.getByText(webhookUrl).first()).toBeVisible({ timeout: 5000 });

    // Cleanup
    await deleteWebhook(request, adminToken, webhook.id);
  });

  test("create and delete a webhook via UI", async ({ page }) => {
    const runId = Date.now();

    // Step 1: Navigate to admin dashboard
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    // Step 2: Navigate to Webhooks section
    const webhooksButton = page.locator("aside").getByRole("button", { name: /^Webhooks$/i });
    await expect(webhooksButton).toBeVisible({ timeout: 5000 });
    await webhooksButton.click();

    // Step 3: Verify webhooks view loaded (assert on heading, not sidebar text)
    await expect(page.getByRole("heading", { name: "Webhooks" })).toBeVisible();

    // Step 4: Click "Add Webhook" button
    const addWebhookBtn = page.getByRole("button", { name: /add webhook/i });
    await expect(addWebhookBtn).toBeVisible({ timeout: 5000 });
    await addWebhookBtn.click();

    // Step 5: Fill webhook URL
    const webhookUrl = `https://httpbin.org/post?run=${runId}`;
    const urlInput = page.getByRole("textbox", { name: /^URL/i });
    await expect(urlInput).toBeVisible({ timeout: 5000 });
    await urlInput.fill(webhookUrl);

    // Step 6: Submit the form
    const submitButton = page.getByRole("button", { name: /^create$|^save$/i });
    await expect(submitButton).toBeVisible();
    await submitButton.click();

    // Step 7: Verify webhook appears in list
    await expect(page.getByText(webhookUrl).first()).toBeVisible({ timeout: 5000 });

    // Step 8: Delete the webhook
    const webhookRow = page.locator("tr").filter({ hasText: webhookUrl }).first();
    await webhookRow.getByRole("button", { name: "Delete" }).click();

    // Step 9: Confirm deletion
    await expect(page.getByText("Are you sure")).toBeVisible({ timeout: 3000 });
    await page.getByRole("button", { name: "Delete", exact: true }).last().click();

    // Step 10: Verify webhook is removed from the table
    // Use a more specific selector - check if it's in a table row
    await expect(
      page.locator("tr").filter({ hasText: webhookUrl })
    ).not.toBeVisible({ timeout: 5000 });
  });
});
