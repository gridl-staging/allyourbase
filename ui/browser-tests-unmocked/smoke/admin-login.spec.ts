import { test, expect } from "../fixtures";
import { readFileSync } from "fs";
import { join } from "path";
import { homedir } from "os";

/**
 * SMOKE TEST: Admin Dashboard - Login
 *
 * Critical Path: Admin enters password → Dashboard loads
 *
 * Note: This test uses its OWN storage state (no pre-auth)
 * since it tests the login flow itself.
 *
 * IMPORTANT: These tests are marked @slow and run serially
 * because they test the login flow which is rate-limited.
 * The auth.setup.ts already validates basic login functionality,
 * so these tests are supplementary.
 */

function resolveAdminPassword(): string {
  if (process.env.AYB_ADMIN_PASSWORD) {
    return process.env.AYB_ADMIN_PASSWORD;
  }
  try {
    const tokenPath = join(homedir(), ".ayb", "admin-token");
    return readFileSync(tokenPath, "utf-8").trim();
  } catch {
    return "admin";
  }
}

// Override storageState — login test must start unauthenticated
test.use({ storageState: { cookies: [], origins: [] } });

test.describe("Smoke: Admin Login", () => {
  // Run login tests serially to avoid rate limiting
  // Tag as @slow since they require pauses between tests
  test.describe.configure({ mode: "serial" });

  test("admin can log in with correct password", async ({ page, authStatus }) => {
    test.slow(); // Mark as slow test - needs extra time
    test.skip(!authStatus.auth, "admin.password not configured — no-auth mode");

    // Step 1: Navigate to admin dashboard with fresh page load
    await page.goto("/admin/", { waitUntil: "domcontentloaded" });

    // Step 2: Verify login form is shown
    await expect(page.getByText("Enter the admin password")).toBeVisible({ timeout: 15000 });

    // Step 3: Enter admin password (env var → ~/.ayb/admin-token → "admin")
    const adminPassword = resolveAdminPassword();
    const passwordInput = page.getByLabel("Password");
    await expect(passwordInput).toBeVisible({ timeout: 5000 });
    await passwordInput.fill(adminPassword);

    // Step 4: Click Sign in
    const signInButton = page.getByRole("button", { name: "Sign in" });
    await expect(signInButton).toBeVisible({ timeout: 5000 });
    await signInButton.click();

    // Step 5: Verify dashboard loads
    await expect(page.locator("aside").getByText("Allyourbase")).toBeVisible({ timeout: 15000 });
  });

  test("admin login rejects wrong password", async ({ page, authStatus }) => {
    test.slow(); // Mark as slow test - needs extra time
    test.skip(!authStatus.auth, "admin.password not configured — no-auth mode");

    // Step 1: Navigate to admin dashboard
    await page.goto("/admin/", { waitUntil: "domcontentloaded" });

    // Step 2: Wait for login form and enter wrong password
    const passwordInput = page.getByLabel("Password");
    await expect(passwordInput).toBeVisible({ timeout: 15000 });
    await passwordInput.fill("wrongpassword123");

    // Step 3: Click Sign in
    const signInButton = page.getByRole("button", { name: "Sign in" });
    await expect(signInButton).toBeVisible({ timeout: 5000 });
    await signInButton.click();

    // Step 4: Verify error message — either "invalid password" or "too many requests" (rate limiter)
    await expect(page.getByText(/invalid password|too many/i)).toBeVisible({ timeout: 5000 });

    // Step 5: Verify we're still on login form
    await expect(signInButton).toBeVisible({ timeout: 5000 });
  });
});
