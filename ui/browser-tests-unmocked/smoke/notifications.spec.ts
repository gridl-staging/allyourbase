import {
  test,
  expect,
  ensureUserByEmail,
  cleanupNotificationsByTitle,
  cleanupUserByEmail,
  probeEndpoint,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Notifications
 *
 * Critical Path: Navigate to Notifications → Submit notification form →
 * verify success state and reset behavior
 */

test.describe("Smoke: Notifications", () => {
  const notificationTitles: string[] = [];
  const userEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (notificationTitles.length > 0) {
      const title = notificationTitles.pop();
      if (!title) continue;
      await cleanupNotificationsByTitle(request, adminToken, title).catch(() => {});
    }
    while (userEmails.length > 0) {
      const email = userEmails.pop();
      if (!email) continue;
      await cleanupUserByEmail(request, adminToken, email).catch(() => {});
    }
  });

  test("admin can send a notification and see success state", async ({ page, request, adminToken }) => {
    // POST probe with empty body returns 400 if service exists, 501/404 if not.
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/notifications", {
      method: "POST",
      data: {},
    });
    test.skip(
      probeStatus === 501 || probeStatus === 404,
      `Notifications service not configured (status ${probeStatus})`,
    );

    const runId = Date.now();
    const testEmail = `notifications-smoke-${runId}@test.com`;
    const testTitle = `Smoke Notification ${runId}`;
    const testChannel = "email";

    const user = await ensureUserByEmail(request, adminToken, testEmail);
    userEmails.push(testEmail);
    notificationTitles.push(testTitle);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: /Notifications/i })).toBeVisible({ timeout: 15_000 });

    await page.getByLabel("User ID").fill(user.id);
    await page.getByLabel("Title").fill(testTitle);
    await page.getByLabel("Body").fill("Created by smoke test");
    await page.getByLabel("Channel").fill(testChannel);

    const sendButton = page.getByRole("button", { name: /Send Notification/i });
    await expect(sendButton).toBeEnabled();
    await sendButton.click();

    await expect(
      page.getByText("Notification sent successfully."),
    ).toBeVisible({ timeout: 5000 });

    // Successful submit clears required fields and disables submit.
    await expect(page.getByLabel("User ID")).toHaveValue("");
    await expect(page.getByLabel("Title")).toHaveValue("");
    await expect(page.getByLabel("Channel")).toHaveValue("");
    await expect(sendButton).toBeDisabled();
  });
});
