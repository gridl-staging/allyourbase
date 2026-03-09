import { test as setup, expect } from "@playwright/test";
import { readFileSync } from "fs";
import { join } from "path";
import { homedir } from "os";

/**
 * AUTH SETUP: Log into the admin dashboard and save auth state.
 *
 * All smoke and full test projects depend on this setup.
 * The saved storageState includes the JWT token in localStorage,
 * which is automatically loaded before each test.
 *
 * Password resolution order:
 * 1. AYB_ADMIN_PASSWORD env var
 * 2. ~/.ayb/admin-token file (written by `ayb start`)
 */

const authFile = "browser-tests-unmocked/.auth/admin.json";

function resolveAdminPassword(): string {
  if (process.env.AYB_ADMIN_PASSWORD) {
    return process.env.AYB_ADMIN_PASSWORD;
  }
  try {
    const tokenPath = join(homedir(), ".ayb", "admin-token");
    return readFileSync(tokenPath, "utf-8").trim();
  } catch {
    throw new Error(
      "No admin password found. Either set AYB_ADMIN_PASSWORD or ensure `ayb start` is running (writes ~/.ayb/admin-token)."
    );
  }
}

setup("authenticate as admin", async ({ page }) => {
  // Navigate to admin login
  await page.goto("/admin/");

  // Wait for login form
  await expect(page.getByText("Enter the admin password")).toBeVisible({
    timeout: 15000,
  });

  // Enter admin password
  const adminPassword = resolveAdminPassword();
  console.log(`Using admin password: ${adminPassword}`);
  await page.getByLabel("Password").fill(adminPassword);

  // Click Sign in
  await page.getByRole("button", { name: "Sign in" }).click();

  // Wait a moment for any error messages to appear
  await page.waitForTimeout(1000);

  // Check for error messages
  const errorElement = page.locator('.bg-red-50, [role="alert"], .text-red-700');
  const hasError = await errorElement.isVisible().catch(() => false);
  if (hasError) {
    const errorText = await errorElement.textContent();
    throw new Error(`Login failed with error: ${errorText}`);
  }

  // Wait for dashboard to load — check for something that only exists on dashboard, not login page
  // The login page has "Allyourbase" too, so we need to check for actual dashboard content
  // Wait for URL to change from /admin/ to /admin/something or for dashboard-specific element
  await Promise.race([
    page.waitForURL(/\/admin\/.+/, { timeout: 10000 }),
    // Or wait for a dashboard-specific element like the sidebar nav
    expect(page.getByRole("navigation")).toBeVisible({ timeout: 10000 }),
  ]);

  // Additional verification: make sure we're NOT on the login page
  const isStillOnLogin = await page.getByLabel("Password").isVisible().catch(() => false);
  if (isStillOnLogin) {
    throw new Error("Login failed - still on login page");
  }

  // CRITICAL: Wait for localStorage to be populated with the admin token
  // The adminLogin() function calls setToken() which writes to localStorage
  await page.waitForFunction(
    () => {
      const token = localStorage.getItem("ayb_admin_token");
      return token !== null && token.length > 0;
    },
    { timeout: 5000 }
  );

  // Verify token is actually in localStorage
  const hasToken = await page.evaluate(() => {
    const token = localStorage.getItem("ayb_admin_token");
    console.log("Admin token in localStorage:", token?.substring(0, 20) + "...");
    return !!token;
  });

  if (!hasToken) {
    throw new Error("Admin token not found in localStorage after login");
  }

  // Save auth state (localStorage with JWT token)
  await page.context().storageState({ path: authFile });
});
