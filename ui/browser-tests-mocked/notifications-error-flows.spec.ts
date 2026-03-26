import { type Page } from "@playwright/test";
import { test, expect, bootstrapMockedAdminApp, mockAdminNotificationApis } from "./fixtures";

type NotificationFormValues = {
  userId: string;
  title: string;
  body: string;
  channel: string;
};

const firstNotification = {
  userId: "user-001",
  title: "Test Alert",
  body: "Please review this alert.",
  channel: "email",
} satisfies NotificationFormValues;

const secondNotification = {
  userId: "user-002",
  title: "Second Alert",
  body: "Retry this message.",
  channel: "push",
} satisfies NotificationFormValues;

async function openNotificationsPage(page: Page) {
  await page.goto("/admin/");
  await page.getByRole("button", { name: /^Notifications$/i }).click();
  await expect(page.getByRole("heading", { name: /Notifications/i })).toBeVisible();
}

async function fillNotificationForm(
  page: Page,
  values: NotificationFormValues,
) {
  await page.getByLabel(/User ID/i).fill(values.userId);
  await page.getByLabel(/Title/i).fill(values.title);
  await page.getByLabel(/Body/i).fill(values.body);
  await page.getByLabel(/Channel/i).fill(values.channel);
}

async function expectNotificationFormValues(
  page: Page,
  values: NotificationFormValues,
) {
  await expect(page.getByLabel(/User ID/i)).toHaveValue(values.userId);
  await expect(page.getByLabel(/Title/i)).toHaveValue(values.title);
  await expect(page.getByLabel(/Body/i)).toHaveValue(values.body);
  await expect(page.getByLabel(/Channel/i)).toHaveValue(values.channel);
}

async function sendNotification(page: Page) {
  await page.getByRole("button", { name: /Send Notification/i }).click();
}

test.describe("Notifications Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("submit failures show accessible error feedback and preserve field values", async ({ page }) => {
    await mockAdminNotificationApis(page, {
      sendResponder: () => ({ status: 500, body: { message: "delivery service down" } }),
    });
    await openNotificationsPage(page);
    await fillNotificationForm(page, firstNotification);

    await sendNotification(page);

    await expect(page.getByRole("alert")).toContainText("Failed to send notification.");
    await expect(page.getByText(/Notification sent successfully/i)).toBeHidden();

    await expectNotificationFormValues(page, firstNotification);

    await expect(page.getByRole("button", { name: /Send Notification/i })).toBeEnabled();
  });

  test("retry clears stale error and a new submit clears stale success", async ({ page }) => {
    let attempt = 0;
    await mockAdminNotificationApis(page, {
      sendResponder: () => {
        attempt += 1;
        if (attempt === 1) {
          return { status: 500, body: { message: "delivery service down" } };
        }
        if (attempt === 2) {
          return {
            status: 201,
            body: {
              id: "notif-002",
              user_id: firstNotification.userId,
              title: firstNotification.title,
              body: firstNotification.body,
              channel: firstNotification.channel,
              created_at: "2026-03-01T00:00:00Z",
            },
          };
        }
        return { status: 500, body: { message: "provider timeout" } };
      },
    });

    await openNotificationsPage(page);
    await fillNotificationForm(page, firstNotification);

    await sendNotification(page);
    await expect(page.getByRole("alert")).toContainText("Failed to send notification.");

    await sendNotification(page);
    await expect(page.getByRole("alert")).toBeHidden();
    await expect(page.getByText(/Notification sent successfully/i)).toBeVisible();

    await fillNotificationForm(page, secondNotification);

    await sendNotification(page);
    await expect(page.getByText(/Notification sent successfully/i)).toBeHidden();
    await expect(page.getByRole("alert")).toContainText("Failed to send notification.");
    await expect(page.getByRole("button", { name: /Send Notification/i })).toBeEnabled();
  });
});
