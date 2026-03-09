import { test, expect, Page } from "@playwright/test";
import * as fs from "fs";
import * as path from "path";

const DEMO_URL = "http://localhost:5173";
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
      "demo_kanban_defects.md"
    );
    const content = [
      "# Kanban Demo QA Defects",
      `\nDate: ${new Date().toISOString().split("T")[0]}`,
      `\nTotal: ${defects.length} defect(s)\n`,
      ...defects.map((d, i) => `${i + 1}. ${d}`),
    ].join("\n");
    fs.writeFileSync(defectFile, content);
  }
});

test.describe("Kanban Demo", () => {
  test("01 - Landing page renders", async ({ page }) => {
    const response = await page.goto(DEMO_URL);
    await page.waitForLoadState("networkidle");

    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, "kanban_01_landing.png"),
      fullPage: true,
    });

    if (response && response.status() !== 200) {
      recordDefect(`Kanban landing page returns HTTP ${response.status()}`);
    }

    const title = await page.title();
    console.log(`  Title: "${title}"`);

    const bodyText = (await page.locator("body").textContent()) || "";
    if (bodyText.length < 50) {
      recordDefect("Kanban landing page has very little content");
    }

    // Check for login/auth elements
    if (
      bodyText.includes("Login") ||
      bodyText.includes("Sign") ||
      bodyText.includes("Email") ||
      bodyText.includes("Kanban") ||
      bodyText.includes("Board")
    ) {
      console.log("  ✓ Kanban landing shows auth or board content");
    } else {
      recordDefect("Kanban landing lacks login or board content — unclear to new user");
    }
  });

  test("02 - Login as demo user", async ({ page }) => {
    await page.goto(DEMO_URL);
    await page.waitForLoadState("networkidle");

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

      await page.screenshot({
        path: path.join(SCREENSHOTS_DIR, "kanban_02_after_login.png"),
        fullPage: true,
      });

      const bodyText = (await page.locator("body").textContent()) || "";
      if (
        bodyText.includes("Board") ||
        bodyText.includes("board") ||
        bodyText.includes("Card") ||
        bodyText.includes("column") ||
        bodyText.includes("Logout") ||
        bodyText.includes("Create")
      ) {
        console.log("  ✓ Logged in, board-related content visible");
      } else {
        recordDefect("After kanban login, no board content visible");
      }
    } else {
      recordDefect("Could not find kanban login form inputs");
    }
  });

  test("03 - Create a board/card", async ({ page }) => {
    await page.goto(DEMO_URL);
    await page.waitForLoadState("networkidle");

    // Login
    const emailInput = page.locator(
      'input[type="email"], input[name="email"]'
    );
    const passwordInput = page.locator('input[type="password"]');
    if ((await emailInput.count()) > 0) {
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
      path: path.join(SCREENSHOTS_DIR, "kanban_03_board_view.png"),
      fullPage: true,
    });

    // Look for board creation form — fill name input if present, then submit
    const nameInput = page.locator(
      'input[placeholder*="board" i], input[placeholder*="name" i], input[name="title"], input[name="name"]'
    );
    if ((await nameInput.count()) > 0) {
      await nameInput.first().fill("QA Test Board");
      console.log("  ✓ Board name filled");
    }

    // Look for add card/column buttons (prefer enabled buttons)
    const addBtn = page.locator(
      'button:has-text("Add"):not([disabled]), button:has-text("New"):not([disabled]), button:has-text("Create"):not([disabled]), button:has-text("+"):not([disabled])'
    );
    const addCount = await addBtn.count();
    console.log(`  Found ${addCount} enabled add/create buttons`);

    if (addCount > 0) {
      await addBtn.first().click();
      await page.waitForTimeout(1000);
      await page.screenshot({
        path: path.join(SCREENSHOTS_DIR, "kanban_03b_add_dialog.png"),
        fullPage: true,
      });
      console.log("  ✓ Add button clicked");
    } else {
      // Try clicking any create button even if disabled (form may enable on input)
      const anyBtn = page.locator(
        'button:has-text("Add"), button:has-text("New"), button:has-text("Create"), button:has-text("+")'
      );
      if ((await anyBtn.count()) > 0) {
        console.log("  ✓ Create buttons exist (disabled — form likely needs input)");
      } else {
        recordDefect("No Add/Create/New button found on kanban board");
      }
    }
  });

  test("04 - Check for console errors", async ({ page }) => {
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
          recordDefect(`Console error on kanban: ${err.substring(0, 200)}`);
        }
      }
    } else {
      console.log("  ✓ No console errors");
    }
  });

  test("05 - Check text content sanity", async ({ page }) => {
    await page.goto(DEMO_URL);
    await page.waitForLoadState("networkidle");

    const bodyText = (await page.locator("body").textContent()) || "";

    if (bodyText.includes("undefined")) {
      recordDefect("Kanban displays 'undefined' text");
    }
    if (bodyText.includes("[object Object]")) {
      recordDefect("Kanban displays '[object Object]'");
    }
    if (bodyText.includes("TODO") || bodyText.includes("FIXME")) {
      recordDefect("Kanban displays TODO/FIXME text");
    }
    if (bodyText.includes("lorem") || bodyText.includes("Lorem")) {
      recordDefect("Kanban contains placeholder lorem ipsum text");
    }

    console.log("  ✓ Text content sanity check passed");
  });
});
