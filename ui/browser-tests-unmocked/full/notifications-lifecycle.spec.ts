import {
  test,
  expect,
  probeEndpoint,
  ensureUserByEmail,
  cleanupUserByEmail,
  cleanupNotificationsByTitle,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Notifications Lifecycle
 *
 * Critical Path: Create notification via UI → verify success → send another
 */

test.describe("Notifications Lifecycle (Full E2E)", () => {
  const userEmails: string[] = [];
  const notificationTitles: string[] = [];

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

  test("create notification via UI and verify success", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/notifications", {
      method: "POST",
      data: {},
    });
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Notifications service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const testEmail = `notif-full-user-${runId}@example.test`;
    const user = await ensureUserByEmail(request, adminToken, testEmail);
    userEmails.push(testEmail);

    const notifTitle = `notif-full-test-${runId}`;
    notificationTitles.push(notifTitle);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: /Notifications/i })).toBeVisible({ timeout: 5000 });

    // Fill notification form
    await page.getByLabel("User ID").fill(user.id);
    await page.getByLabel("Title").fill(notifTitle);
    await page.getByLabel("Body").fill(`Lifecycle test notification body ${runId}`);
    const sendButton = page.getByRole("button", { name: /Send Notification/i });
    const sendEnabled = await sendButton.isEnabled();
    test.skip(!sendEnabled, "Notification send action is disabled in this environment");
    await sendButton.click();

    await expect(page.getByText(/Notification sent successfully/i)).toBeVisible({ timeout: 5000 });

    // Send a second notification to confirm form is reusable
    const secondTitle = `notif-full-second-${runId}`;
    notificationTitles.push(secondTitle);

    await page.getByLabel("User ID").clear();
    await page.getByLabel("User ID").fill(user.id);
    await page.getByLabel("Title").clear();
    await page.getByLabel("Title").fill(secondTitle);
    await page.getByLabel("Body").clear();
    await page.getByLabel("Body").fill(`Second notification body ${runId}`);
    const sendButton2 = page.getByRole("button", { name: /Send Notification/i });
    const sendEnabled2 = await sendButton2.isEnabled();
    test.skip(!sendEnabled2, "Notification resend action is disabled in this environment");
    await sendButton2.click();

    await expect(page.getByText(/Notification sent successfully/i)).toBeVisible({ timeout: 5000 });
  });
});
