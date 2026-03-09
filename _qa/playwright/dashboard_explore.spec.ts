import { test, expect, Page } from "@playwright/test";
import * as fs from "fs";
import * as path from "path";

const BASE_URL = "http://localhost:8090";
const ADMIN_PASSWORD = "6ec99beaa59ff6a5ad311ed7929fedec";
const SCREENSHOTS_DIR = path.join(__dirname, "..", "results", "screenshots");

// Defect tracking
const defects: string[] = [];
function recordDefect(desc: string) {
  defects.push(desc);
  console.log(`  DEFECT: ${desc}`);
}

test.beforeAll(() => {
  fs.mkdirSync(SCREENSHOTS_DIR, { recursive: true });
});

test.afterAll(() => {
  if (defects.length > 0) {
    const defectFile = path.join(
      __dirname,
      "..",
      "results",
      "dashboard_defects.md"
    );
    const content = [
      "# Dashboard QA Defects",
      `\nDate: ${new Date().toISOString().split("T")[0]}`,
      `\nTotal: ${defects.length} defect(s)\n`,
      ...defects.map((d, i) => `${i + 1}. ${d}`),
    ].join("\n");
    fs.writeFileSync(defectFile, content);
  }
});

async function adminLogin(page: Page) {
  await page.goto(`${BASE_URL}/admin`);
  // Wait for the login form or dashboard to load
  await page.waitForLoadState("networkidle");

  // Check if we need to login
  const passwordInput = page.locator(
    'input[type="password"], input[name="password"]'
  );
  if ((await passwordInput.count()) > 0) {
    await passwordInput.fill(ADMIN_PASSWORD);
    // Find and click submit button
    const submitBtn = page.locator(
      'button[type="submit"], button:has-text("Login"), button:has-text("Sign in"), button:has-text("Enter")'
    );
    await submitBtn.click();
    await page.waitForLoadState("networkidle");
    // Wait a bit for the dashboard to load
    await page.waitForTimeout(2000);
  }
}

