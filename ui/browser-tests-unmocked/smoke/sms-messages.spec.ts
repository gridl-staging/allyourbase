import { test, expect, seedSMSMessage, cleanupSMSMessages } from "../fixtures";

/**
 * SMOKE TEST: SMS Messages - List View and Send Modal
 *
 * Critical Path: Navigate to SMS Messages → Verify seeded message renders → Test send modal
 */

test.describe("Smoke: SMS Messages", () => {
  test("seeded message renders in messages list", async ({ page, request, adminToken }) => {
    const runId = Date.now();

    // Arrange: seed a message via SQL
    await seedSMSMessage(request, adminToken, {
      body: `smoke-sms-${runId}`,
      to_phone: "+15559990001",
    });

    // Act: navigate to SMS Messages
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /SMS Messages/i }).click();

    // Assert: heading visible (page-body content)
    await expect(page.getByRole("heading", { name: /SMS Messages/i })).toBeVisible({ timeout: 5000 });

    // Assert: seeded phone number visible in table
    await expect(page.getByText("+15559990001").first()).toBeVisible({ timeout: 5000 });

    // Assert: seeded body text visible
    await expect(page.getByText(`smoke-sms-${runId}`).first()).toBeVisible();

    // Cleanup
    await cleanupSMSMessages(request, adminToken, `smoke-sms-${runId}`);
  });

  test("Send SMS modal opens and closes", async ({ page }) => {
    // Act: navigate to SMS Messages
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /SMS Messages/i }).click();
    await expect(page.getByRole("heading", { name: /SMS Messages/i })).toBeVisible({ timeout: 5000 });

    // Act: click Send SMS button
    await page.getByRole("button", { name: /Send SMS/i }).click();

    // Assert: modal content visible
    await expect(page.getByRole("heading", { name: /Send Test SMS/i })).toBeVisible();
    await expect(page.getByLabel(/To \(phone number\)/i)).toBeVisible();
    await expect(page.getByLabel(/Message body/i)).toBeVisible();

    // Act: click Cancel
    await page.getByRole("button", { name: /Cancel/i }).click();

    // Assert: modal closed
    await expect(page.getByRole("heading", { name: /Send Test SMS/i })).toBeHidden();
  });
});
