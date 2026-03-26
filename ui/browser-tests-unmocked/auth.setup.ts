/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_2_auth_jwt_and_private_function_proof/allyourbase_dev/ui/browser-tests-unmocked/auth.setup.ts.
 */
import { test as setup, expect } from "@playwright/test";
import { getBrowserUnmockedSkipReason } from "./browser-preflight";
import { resolveAdminBootstrapCredential } from "./admin-bootstrap";

/**
 * AUTH SETUP: Log into the admin dashboard and save auth state.
 *
 * All smoke and full test projects depend on this setup.
 * The saved storageState includes the JWT token in localStorage,
 * which is automatically loaded before each test.
 *
 * Admin auth resolution order:
 * 1. AYB_ADMIN_PASSWORD env var
 * 2. ~/.ayb/admin-token file (written by `ayb start`, usually a bearer token)
 */

const authFile = "browser-tests-unmocked/.auth/admin.json";
const browserSkipReason = getBrowserUnmockedSkipReason();

if (browserSkipReason) {
  setup.skip(true, browserSkipReason);
}

/**
 * TODO: Document bootstrapWithSavedToken.
 */
async function bootstrapWithSavedToken(page: import("@playwright/test").Page, token: string): Promise<boolean> {
  await page.goto("/admin/", { waitUntil: "domcontentloaded" });
  await page.evaluate((savedToken: string) => {
    window.localStorage.setItem("ayb_admin_token", savedToken);
  }, token);
  await page.goto("/admin/", { waitUntil: "domcontentloaded" });
  try {
    await expect(page.getByRole("navigation")).toBeVisible({ timeout: 5000 });
    return true;
  } catch {
    // If the saved file contains a legacy password rather than a bearer token,
    // clear the invalid token and continue with password-form login.
    await page.evaluate(() => {
      window.localStorage.removeItem("ayb_admin_token");
    });
    return false;
  }
}

/**
 * TODO: Document loginWithPassword.
 */
async function loginWithPassword(page: import("@playwright/test").Page, password: string): Promise<void> {
  await page.goto("/admin/", { waitUntil: "domcontentloaded" });

  // Wait for login form
  await expect(page.getByText("Enter the admin password")).toBeVisible({
    timeout: 15000,
  });

  // Enter admin password
  await page.getByLabel("Password").fill(password);

  // Click Sign in
  await page.getByRole("button", { name: "Sign in" }).click();

  // The admin SPA writes the JWT to localStorage immediately after the API
  // call succeeds (before the boot/schema-fetch cycle even starts), so the
  // token appearing in localStorage is the fastest reliable signal that login
  // worked.  The SPA never changes the URL from /admin/ — all routing is
  // client-side state — so a URL-based wait would hang forever.
  await page.waitForFunction(
    () => {
      const token = localStorage.getItem("ayb_admin_token");
      return token !== null && token.length > 0;
    },
    { timeout: 15000 },
  );

  // Wait for the dashboard to finish booting — the sidebar <nav> only renders
  // once the schema has loaded and Layout mounts.  We intentionally skip a
  // generic .bg-red-50 error check here: the token wait already proves login
  // succeeded, and the dashboard may legitimately show red-styled elements
  // (e.g. a table-browser query error) that are NOT login failures.
  await expect(page.getByRole("navigation")).toBeVisible({ timeout: 15000 });
}

setup("authenticate as admin", async ({ page }) => {
  const credential = resolveAdminBootstrapCredential();
  if (credential.source === "saved-admin-auth") {
    const didBootstrapFromToken = await bootstrapWithSavedToken(page, credential.value);
    if (!didBootstrapFromToken) {
      await loginWithPassword(page, credential.value);
    }
  } else {
    await loginWithPassword(page, credential.value);
  }

  // Save auth state (localStorage with JWT token)
  await page.context().storageState({ path: authFile });
});
