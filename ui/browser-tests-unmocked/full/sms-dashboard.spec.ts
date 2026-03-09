import { test, expect, execSQL, seedSMSMessage, cleanupSMSDailyCounts, cleanupSMSDailyCountsAll, seedSMSDailyCounts, seedSMSMessageBatch, isSMSProviderConfigured } from "../fixtures";

/**
 * FULL E2E TEST: SMS Dashboard
 *
 * Tests SMS Messages list, status badges, Send SMS modal validation,
 * SMS Health stats (all 3 windows), warning badge, error messages,
 * and pagination.
 */

test.describe("SMS Dashboard (Full E2E)", () => {
  // Serial mode: daily counts tests modify shared CURRENT_DATE row in
  // _ayb_sms_daily_counts. Parallel execution would race and overwrite
  // each other's seeded values, causing non-deterministic failures.
  test.describe.configure({ mode: "serial" });

  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded messages render with correct status badges", async ({ page, request, adminToken }) => {
    const runId = Date.now();

    // Register cleanup early
    pendingCleanup.push(`DELETE FROM _ayb_sms_messages WHERE body LIKE '%full-sms-${runId}%'`);

    // Arrange: seed 3 messages with different statuses
    await seedSMSMessage(request, adminToken, {
      body: `full-sms-${runId}-delivered`,
      to_phone: "+15550001001",
      status: "delivered",
    });
    await seedSMSMessage(request, adminToken, {
      body: `full-sms-${runId}-failed`,
      to_phone: "+15550001002",
      status: "failed",
    });
    await seedSMSMessage(request, adminToken, {
      body: `full-sms-${runId}-pending`,
      to_phone: "+15550001003",
      status: "pending",
    });

    // Act: navigate to SMS Messages
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /SMS Messages/i }).click();
    await expect(page.getByRole("heading", { name: /SMS Messages/i })).toBeVisible({ timeout: 5000 });

    // Assert: each seeded phone number visible in table
    await expect(page.getByText("+15550001001").first()).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("+15550001002").first()).toBeVisible();
    await expect(page.getByText("+15550001003").first()).toBeVisible();

    // Assert: status badges visible in respective rows
    const deliveredRow = page.locator("tr").filter({ hasText: "+15550001001" });
    await expect(deliveredRow.getByTestId("status-badge-delivered")).toBeVisible();

    const failedRow = page.locator("tr").filter({ hasText: "+15550001002" });
    await expect(failedRow.getByTestId("status-badge-failed")).toBeVisible();

    const pendingRow = page.locator("tr").filter({ hasText: "+15550001003" });
    await expect(pendingRow.getByTestId("status-badge-pending")).toBeVisible();
  });

  test("error message renders in failed message row", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    pendingCleanup.push(`DELETE FROM _ayb_sms_messages WHERE body LIKE '%full-sms-err-${runId}%'`);

    // Arrange: seed a failed message with error_message
    await seedSMSMessage(request, adminToken, {
      body: `full-sms-err-${runId}`,
      to_phone: "+15550009999",
      status: "failed",
      error_message: "provider timeout",
    });

    // Act: navigate to SMS Messages
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /SMS Messages/i }).click();
    await expect(page.getByRole("heading", { name: /SMS Messages/i })).toBeVisible({ timeout: 5000 });

    // Assert: phone number visible
    await expect(page.getByText("+15550009999").first()).toBeVisible({ timeout: 5000 });

    // Assert: error message visible in the row
    const failedRow = page.locator("tr").filter({ hasText: "+15550009999" });
    await expect(failedRow.getByText("provider timeout")).toBeVisible();

    // Assert: status badge shows failed
    await expect(failedRow.getByTestId("status-badge-failed")).toBeVisible();
  });

  test("Send SMS modal validates inputs", async ({ page }) => {
    // Act: navigate to SMS Messages
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /SMS Messages/i }).click();
    await expect(page.getByRole("heading", { name: /SMS Messages/i })).toBeVisible({ timeout: 5000 });

    // Act: open Send SMS modal
    await page.getByRole("button", { name: /Send SMS/i }).click();
    await expect(page.getByRole("heading", { name: /Send Test SMS/i })).toBeVisible();

    // Assert: Send button is disabled initially
    const sendButton = page.getByRole("button", { name: /^Send$/i });
    await expect(sendButton).toBeDisabled();

    // Act: fill phone number only
    await page.getByLabel(/To \(phone number\)/i).fill("+15551234567");

    // Assert: Send button still disabled (no message body)
    await expect(sendButton).toBeDisabled();

    // Act: fill message body
    await page.getByLabel(/Message body/i).fill("test message");

    // Assert: Send button is enabled
    await expect(sendButton).toBeEnabled();

    // Act: click Cancel
    await page.getByRole("button", { name: /Cancel/i }).click();

    // Assert: modal closed
    await expect(page.getByRole("heading", { name: /Send Test SMS/i })).toBeHidden();
  });

  test("send test SMS and verify result", async ({ page, request, adminToken }) => {
    // Pre-check: is SMS provider configured?
    const providerReady = await isSMSProviderConfigured(request, adminToken);
    if (!providerReady) {
      test.skip(true, "SMS provider not configured in test environment");
      return;
    }

    // Act: navigate to SMS Messages
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /SMS Messages/i }).click();
    await expect(page.getByRole("heading", { name: /SMS Messages/i })).toBeVisible({ timeout: 5000 });

    // Act: open send modal
    await page.getByTestId("open-send-modal").click();
    await expect(page.getByRole("heading", { name: /Send Test SMS/i })).toBeVisible();

    // Act: fill form and send — use +12025551234 (valid US number that passes NormalizePhone)
    await page.getByLabel(/To \(phone number\)/i).fill("+12025551234");
    await page.getByLabel(/Message body/i).fill("e2e test message");
    await page.getByRole("button", { name: /^Send$/i }).click();

    // Assert: result shows status (queued/sent) and phone number
    const result = page.getByTestId("send-result");
    await expect(result).toBeVisible({ timeout: 10000 });
    await expect(result).toContainText("+12025551234");

    // No cleanup needed — admin sends are not stored in DB
  });

  test("SMS Health stats display with seeded daily counts", async ({ page, request, adminToken }) => {
    // Register cleanup for full 30-day window
    pendingCleanup.push("DELETE FROM _ayb_sms_daily_counts WHERE date >= CURRENT_DATE - INTERVAL '29 days'");

    // Clean ALL daily counts in the 30-day window for deterministic assertions
    // across Today, Last 7 Days, and Last 30 Days cards
    await cleanupSMSDailyCountsAll(request, adminToken);

    // Arrange: seed daily counts — sent=25, confirmed=20, failed=3, rate=80.0%
    await seedSMSDailyCounts(request, adminToken, { count: 25, confirm_count: 20, fail_count: 3 });

    // Act: navigate to SMS Health
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /SMS Health/i }).click();
    await expect(page.getByRole("heading", { name: /SMS Health/i })).toBeVisible({ timeout: 5000 });

    // Assert: all three card labels visible
    await expect(page.getByText("Today")).toBeVisible();
    await expect(page.getByText("Last 7 Days")).toBeVisible();
    await expect(page.getByText("Last 30 Days")).toBeVisible();

    // Assert: stat row labels visible
    await expect(page.getByText("Sent").first()).toBeVisible();
    await expect(page.getByText("Confirmed").first()).toBeVisible();
    await expect(page.getByText("Failed").first()).toBeVisible();
    await expect(page.getByText("Conversion Rate").first()).toBeVisible();

    // Assert: Today card — all 4 values verified
    // Use { exact: true } to prevent substring matches (e.g. "3" inside "Last 30 Days")
    const todayCard = page.getByTestId("sms-stats-today");
    await expect(todayCard.getByText("25", { exact: true })).toBeVisible();
    await expect(todayCard.getByText("20", { exact: true })).toBeVisible();
    await expect(todayCard.getByText("3", { exact: true })).toBeVisible();
    await expect(todayCard.getByText("80.0%", { exact: true })).toBeVisible();

    // Assert: Last 7 Days card — same values (only today seeded, full window cleaned)
    const weekCard = page.getByTestId("sms-stats-last_7d");
    await expect(weekCard.getByText("25", { exact: true })).toBeVisible();
    await expect(weekCard.getByText("20", { exact: true })).toBeVisible();
    await expect(weekCard.getByText("3", { exact: true })).toBeVisible();
    await expect(weekCard.getByText("80.0%", { exact: true })).toBeVisible();

    // Assert: Last 30 Days card — same values
    const monthCard = page.getByTestId("sms-stats-last_30d");
    await expect(monthCard.getByText("25", { exact: true })).toBeVisible();
    await expect(monthCard.getByText("20", { exact: true })).toBeVisible();
    await expect(monthCard.getByText("3", { exact: true })).toBeVisible();
    await expect(monthCard.getByText("80.0%", { exact: true })).toBeVisible();

    // Assert: no warning badge (80% > 10% threshold)
    await expect(page.getByTestId("sms-warning-badge")).toBeHidden();
  });

  test("SMS Health warning badge displays for low conversion rate", async ({ page, request, adminToken }) => {
    // Register cleanup
    pendingCleanup.push("DELETE FROM _ayb_sms_daily_counts WHERE date >= CURRENT_DATE - INTERVAL '29 days'");

    // Clean and seed with low conversion rate: 5/100 = 5.0% < 10% threshold
    await cleanupSMSDailyCountsAll(request, adminToken);
    await seedSMSDailyCounts(request, adminToken, { count: 100, confirm_count: 5, fail_count: 90 });

    // Act: navigate to SMS Health
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /SMS Health/i }).click();
    await expect(page.getByRole("heading", { name: /SMS Health/i })).toBeVisible({ timeout: 5000 });

    // Assert: warning badge visible with correct text
    await expect(page.getByTestId("sms-warning-badge")).toBeVisible();
    await expect(page.getByTestId("sms-warning-badge")).toContainText("low conversion rate");

    // Assert: Today card shows low conversion rate values
    const todayCard = page.getByTestId("sms-stats-today");
    await expect(todayCard.getByText("100")).toBeVisible();
    await expect(todayCard.getByText("5.0%")).toBeVisible();
  });

  test("pagination appears and works with many messages", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    pendingCleanup.push(`DELETE FROM _ayb_sms_messages WHERE body LIKE '%page-test-${runId}-%'`);

    // Arrange: seed 55 messages via batch SQL (exceeds default perPage=50)
    await seedSMSMessageBatch(request, adminToken, 55, `page-test-${runId}-`);

    // Act: navigate to SMS Messages
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /SMS Messages/i }).click();
    await expect(page.getByRole("heading", { name: /SMS Messages/i })).toBeVisible({ timeout: 5000 });

    // Assert: pagination controls visible
    await expect(page.getByTestId("pagination-next")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("pagination-prev")).toBeVisible();
    await expect(page.getByText(/Page 1 of/)).toBeVisible();

    // Assert: Prev is disabled on page 1
    await expect(page.getByTestId("pagination-prev")).toBeDisabled();

    // Act: click Next
    await page.getByTestId("pagination-next").click();

    // Assert: Page 2 visible
    await expect(page.getByText(/Page 2 of/)).toBeVisible();

    // Assert: Prev is now enabled on page 2
    await expect(page.getByTestId("pagination-prev")).toBeEnabled();

    // Act: click Prev to go back to page 1
    await page.getByTestId("pagination-prev").click();

    // Assert: back to Page 1
    await expect(page.getByText(/Page 1 of/)).toBeVisible();
  });
});
