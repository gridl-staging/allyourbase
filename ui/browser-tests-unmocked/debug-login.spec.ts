import { test, expect } from "@playwright/test";

test.use({ baseURL: "http://localhost:8090" });

test("debug admin login flow", async ({ page }) => {
  // Capture console logs
  const consoleLogs: string[] = [];
  page.on("console", (msg) => {
    const text = `[${msg.type()}] ${msg.text()}`;
    consoleLogs.push(text);
    console.log(text);
  });

  // Capture network requests
  const requests: Array<{ url: string; method: string; status?: number; body?: any }> = [];

  page.on("request", (request) => {
    requests.push({
      url: request.url(),
      method: request.method(),
    });
  });

  page.on("response", async (response) => {
    const req = requests.find((r) => r.url === response.url() && !r.status);
    if (req) {
      req.status = response.status();
      console.log(`[NETWORK] ${req.method} ${req.url} -> ${req.status}`);

      // Log response body for API calls
      if (req.url.includes("/api/")) {
        try {
          const body = await response.json();
          req.body = body;
          console.log(`[RESPONSE] ${req.url}:`, JSON.stringify(body, null, 2));
        } catch (e) {
          // Not JSON
        }
      }
    }
  });

  // Navigate to admin login
  console.log("\n=== Navigating to /admin/ ===");
  await page.goto("/admin/");

  // Wait for login form
  await expect(page.getByText("Enter the admin password")).toBeVisible({
    timeout: 10000,
  });
  console.log("\n=== Login form visible ===");

  // Enter admin password
  const adminPassword = process.env.AYB_ADMIN_PASSWORD || "admin";
  console.log(`\n=== Entering password: ${adminPassword} ===`);
  await page.getByLabel("Password").fill(adminPassword);

  // Click Sign in
  console.log("\n=== Clicking Sign in button ===");
  await page.getByRole("button", { name: "Sign in" }).click();

  // Wait a bit to see what happens
  console.log("\n=== Waiting 5 seconds to observe behavior ===");
  await page.waitForTimeout(5000);

  // Check current URL
  const currentUrl = page.url();
  console.log(`\n=== Current URL: ${currentUrl} ===`);

  // Check localStorage
  const localStorage = await page.evaluate(() => {
    return {
      token: window.localStorage.getItem("ayb_admin_token"),
      allKeys: Object.keys(window.localStorage),
      allItems: { ...window.localStorage },
    };
  });
  console.log(`\n=== localStorage ===`, JSON.stringify(localStorage, null, 2));

  // Check if still on login page
  const isStillOnLogin = await page
    .getByLabel("Password")
    .isVisible()
    .catch(() => false);
  console.log(`\n=== Still on login page: ${isStillOnLogin} ===`);

  // Check for error messages
  const errorElement = page.locator('.bg-red-50, [role="alert"], .text-red-700');
  const hasError = await errorElement.isVisible().catch(() => false);
  if (hasError) {
    const errorText = await errorElement.textContent();
    console.log(`\n=== ERROR MESSAGE: ${errorText} ===`);
  }

  // Check for navigation element
  const hasNav = await page
    .getByRole("navigation")
    .isVisible()
    .catch(() => false);
  console.log(`\n=== Has navigation element: ${hasNav} ===`);

  // Check for any text that might indicate we're on dashboard
  const bodyText = await page.locator("body").textContent();
  console.log(`\n=== Page contains "SQL Editor": ${bodyText?.includes("SQL Editor")} ===`);
  console.log(`\n=== Page contains "Loading": ${bodyText?.includes("Loading")} ===`);

  // Take a screenshot
  await page.screenshot({ path: "debug-login.png", fullPage: true });
  console.log(`\n=== Screenshot saved to debug-login.png ===`);

  // Print summary of network requests
  console.log("\n=== Network Requests Summary ===");
  requests.forEach((req) => {
    console.log(`${req.method} ${req.url} -> ${req.status || "pending"}`);
  });

  console.log("\n=== Test complete (no assertions, just debugging) ===");
});
