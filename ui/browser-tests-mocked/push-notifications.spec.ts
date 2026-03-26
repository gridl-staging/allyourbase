import { test, expect, bootstrapMockedAdminApp, mockAdminPushApis } from "./fixtures";

test.describe("Push Notifications (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("load-and-verify: seeded devices render in default list view", async ({ page }) => {
    await mockAdminPushApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Push Notifications$/i }).click();

    await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible();
    await expect(page.getByText("abcdef1234...xyz789")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("fcm")).toBeVisible();
    await expect(page.getByText("android")).toBeVisible();
    await expect(page.getByText("user-001")).toBeVisible();
    await expect(page.getByText("Pixel 8")).toBeVisible();
  });

  test("register device flow: fills form, submits, sees toast, and refreshes list", async ({ page }) => {
    const apis = await mockAdminPushApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Push Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible();

    // Open register modal
    await page.getByRole("button", { name: /Register Device/i }).click();
    await expect(page.getByRole("heading", { name: "Register Device" })).toBeVisible();

    // Fill form — use exact:true to match modal labels, not "Filter App ID"
    await page.getByLabel("App ID", { exact: true }).fill("app-register-test");
    await page.getByLabel("User ID", { exact: true }).fill("user-register-test");
    await page.getByLabel("Token").fill("my-new-token-value");
    await page.getByLabel("Device Name").fill("Test Phone");

    // Submit
    await page.getByRole("button", { name: "Save Device" }).click();

    // Verify toast
    await expect(page.getByText("Device registered")).toBeVisible({ timeout: 5000 });

    // Verify register was called
    expect(apis.registerCalls).toBe(1);
    expect(apis.lastRegisterBody?.token).toBe("my-new-token-value");
    expect(apis.lastRegisterBody?.device_name).toBe("Test Phone");

    // Verify device list refreshed (should see second load call)
    expect(apis.listDevicesCalls).toBeGreaterThanOrEqual(2);
  });

  test("revoke device: clicks revoke, sees toast, and refreshes list", async ({ page }) => {
    const apis = await mockAdminPushApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Push Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible();

    // Wait for device row to appear
    await expect(page.getByText("abcdef1234...xyz789")).toBeVisible({ timeout: 5000 });

    // Click revoke on the first device
    await page.getByRole("button", { name: /Revoke device device-001/i }).click();

    // Verify toast
    await expect(page.getByText("Revoked device device-001")).toBeVisible({ timeout: 5000 });

    // Verify revoke was called
    expect(apis.revokeCalls).toBe(1);
    expect(apis.lastRevokedId).toBe("device-001");
  });

  test("device filters: applies filters and sends correct query params", async ({ page }) => {
    const apis = await mockAdminPushApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Push Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible();
    await expect(page.getByText("abcdef1234...xyz789")).toBeVisible({ timeout: 5000 });

    // Fill device filters
    await page.getByLabel("Filter App ID").fill("  app-filter-test  ");
    await page.getByLabel("Filter User ID").fill("  user-filter-test  ");
    await page.getByText("Include inactive").click();

    // Apply
    await page.getByRole("button", { name: "Apply Filters" }).click();

    // Verify trimmed query params were sent
    await expect.poll(() => apis.lastDevicesQuery, { timeout: 3000 }).toBeTruthy();
    expect(apis.lastDevicesQuery?.get("app_id")).toBe("app-filter-test");
    expect(apis.lastDevicesQuery?.get("user_id")).toBe("user-filter-test");
    expect(apis.lastDevicesQuery?.get("include_inactive")).toBe("true");
  });

  test("deliveries tab: switches tab, loads deliveries with status badges", async ({ page }) => {
    await mockAdminPushApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Push Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible();

    // Switch to deliveries tab
    await page.getByRole("button", { name: /^Deliveries$/i }).click();

    // Verify deliveries render
    await expect(page.getByText("Hello push")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("delivery-user-001")).toBeVisible();

    // Verify sent status badge in delivery row
    const deliveryRow = page.locator("tr").filter({ hasText: "Hello push" });
    await expect(deliveryRow.getByText("sent", { exact: true })).toBeVisible();
  });

  test("delivery status filter: filters by status", async ({ page }) => {
    const apis = await mockAdminPushApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Push Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible();

    // Switch to deliveries
    await page.getByRole("button", { name: /^Deliveries$/i }).click();
    await expect(page.getByText("Hello push")).toBeVisible({ timeout: 5000 });

    // Select failed status
    await page.getByLabel("Status").selectOption("failed");

    // Apply
    await page.getByRole("button", { name: "Apply Filters" }).click();

    // Verify query included status param
    await expect.poll(() => apis.lastDeliveriesQuery?.get("status"), { timeout: 3000 }).toBe("failed");
  });

  test("send test push flow: fills form, sends, sees toast", async ({ page }) => {
    const apis = await mockAdminPushApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Push Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible();

    // Switch to deliveries tab (send button is there)
    await page.getByRole("button", { name: /^Deliveries$/i }).click();
    await expect(page.getByText("Hello push")).toBeVisible({ timeout: 5000 });

    // Open send modal
    await page.getByRole("button", { name: /Send Test Push/i }).click();
    await expect(page.getByRole("heading", { name: "Send Test Push" })).toBeVisible();

    // Fill send form — use exact:true to match modal labels, not "Filter App ID"
    await page.getByLabel("App ID", { exact: true }).fill("app-send-test");
    await page.getByLabel("User ID", { exact: true }).fill("user-send-test");
    await page.getByLabel("Title").fill("Test Notification");
    await page.getByLabel("Body", { exact: true }).fill("This is a test push body");

    // Submit
    await page.getByRole("button", { name: "Send Push" }).click();

    // Verify toast
    await expect(page.getByText("Push delivery queued")).toBeVisible({ timeout: 5000 });

    // Verify send was called with correct data
    expect(apis.sendCalls).toBe(1);
    expect(apis.lastSendBody?.title).toBe("Test Notification");
    expect(apis.lastSendBody?.body).toBe("This is a test push body");
    expect(apis.lastSendBody?.app_id).toBe("app-send-test");
    expect(apis.lastSendBody?.user_id).toBe("user-send-test");
  });

  test("register device shows error toast on server failure", async ({ page }) => {
    await mockAdminPushApis(page, {
      registerResponder: () => ({
        status: 400,
        body: { message: "invalid provider value" },
      }),
    });

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Push Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible();

    // Open register modal
    await page.getByRole("button", { name: /Register Device/i }).click();
    await expect(page.getByRole("heading", { name: "Register Device" })).toBeVisible();

    // Fill required fields — use exact:true to match modal labels
    await page.getByLabel("App ID", { exact: true }).fill("app-err");
    await page.getByLabel("User ID", { exact: true }).fill("user-err");
    await page.getByLabel("Token").fill("token-err");

    // Submit
    await page.getByRole("button", { name: "Save Device" }).click();

    // Verify error toast
    await expect(page.getByText("invalid provider value")).toBeVisible({ timeout: 5000 });
  });

  test("send push shows error toast on server 500", async ({ page }) => {
    await mockAdminPushApis(page, {
      sendResponder: () => ({
        status: 500,
        body: { message: "internal error" },
      }),
    });

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Push Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible();

    await page.getByRole("button", { name: /^Deliveries$/i }).click();
    await expect(page.getByText("Hello push")).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: /Send Test Push/i }).click();
    await expect(page.getByRole("heading", { name: "Send Test Push" })).toBeVisible();

    // Fill send form — use exact:true to match modal labels
    await page.getByLabel("App ID", { exact: true }).fill("app-err");
    await page.getByLabel("User ID", { exact: true }).fill("user-err");
    await page.getByLabel("Title").fill("Err title");
    await page.getByLabel("Body", { exact: true }).fill("Err body");
    await page.getByRole("button", { name: "Send Push" }).click();

    await expect(page.getByText("internal error")).toBeVisible({ timeout: 5000 });
  });

  test("delivery detail: clicks View to expand delivery details", async ({ page }) => {
    await mockAdminPushApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Push Notifications$/i }).click();
    await expect(page.getByRole("heading", { name: "Push Notifications" })).toBeVisible();

    await page.getByRole("button", { name: /^Deliveries$/i }).click();
    await expect(page.getByText("Hello push")).toBeVisible({ timeout: 5000 });

    // Click View button on the delivery row
    await page.getByRole("button", { name: /View delivery delivery-001/i }).click();

    // Verify expanded detail content
    await expect(page.getByText("Push notification body text")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText('"action_url"')).toBeVisible();
    await expect(page.getByText("job-001")).toBeVisible();

    // Click the same button (now shows "Hide") to collapse
    await page.getByRole("button", { name: /View delivery delivery-001|Hide/i }).click();
    await expect(page.getByText("Push notification body text")).toBeHidden();
  });
});
