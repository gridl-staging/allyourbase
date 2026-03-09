import { test, expect } from "@playwright/test";
import {
  uniqueEmail,
  TEST_PASSWORD,
  DEMO_ACCOUNTS,
  registerUser,
  loginUser,
  loginWithDemoAccount,
} from "./helpers";

test.describe("Authentication", () => {
  test("shows login form by default", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.getByRole("heading", { name: "Live Polls" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
    await expect(page.getByPlaceholder("Email")).toBeVisible();
    await expect(page.getByPlaceholder("Password")).toBeVisible();
  });

  test("shows demo accounts on login page", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByText("Demo accounts")).toBeVisible();
    for (const acct of DEMO_ACCOUNTS) {
      await expect(page.getByText(acct.email)).toBeVisible();
    }
  });

  test("can toggle between login and register", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    await page.getByRole("button", { name: "Register" }).click();
    await expect(
      page.getByRole("button", { name: "Create Account" }),
    ).toBeVisible();

    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
  });

  test("can register a new user", async ({ page }) => {
    const email = await registerUser(page);
    await expect(
      page.getByRole("heading", { name: "Live Polls" }),
    ).toBeVisible();
    await expect(page.getByText("Sign out")).toBeVisible();
    // Logged-in user's email should be visible in the header.
    await expect(page.getByTestId("user-email")).toHaveText(email);
  });

  test("can login with demo account", async ({ page }) => {
    await loginWithDemoAccount(page);
    await expect(
      page.getByRole("heading", { name: "Live Polls" }),
    ).toBeVisible();
    await expect(page.getByText("Sign out")).toBeVisible();
  });

  test("clicking demo account fills credentials", async ({ page }) => {
    await page.goto("/");
    const acct = DEMO_ACCOUNTS[0];
    await page.getByText(acct.email).click();

    // Verify fields were filled.
    await expect(page.getByPlaceholder("Email")).toHaveValue(acct.email);
    await expect(page.getByPlaceholder("Password")).toHaveValue(acct.password);
  });

  test("can login with existing credentials", async ({ page }) => {
    // Register first.
    const email = await registerUser(page);

    // Logout.
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Login.
    await loginUser(page, email);
    await expect(
      page.getByRole("heading", { name: "Live Polls" }),
    ).toBeVisible();
  });

  test("shows error for invalid credentials", async ({ page }) => {
    await page.goto("/");
    await page.getByPlaceholder("Email").fill("wrong@example.com");
    await page.getByPlaceholder("Password").fill("wrongpassword");
    await page.getByRole("button", { name: "Sign In" }).click();

    // Should show an error message.
    await expect(page.getByText(/wrong|invalid|error|failed/i)).toBeVisible({
      timeout: 5000,
    });
  });

  test("can logout", async ({ page }) => {
    await loginWithDemoAccount(page);
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
  });

  test("persists auth across page reload", async ({ page }) => {
    await loginWithDemoAccount(page);
    await page.reload();
    // Should still be on the main UI (not the login form).
    // Check for "Sign out" (unique to main UI — heading "Live Polls" appears on both auth form and main UI).
    await expect(page.getByText("Sign out")).toBeVisible({ timeout: 10000 });
  });

  test("shows error when registering with duplicate email", async ({
    page,
  }) => {
    const email = await registerUser(page);

    // Logout.
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Try to register again with same email.
    await page.getByRole("button", { name: "Register" }).click();
    await page.getByPlaceholder("Email").fill(email);
    await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
    await page.getByRole("button", { name: "Create Account" }).click();

    // Should show an error.
    await expect(
      page.getByText(/already|exists|duplicate|taken/i),
    ).toBeVisible({ timeout: 5000 });
  });

  test("can login, logout, then login again", async ({ page }) => {
    const email = await registerUser(page);

    // Logout.
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Login again.
    await loginUser(page, email);
    await expect(page.getByText("Sign out")).toBeVisible();

    // Logout again.
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Login one more time.
    await loginUser(page, email);
    await expect(page.getByText("Sign out")).toBeVisible();
  });

  test("login page subtitle matches mode", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.getByText("Sign in to create and vote on polls"),
    ).toBeVisible();

    await page.getByRole("button", { name: "Register" }).click();
    await expect(page.getByText("Create your account")).toBeVisible();
  });
});
