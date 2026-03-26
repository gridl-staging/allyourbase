import {
  test,
  expect,
  seedSMSDailyCounts,
  cleanupSMSDailyCountsAll,
  skipUnlessSMSProviderConfigured,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: SMS Health — Stats Dashboard
 *
 * Critical Path: Seed daily SMS counts → Navigate to SMS Health → Verify the
 * seeded numeric values render in the stat cards (not just static labels).
 */

test.describe("Smoke: SMS Health", () => {
  let seededDailyCounts = false;

  test.afterEach(async ({ request, adminToken }) => {
    if (!seededDailyCounts) {
      return;
    }
    await cleanupSMSDailyCountsAll(request, adminToken).catch(() => {});
    seededDailyCounts = false;
  });

  test("seeded SMS daily counts render in stat cards", async ({
    page,
    request,
    adminToken,
  }) => {
    await skipUnlessSMSProviderConfigured(request, adminToken, test.info());

    // Arrange: clear existing counts then seed deterministic values for today
    seededDailyCounts = true;
    await cleanupSMSDailyCountsAll(request, adminToken);
    await seedSMSDailyCounts(request, adminToken, {
      count: 42,
      confirm_count: 38,
      fail_count: 3,
    });

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page
      .locator("aside")
      .getByRole("button", { name: /SMS Health/i })
      .click();
    await expect(
      page.getByRole("heading", { name: /SMS Health/i }),
    ).toBeVisible({ timeout: 5000 });

    // Verify stat card time-window labels
    await expect(page.getByText("Today")).toBeVisible();
    await expect(page.getByText("Last 7 Days")).toBeVisible();
    await expect(page.getByText("Last 30 Days")).toBeVisible();

    // Verify seeded numeric values appear in the page body
    // "42" should appear as the Sent count for Today (and 7d/30d windows)
    await expect(page.getByText("42").first()).toBeVisible({ timeout: 5000 });

    // Verify stat row labels
    await expect(page.getByText("Sent").first()).toBeVisible();
    await expect(page.getByText("Confirmed").first()).toBeVisible();
    await expect(page.getByText("Failed").first()).toBeVisible();
    await expect(page.getByText("Conversion Rate").first()).toBeVisible();
  });
});
