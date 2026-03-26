import { test, expect, cleanupPushTestData, isPushEnabled, seedPushDelivery, seedPushDeviceToken, PUSH_TEST_APP_ID, PUSH_TEST_USER_ID, waitForDashboard } from "../fixtures";
import type { Locator, Page } from "@playwright/test";

function previewToken(token: string): string {
  if (token.length <= 16) return token;
  return `${token.slice(0, 10)}...${token.slice(-6)}`;
}

async function openPushNotificationsPage(page: Page): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.locator("aside").getByRole("button", { name: /^Push Notifications$/i }).click();
  await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible({ timeout: 5000 });
}

async function readDeliveryStatus(row: Locator): Promise<"pending" | "sent" | "failed" | "invalid_token" | ""> {
  const text = (await row.textContent()) || "";
  if (text.includes("invalid_token")) return "invalid_token";
  if (text.includes("failed")) return "failed";
  if (text.includes("sent")) return "sent";
  if (text.includes("pending")) return "pending";
  return "";
}

test.describe("Push Notifications Lifecycle (Browser Unmocked)", () => {
  const pendingTokenPatterns: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const tokenPattern of pendingTokenPatterns) {
      await cleanupPushTestData(request, adminToken, tokenPattern).catch(() => {});
    }
    pendingTokenPatterns.length = 0;
  });

  test("load-and-verify: seeded push data renders in devices and deliveries views", async ({ page, request, adminToken }) => {
    const pushEnabled = await isPushEnabled(request, adminToken);
    test.skip(!pushEnabled, "Push service is not enabled in this environment");

    const runID = Date.now();
    const tokenValue = `push-unmocked-seed-${runID}`;
    const seededTitle = `push-seeded-title-${runID}`;
    pendingTokenPatterns.push(tokenValue);

    await seedPushDelivery(request, adminToken, {
      tokenValue,
      provider: "fcm",
      platform: "android",
      title: seededTitle,
      body: `push-seeded-body-${runID}`,
      status: "sent",
      dataPayload: {
        source: "unmocked-seed",
      },
    });

    await openPushNotificationsPage(page);

    const deviceRow = page.locator("tr").filter({ hasText: previewToken(tokenValue) }).first();
    await expect(deviceRow).toBeVisible({ timeout: 10000 });
    await expect(deviceRow.getByText("fcm", { exact: true })).toBeVisible();
    await expect(deviceRow.getByText("android", { exact: true })).toBeVisible();

    await page.getByRole("button", { name: /^Deliveries$/i }).click();
    const deliveryRow = page.locator("tr").filter({ hasText: seededTitle }).first();
    await expect(deliveryRow).toBeVisible({ timeout: 10000 });
    await expect(deliveryRow.getByText("sent", { exact: true })).toBeVisible();
  });

  test("registers a device, sends push, and verifies delivery lifecycle in UI", async ({ page, request, adminToken }) => {
    const pushEnabled = await isPushEnabled(request, adminToken);
    test.skip(!pushEnabled, "Push service is not enabled in this environment");

    const runID = Date.now();
    const setupToken = `push-unmocked-setup-${runID}`;
    const tokenValue = `push-unmocked-lifecycle-${runID}`;
    const deviceName = `Lifecycle Device ${runID}`;
    const title = `Push lifecycle title ${runID}`;
    const body = `Push lifecycle body ${runID}`;

    pendingTokenPatterns.push(setupToken);
    pendingTokenPatterns.push(tokenValue);

    // Arrange: ensure fixed push fixture app/user rows exist before UI-only actions.
    await seedPushDeviceToken(request, adminToken, {
      tokenValue: setupToken,
      provider: "fcm",
      platform: "android",
      deviceName: `Setup Device ${runID}`,
    });

    await openPushNotificationsPage(page);

    await page.getByRole("button", { name: /Register Device/i }).click();
    await expect(page.getByRole("heading", { name: "Register Device" })).toBeVisible();

    await page.getByLabel("App ID", { exact: true }).fill(PUSH_TEST_APP_ID);
    await page.getByLabel("User ID", { exact: true }).fill(PUSH_TEST_USER_ID);
    await page.getByLabel("Provider").selectOption("fcm");
    await page.getByLabel("Platform").selectOption("android");
    await page.getByLabel("Token").fill(tokenValue);
    await page.getByLabel("Device Name").fill(deviceName);
    await page.getByRole("button", { name: "Save Device" }).click();

    await expect(page.getByText("Device registered")).toBeVisible({ timeout: 5000 });

    const registeredDeviceRow = page.locator("tr").filter({ hasText: previewToken(tokenValue) }).first();
    await expect(registeredDeviceRow).toBeVisible({ timeout: 10000 });
    await expect(registeredDeviceRow.getByText(deviceName)).toBeVisible();

    await page.getByRole("button", { name: /^Deliveries$/i }).click();
    await page.getByRole("button", { name: /Send Test Push/i }).click();
    await expect(page.getByRole("heading", { name: "Send Test Push" })).toBeVisible();

    await page.getByLabel("App ID", { exact: true }).fill(PUSH_TEST_APP_ID);
    await page.getByLabel("User ID", { exact: true }).fill(PUSH_TEST_USER_ID);
    await page.getByLabel("Title").fill(title);
    await page.getByLabel("Body", { exact: true }).fill(body);
    await page.getByLabel("Data (JSON)").fill(`{"run_id":"${runID}","source":"browser-unmocked"}`);
    await page.getByRole("button", { name: "Send Push" }).click();

    await expect(page.getByText("Push delivery queued")).toBeVisible({ timeout: 5000 });

    await page.getByLabel("Filter App ID").fill(PUSH_TEST_APP_ID);
    await page.getByLabel("Filter User ID").fill(PUSH_TEST_USER_ID);
    await page.getByRole("button", { name: "Apply Filters" }).click();

    const deliveryRow = page.locator("tr").filter({ hasText: title }).first();
    await expect(deliveryRow).toBeVisible({ timeout: 10000 });

    await expect
      .poll(async () => {
        await page.getByRole("button", { name: "Apply Filters" }).click();
        return readDeliveryStatus(deliveryRow);
      }, { timeout: 15000 })
      .toMatch(/sent|failed|invalid_token/);

    await deliveryRow.getByRole("button", { name: /View delivery/i }).click();
    await expect(page.getByText(body)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(`"run_id": "${runID}"`)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("Job ID")).toBeVisible();
  });
});
