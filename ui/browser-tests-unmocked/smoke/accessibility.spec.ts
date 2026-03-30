import {
  test,
  expect,
  waitForDashboard,
  execSQL,
  buildParallelSafeRunID,
  dropTableIfExists,
} from "../fixtures";
import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";

/**
 * SMOKE TEST: Accessibility (axe-core)
 *
 * Runs automated WCAG 2.1 AA accessibility scans on dashboard pages.
 * Critical/serious violations fail; moderate/minor are logged for follow-up.
 */

test.describe("Smoke: Accessibility", () => {
  // Run each a11y scan test in parallel — they are independent page loads.
  test.describe.configure({ mode: "parallel" });

  const pendingCleanupTables: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const tableName of pendingCleanupTables) {
      await dropTableIfExists(request, adminToken, tableName, "a11y table-browser cleanup").catch(
        () => {},
      );
    }
    pendingCleanupTables.length = 0;
  });

  /**
   * Run axe-core on the current page and assert zero critical/serious
   * violations. Logs moderate/minor issues as warnings without failing.
   */
  async function assertAccessible(page: Page, pageName: string) {
    const results = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
      .exclude(".cm-editor")
      .analyze();

    const critical = results.violations.filter(
      (violation) => violation.impact === "critical" || violation.impact === "serious",
    );
    const minor = results.violations.filter(
      (violation) => violation.impact === "moderate" || violation.impact === "minor",
    );

    if (minor.length > 0) {
      console.log(
        `[a11y] ${pageName}: ${minor.length} moderate/minor issue(s):`,
        minor.map((violation) => `${violation.id}: ${violation.help} (${violation.nodes.length} node(s))`),
      );
    }

    expect(
      critical,
      `${pageName}: ${critical.length} critical/serious a11y violation(s): ${critical
        .map((violation) => `${violation.id}: ${violation.help}`)
        .join("; ")}`,
    ).toHaveLength(0);
  }

  /**
   * Navigate to a dashboard page via a sidebar button and run an axe scan.
   */
  async function navigateAndScan(page: Page, buttonName: RegExp, pageName: string) {
    const sidebar = page.locator("aside");
    const button = sidebar.getByRole("button", { name: buttonName });

    await expect(button, `Expected sidebar target ${buttonName} for ${pageName}`).toBeVisible({
      timeout: 5000,
    });
    await button.click();
    await expect(button).toHaveClass(/font-medium/, { timeout: 5000 });
    await page.locator("main").waitFor({ state: "visible", timeout: 5000 });
    await assertAccessible(page, pageName);
  }

  test("dashboard home page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await assertAccessible(page, "Dashboard Home");
  });

  test("table browser page is accessible", async ({ page, request, adminToken }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const tableName = `a11y_table_browser_${runID}`;
    pendingCleanupTables.push(tableName);

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY,
        title TEXT NOT NULL
      );

      INSERT INTO ${tableName} (title) VALUES ('a11y seeded row ${runID}');`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");
    const refreshButton = page.getByRole("button", { name: /refresh schema/i });
    const tableButton = sidebar.getByRole("button", { name: tableName, exact: true });

    await expect(refreshButton).toBeVisible({ timeout: 5000 });
    await expect
      .poll(
        async () => {
          await refreshButton.click();
          return tableButton.isVisible();
        },
        { timeout: 15_000 },
      )
      .toBe(true);

    await tableButton.click();
    await page.locator("main").waitFor({ state: "visible", timeout: 5000 });
    await assertAccessible(page, "Table Browser");
  });

  test("database: SQL editor page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^SQL Editor$/i, "SQL Editor");
  });

  test("database: Functions page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Functions$/i, "Functions");
  });

  test("database: RLS policies page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^RLS Policies$/i, "RLS Policies");
  });

  test("database: Matviews page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Matviews$/i, "Matviews");
  });

  test("database: Schema designer page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Schema Designer$/i, "Schema Designer");
  });

  test("database: FDW page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^FDW$/i, "FDW");
  });

  test("services: Storage page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Storage$/i, "Storage");
  });

  test("services: Sites page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Sites$/i, "Sites");
  });

  test("services: Edge functions page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Edge Functions$/i, "Edge Functions");
  });

  test("services: Webhooks page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Webhooks$/i, "Webhooks");
  });

  test("messaging: SMS health page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^SMS Health$/i, "SMS Health");
  });

  test("messaging: SMS messages page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^SMS Messages$/i, "SMS Messages");
  });

  test("messaging: Email templates page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Email Templates$/i, "Email Templates");
  });

  test("messaging: Push notifications page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Push Notifications$/i, "Push Notifications");
  });

  test("admin: Users page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Users$/i, "Users");
  });

  test("admin: Apps page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Apps$/i, "Apps");
  });

  test("admin: API keys page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^API Keys$/i, "API Keys");
  });

  test("admin: OAuth clients page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^OAuth Clients$/i, "OAuth Clients");
  });

  test("admin: API explorer page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^API Explorer$/i, "API Explorer");
  });

  test("admin: Jobs page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Jobs$/i, "Jobs");
  });

  test("admin: Schedules page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Schedules$/i, "Schedules");
  });

  test("admin: Realtime inspector page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Realtime Inspector$/i, "Realtime Inspector");
  });

  test("admin: Security advisor page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Security Advisor$/i, "Security Advisor");
  });

  test("admin: Performance advisor page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Performance Advisor$/i, "Performance Advisor");
  });

  test("admin: Backups page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Backups$/i, "Backups");
  });

  test("admin: Analytics page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Analytics$/i, "Analytics");
  });

  test("admin: Usage page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Usage$/i, "Usage");
  });

  test("admin: Replicas page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Replicas$/i, "Replicas");
  });

  test("admin: Branches page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Branches$/i, "Branches");
  });

  test("admin: Audit logs page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Audit Logs$/i, "Audit Logs");
  });

  test("admin: Admin logs page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Admin Logs$/i, "Admin Logs");
  });

  test("admin: Secrets page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Secrets$/i, "Secrets");
  });

  test("admin: Custom domains page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Custom Domains$/i, "Custom Domains");
  });

  test("admin: Extensions page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Extensions$/i, "Extensions");
  });

  test("admin: Vector indexes page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Vector Indexes$/i, "Vector Indexes");
  });

  test("admin: Log drains page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Log Drains$/i, "Log Drains");
  });

  test("admin: Stats page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Stats$/i, "Stats");
  });

  test("admin: Notifications page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Notifications$/i, "Notifications");
  });

  test("admin: Incidents page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Incidents$/i, "Incidents");
  });

  test("admin: Support tickets page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Support Tickets$/i, "Support Tickets");
  });

  test("admin: Tenants page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Tenants$/i, "Tenants");
  });

  test("admin: Organizations page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Organizations$/i, "Organizations");
  });

  test("ai: AI assistant page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^AI Assistant$/i, "AI Assistant");
  });

  test("auth: Auth settings page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Auth Settings$/i, "Auth Settings");
  });

  test("auth: MFA management page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^MFA Management$/i, "MFA Management");
  });

  test("auth: Account linking page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Account Linking$/i, "Account Linking");
  });

  test("auth: SAML page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^SAML$/i, "SAML");
  });

  test("auth: Auth hooks page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await navigateAndScan(page, /^Auth Hooks$/i, "Auth Hooks");
  });
});