test.describe("Dashboard Exploration", () => {
  test("01 - Admin login page renders correctly", async ({ page }) => {
    await page.goto(`${BASE_URL}/admin`);
    await page.waitForLoadState("networkidle");
    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "01_admin_login.png"),
      fullPage: true,
    });

    // Check for login form elements
    const title = await page.title();
    console.log(`  Page title: "${title}"`);
    if (!title || title === "undefined" || title === "") {
      recordDefect(`Admin page has empty/missing title: "${title}"`);
    }

    // Check for password input
    const passwordInput = page.locator(
      'input[type="password"], input[name="password"]'
    );
    const hasPasswordField = (await passwordInput.count()) > 0;
    if (!hasPasswordField) {
      // Maybe already logged in
      console.log("  No password field — may already be logged in");
    }

    // Check for branding
    const bodyText = await page.locator("body").textContent();
    if (
      bodyText &&
      (bodyText.includes("Allyourbase") ||
        bodyText.includes("AYB") ||
        bodyText.includes("allyourbase"))
    ) {
      console.log("  ✓ Branding visible");
    } else {
      recordDefect(
        "No AYB branding visible on login page — new user won't know what product this is"
      );
    }
  });

  test("02 - Dashboard main layout after login", async ({ page }) => {
    await adminLogin(page);
    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "02_dashboard_main.png"),
      fullPage: true,
    });

    // Check sidebar exists
    const sidebar = page.locator(
      'nav, [class*="sidebar"], [class*="Sidebar"], aside'
    );
    if ((await sidebar.count()) > 0) {
      console.log("  ✓ Sidebar/nav exists");
    } else {
      recordDefect("No sidebar/navigation visible on dashboard");
    }

    // Check for key navigation items
    const bodyText = (await page.locator("body").textContent()) || "";
    const expectedItems = [
      "SQL",
      "Users",
      "API",
      "Storage",
      "Webhooks",
      "RLS",
    ];
    for (const item of expectedItems) {
      if (bodyText.includes(item)) {
        console.log(`  ✓ Nav item "${item}" found`);
      } else {
        recordDefect(`Expected navigation item "${item}" not found in sidebar`);
      }
    }
  });

  test("03 - SQL Editor page", async ({ page }) => {
    await adminLogin(page);

    // Click SQL Editor in sidebar
    const sqlLink = page.locator(
      'text="SQL Editor", text="SQL", a:has-text("SQL")'
    );
    if ((await sqlLink.count()) > 0) {
      await sqlLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "03_sql_editor.png"),
      fullPage: true,
    });

    // Check for SQL input area (textarea, codemirror, or contenteditable)
    const sqlInput = page.locator(
      'textarea, [class*="editor"], [class*="Editor"], [contenteditable="true"], .cm-editor'
    );
    if ((await sqlInput.count()) > 0) {
      console.log("  ✓ SQL input area found");
    } else {
      recordDefect("SQL Editor page has no visible SQL input area");
    }

    // Check for Run/Execute button
    const runBtn = page.locator(
      'button:has-text("Run"), button:has-text("Execute"), button:has-text("Submit")'
    );
    if ((await runBtn.count()) > 0) {
      console.log("  ✓ Run/Execute button found");
    } else {
      recordDefect("SQL Editor has no Run/Execute button");
    }
  });

  test("04 - Users page", async ({ page }) => {
    await adminLogin(page);

    const usersLink = page.locator(
      'text="Users", a:has-text("Users"), button:has-text("Users")'
    );
    if ((await usersLink.count()) > 0) {
      await usersLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "04_users.png"),
      fullPage: true,
    });

    // Check for user-related content
    const bodyText = (await page.locator("body").textContent()) || "";
    if (
      bodyText.includes("email") ||
      bodyText.includes("Email") ||
      bodyText.includes("user")
    ) {
      console.log("  ✓ Users page shows user-related content");
    } else {
      recordDefect("Users page does not show email/user content");
    }
  });

  test("05 - API Explorer page", async ({ page }) => {
    await adminLogin(page);

    const apiLink = page.locator(
      'text="API Explorer", text="API", a:has-text("API Explorer"), a:has-text("API")'
    );
    if ((await apiLink.count()) > 0) {
      await apiLink.last().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "05_api_explorer.png"),
      fullPage: true,
    });

    // Check for API explorer elements
    const bodyText = (await page.locator("body").textContent()) || "";
    if (
      bodyText.includes("GET") ||
      bodyText.includes("POST") ||
      bodyText.includes("endpoint") ||
      bodyText.includes("collection")
    ) {
      console.log("  ✓ API Explorer shows HTTP method/endpoint content");
    } else {
      recordDefect(
        "API Explorer does not show HTTP methods or endpoint content"
      );
    }
  });

  test("06 - Storage page", async ({ page }) => {
    await adminLogin(page);

    const storageLink = page.locator(
      'text="Storage", a:has-text("Storage"), button:has-text("Storage")'
    );
    if ((await storageLink.count()) > 0) {
      await storageLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "06_storage.png"),
      fullPage: true,
    });
  });

  test("07 - Webhooks page", async ({ page }) => {
    await adminLogin(page);

    const webhooksLink = page.locator(
      'text="Webhooks", a:has-text("Webhooks"), button:has-text("Webhooks")'
    );
    if ((await webhooksLink.count()) > 0) {
      await webhooksLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "07_webhooks.png"),
      fullPage: true,
    });

    // Check for webhook creation UI
    const bodyText = (await page.locator("body").textContent()) || "";
    if (bodyText.includes("webhook") || bodyText.includes("Webhook")) {
      console.log("  ✓ Webhooks page has webhook content");
    }
  });

  test("08 - RLS Policies page", async ({ page }) => {
    await adminLogin(page);

    const rlsLink = page.locator(
      'text="RLS", text="RLS Policies", a:has-text("RLS"), button:has-text("RLS")'
    );
    if ((await rlsLink.count()) > 0) {
      await rlsLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "08_rls_policies.png"),
      fullPage: true,
    });
  });

  test("09 - API Keys page", async ({ page }) => {
    await adminLogin(page);

    const apiKeysLink = page.locator(
      'text="API Keys", a:has-text("API Keys"), button:has-text("API Keys")'
    );
    if ((await apiKeysLink.count()) > 0) {
      await apiKeysLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "09_api_keys.png"),
      fullPage: true,
    });
  });

  test("10 - Functions page", async ({ page }) => {
    await adminLogin(page);

    const functionsLink = page.locator(
      'text="Functions", a:has-text("Functions"), button:has-text("Functions")'
    );
    if ((await functionsLink.count()) > 0) {
      await functionsLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "10_functions.png"),
      fullPage: true,
    });
  });

  test("11 - Table browser (click first table in sidebar)", async ({
    page,
  }) => {
    await adminLogin(page);
    await page.waitForTimeout(2000);

    // Look for tables section in sidebar - tables are listed under TABLES heading
    const tableLinks = page.locator(
      '[class*="sidebar"] a, nav a, aside a'
    );
    const allLinks = await tableLinks.allTextContents();
    console.log(`  Sidebar links found: ${allLinks.join(", ")}`);

    // Try to click on ayb_users table
    const aybUsersLink = page.locator(
      'text="ayb_users", a:has-text("ayb_users"), button:has-text("ayb_users")'
    );
    if ((await aybUsersLink.count()) > 0) {
      await aybUsersLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
      await page.screenshot({
        path: path.join(SCREENSHOTS_DIR, "11_table_browser.png"),
        fullPage: true,
      });
      console.log("  ✓ Clicked ayb_users table");

      // Check for table data display
      const bodyText = (await page.locator("body").textContent()) || "";
      if (bodyText.includes("email") || bodyText.includes("id")) {
        console.log("  ✓ Table browser shows column data");
      } else {
        recordDefect("Table browser for ayb_users doesn't show expected columns (email, id)");
      }
    } else {
      recordDefect("Cannot find ayb_users table in sidebar to click");
    }
  });

  test("12 - Command palette (Cmd+K)", async ({ page }) => {
    await adminLogin(page);

    // Open command palette
    await page.keyboard.press("Meta+k");
    await page.waitForTimeout(500);

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "12_command_palette.png"),
      fullPage: true,
    });

    // Check if command palette is visible
    const palette = page.locator(
      '[class*="command"], [class*="Command"], [class*="palette"], [class*="Palette"], [role="dialog"]'
    );
    if ((await palette.count()) > 0) {
      console.log("  ✓ Command palette opened");
    } else {
      // Try Ctrl+K for non-Mac
      await page.keyboard.press("Control+k");
      await page.waitForTimeout(500);
      if ((await palette.count()) > 0) {
        console.log("  ✓ Command palette opened (Ctrl+K)");
      } else {
        recordDefect("Command palette does not open with Cmd+K or Ctrl+K");
      }
    }
  });

  test("13 - Check for console errors on dashboard", async ({ page }) => {
    const consoleErrors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") {
        consoleErrors.push(msg.text());
      }
    });

    await adminLogin(page);
    // Navigate through a few pages
    await page.waitForTimeout(3000);

    if (consoleErrors.length > 0) {
      console.log(`  Console errors found: ${consoleErrors.length}`);
      for (const err of consoleErrors) {
        console.log(`    - ${err.substring(0, 200)}`);
        if (
          !err.includes("favicon") &&
          !err.includes("404") &&
          !err.includes("net::ERR")
        ) {
          recordDefect(`Console error on dashboard: ${err.substring(0, 150)}`);
        }
      }
    } else {
      console.log("  ✓ No console errors on dashboard");
    }
  });

  test("14 - Check responsive layout at mobile width", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await adminLogin(page);
    await page.waitForTimeout(1000);

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "14_mobile_view.png"),
      fullPage: true,
    });

    // Check if content is usable at mobile width
    const bodyText = (await page.locator("body").textContent()) || "";
    if (bodyText.length > 50) {
      console.log("  ✓ Dashboard renders at mobile width");
    } else {
      recordDefect("Dashboard may not render properly at mobile width");
    }
  });

  test("15 - Check all text is readable (no truncation/overflow issues)", async ({
    page,
  }) => {
    await adminLogin(page);
    await page.waitForTimeout(2000);

    // Check for horizontal scrollbar (overflow)
    const hasHorizontalScroll = await page.evaluate(() => {
      return document.documentElement.scrollWidth > document.documentElement.clientWidth;
    });

    if (hasHorizontalScroll) {
      recordDefect("Dashboard has horizontal scrollbar — content may overflow");
    } else {
      console.log("  ✓ No horizontal scroll overflow");
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "15_layout_check.png"),
      fullPage: true,
    });
  });

  test("16 - SMS section pages", async ({ page }) => {
    await adminLogin(page);

    // SMS Health
    const smsHealthLink = page.locator(
      'text="SMS Health", a:has-text("SMS Health"), button:has-text("SMS Health")'
    );
    if ((await smsHealthLink.count()) > 0) {
      await smsHealthLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
      await page.screenshot({
        path: path.join(SCREENSHOTS_DIR, "16a_sms_health.png"),
        fullPage: true,
      });
      console.log("  ✓ SMS Health page loaded");
    }

    // SMS Messages
    const smsMessagesLink = page.locator(
      'text="SMS Messages", a:has-text("SMS Messages"), button:has-text("SMS Messages")'
    );
    if ((await smsMessagesLink.count()) > 0) {
      await smsMessagesLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
      await page.screenshot({
        path: path.join(SCREENSHOTS_DIR, "16b_sms_messages.png"),
        fullPage: true,
      });
      console.log("  ✓ SMS Messages page loaded");
    }
  });

  test("17 - SQL Editor execute query and check results", async ({ page }) => {
    await adminLogin(page);

    // Navigate to SQL Editor
    const sqlLink = page.locator(
      'text="SQL Editor", text="SQL", a:has-text("SQL")'
    );
    if ((await sqlLink.count()) > 0) {
      await sqlLink.first().click();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(1000);
    }

    // Try to type a query and execute it
    const editor = page.locator(
      'textarea, .cm-editor .cm-content, [contenteditable="true"]'
    );
    if ((await editor.count()) > 0) {
      await editor.first().click();
      await page.keyboard.type("SELECT 1 + 1 AS result;");
      await page.waitForTimeout(500);

      // Click Run
      const runBtn = page.locator(
        'button:has-text("Run"), button:has-text("Execute")'
      );
      if ((await runBtn.count()) > 0) {
        await runBtn.first().click();
        await page.waitForTimeout(2000);
      } else {
        // Try keyboard shortcut
        await page.keyboard.press("Control+Enter");
        await page.waitForTimeout(2000);
      }

      await page.screenshot({
        path: path.join(SCREENSHOTS_DIR, "17_sql_query_result.png"),
        fullPage: true,
      });

      // Check for result
      const bodyText = (await page.locator("body").textContent()) || "";
      if (bodyText.includes("2") || bodyText.includes("result")) {
        console.log("  ✓ SQL query result displayed");
      } else {
        recordDefect("SQL query 'SELECT 1+1' did not display result '2' on page");
      }
    }
  });
});
