import { test, expect, waitForDashboard } from "../fixtures";
import { resolveAdminPasswordForBrowserLogin } from "../admin-bootstrap";

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

// Override storageState — login test must start unauthenticated
test.use({ storageState: { cookies: [], origins: [] } });

test.describe("Smoke: Admin Login", () => {
  // Run login tests serially to avoid rate limiting
  // Tag as @slow since they require pauses between tests
  test.describe.configure({ mode: "serial" });

  test("admin can log in with correct password", async ({ page, request, authStatus }) => {
    test.slow(); // Mark as slow test - needs extra time
    test.skip(!authStatus.auth, "admin.password not configured — no-auth mode");

    // Step 1: Navigate to admin dashboard with fresh page load
    await page.goto("/admin/", { waitUntil: "domcontentloaded" });

    // Step 2: Verify login form is shown
    await expect(page.getByText("Enter the admin password")).toBeVisible({ timeout: 15000 });

    // Step 3: Enter an actual admin password. The saved admin auth file is
    // usually already a bearer token, so only use it here when the server
    // still accepts it as a legacy password value.
    const adminPassword = await resolveAdminPasswordForBrowserLogin(request);
    test.skip(
      adminPassword === null,
      "positive admin password login requires AYB_ADMIN_PASSWORD or a legacy password in ~/.ayb/admin-token",
    );
    const passwordInput = page.getByLabel("Password");
    await expect(passwordInput).toBeVisible({ timeout: 5000 });
    await passwordInput.fill(adminPassword);

    // Step 4: Click Sign in
    const signInButton = page.getByRole("button", { name: "Sign in" });
    await expect(signInButton).toBeVisible({ timeout: 5000 });
    await signInButton.click();

    // Step 5: Verify dashboard loads
    await waitForDashboard(page);
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
