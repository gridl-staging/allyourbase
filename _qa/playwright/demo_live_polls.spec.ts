import { test, expect, Page } from "@playwright/test";
import * as fs from "fs";
import * as path from "path";

const DEMO_URL = "http://localhost:5175";
const API_URL = "http://localhost:8090";
const SCREENSHOTS_DIR = path.join(__dirname, "..", "results", "screenshots");

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
      "demo_live_polls_defects.md"
    );
    const content = [
      "# Live-Polls Demo QA Defects",
      `\nDate: ${new Date().toISOString().split("T")[0]}`,
      `\nTotal: ${defects.length} defect(s)\n`,
      ...defects.map((d, i) => `${i + 1}. ${d}`),
    ].join("\n");
    fs.writeFileSync(defectFile, content);
  }
});

test.describe("Live-Polls Demo", () => {
  test("01 - Landing page renders", async ({ page }) => {
    const response = await page.goto(DEMO_URL);
    await page.waitForLoadState("networkidle");

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "polls_01_landing.png"),
      fullPage: true,
    });

    // Check HTTP status
    if (response && response.status() !== 200) {
      recordDefect(`Landing page returns HTTP ${response.status()}`);
    }

    // Check title
    const title = await page.title();
    console.log(`  Title: "${title}"`);

    // Check for key elements
    const bodyText = (await page.locator("body").textContent()) || "";
    if (bodyText.length < 50) {
      recordDefect("Landing page has very little content — may be blank");
    }

    // Look for login/register/sign-up elements
    if (
      bodyText.includes("Login") ||
      bodyText.includes("Sign") ||
      bodyText.includes("Register") ||
      bodyText.includes("Email") ||
      bodyText.includes("Poll")
    ) {
      console.log("  ✓ Landing page has auth/poll related content");
    } else {
      recordDefect(
        "Landing page doesn't show login/register or poll content — confusing for new users"
      );
    }
  });

  test("02 - Register a new user", async ({ page }) => {
    await page.goto(DEMO_URL);
    await page.waitForLoadState("networkidle");

    // Look for register/sign-up link or tab
    const registerLink = page.locator(
      'text="Register", text="Sign up", text="Sign Up", text="Create account", a:has-text("Register"), button:has-text("Register"), a:has-text("Sign up")'
    );
    if ((await registerLink.count()) > 0) {
      await registerLink.first().click();
      await page.waitForTimeout(500);
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "polls_02_register_form.png"),
      fullPage: true,
    });

    // Fill registration form
    const emailInput = page.locator(
      'input[type="email"], input[name="email"], input[placeholder*="email" i]'
    );
    const passwordInput = page.locator(
      'input[type="password"], input[name="password"]'
    );

    if ((await emailInput.count()) > 0 && (await passwordInput.count()) > 0) {
      const testEmail = `qa_polls_${Date.now()}@example.com`;
      await emailInput.first().fill(testEmail);
      await passwordInput.first().fill("TestPassword123!");

      // Submit
      const submitBtn = page.locator(
        'button[type="submit"], button:has-text("Register"), button:has-text("Sign up"), button:has-text("Create")'
      );
      if ((await submitBtn.count()) > 0) {
        await submitBtn.first().click();
        await page.waitForTimeout(2000);
        await page.waitForLoadState("networkidle");
      }

      await page.screenshot({
        path: path.join(SCREENSHOTS_DIR, "polls_02b_after_register.png"),
        fullPage: true,
      });

      console.log("  ✓ Registration form submitted");
    } else {
      recordDefect(
        "Could not find email/password inputs for registration"
      );
    }
  });

  test("03 - Login as demo user", async ({ page }) => {
    await page.goto(DEMO_URL);
    await page.waitForLoadState("networkidle");

    // Fill login form
    const emailInput = page.locator(
      'input[type="email"], input[name="email"], input[placeholder*="email" i]'
    );
    const passwordInput = page.locator(
      'input[type="password"], input[name="password"]'
    );

    if ((await emailInput.count()) > 0 && (await passwordInput.count()) > 0) {
      await emailInput.first().fill("alice@demo.test");
      await passwordInput.first().fill("password123");

      const submitBtn = page.locator(
        'button[type="submit"], button:has-text("Login"), button:has-text("Sign in"), button:has-text("Log in")'
      );
      if ((await submitBtn.count()) > 0) {
        await submitBtn.first().click();
        await page.waitForTimeout(2000);
        await page.waitForLoadState("networkidle");
      }

      await page.screenshot({
        path: path.join(SCREENSHOTS_DIR, "polls_03_after_login.png"),
        fullPage: true,
      });

      // Check we're logged in (should see polls or create poll UI)
      const bodyText = (await page.locator("body").textContent()) || "";
      if (
        bodyText.includes("Poll") ||
        bodyText.includes("poll") ||
        bodyText.includes("Create") ||
        bodyText.includes("Logout") ||
        bodyText.includes("logout")
      ) {
        console.log("  ✓ Successfully logged in, poll-related content visible");
      } else {
        recordDefect(
          "After login, no poll-related content visible — user may be confused"
        );
      }
    } else {
      recordDefect("Could not find login form inputs");
    }
  });

  test("04 - Create a poll", async ({ page }) => {
    await page.goto(DEMO_URL);
    await page.waitForLoadState("networkidle");

    // Login first
    const emailInput = page.locator(
      'input[type="email"], input[name="email"], input[placeholder*="email" i]'
    );
    const passwordInput = page.locator(
      'input[type="password"], input[name="password"]'
    );
    if ((await emailInput.count()) > 0 && (await passwordInput.count()) > 0) {
      await emailInput.first().fill("alice@demo.test");
      await passwordInput.first().fill("password123");
      const submitBtn = page.locator(
        'button[type="submit"], button:has-text("Login"), button:has-text("Sign in")'
      );
      if ((await submitBtn.count()) > 0) {
        await submitBtn.first().click();
        await page.waitForTimeout(2000);
        await page.waitForLoadState("networkidle");
      }
    }

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "polls_04_logged_in_home.png"),
      fullPage: true,
    });

    // Look for create poll button/link
    const createBtn = page.locator(
      'button:has-text("Create"), button:has-text("New Poll"), a:has-text("Create"), a:has-text("New")'
    );
    if ((await createBtn.count()) > 0) {
      await createBtn.first().click();
      await page.waitForTimeout(1000);

      await page.screenshot({
        path: path.join(SCREENSHOTS_DIR, "polls_04b_create_form.png"),
        fullPage: true,
      });
      console.log("  ✓ Create poll form opened");
    } else {
      recordDefect("No 'Create Poll' or 'New Poll' button visible after login");
    }
  });

  test("05 - Check for console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") {
        errors.push(msg.text());
      }
    });

    await page.goto(DEMO_URL);
    await page.waitForLoadState("networkidle");
    await page.waitForTimeout(3000);

    if (errors.length > 0) {
      for (const err of errors) {
        if (!err.includes("favicon") && !err.includes("net::ERR")) {
          recordDefect(`Console error on live-polls: ${err.substring(0, 200)}`);
        }
      }
    } else {
      console.log("  ✓ No console errors");
    }
  });

  test("06 - Check text content and labels make sense", async ({ page }) => {
    await page.goto(DEMO_URL);
    await page.waitForLoadState("networkidle");

    const bodyText = (await page.locator("body").textContent()) || "";

    // Check for placeholder text or lorem ipsum
    if (bodyText.includes("lorem") || bodyText.includes("Lorem")) {
      recordDefect("Page contains placeholder lorem ipsum text");
    }

    // Check for undefined/null text
    if (bodyText.includes("undefined") || bodyText.includes("null")) {
      recordDefect(
        "Page displays 'undefined' or 'null' — likely a rendering bug"
      );
    }

    // Check for [object Object]
    if (bodyText.includes("[object Object]")) {
      recordDefect("Page displays '[object Object]' — object not properly serialized");
    }

    // Check for TODO or FIXME in visible text
    if (bodyText.includes("TODO") || bodyText.includes("FIXME")) {
      recordDefect("Page displays TODO/FIXME text to user");
    }

    console.log("  ✓ Text content sanity check passed");
  });
});
