import { test, expect } from "@playwright/test";
import { uniqueEmail, TEST_PASSWORD, registerUser, loginUser } from "./helpers";

test.describe("Authentication", () => {
  test("shows login form by default", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Kanban Board" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
    await expect(page.getByPlaceholder("you@example.com")).toBeVisible();
    await expect(
      page.getByPlaceholder("At least 8 characters"),
    ).toBeVisible();
  });

  test("can toggle between login and register", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    await page.getByRole("button", { name: "Sign up" }).click();
    await expect(
      page.getByRole("button", { name: "Create Account" }),
    ).toBeVisible();

    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
  });

  test("can register a new user", async ({ page }) => {
    const email = await registerUser(page);
    // After registration, we should see the board list
    await expect(page.getByText("Your Boards")).toBeVisible();
    await expect(page.getByText("Sign out")).toBeVisible();
    // Logged-in user's email should be visible in the header.
    await expect(page.getByTestId("user-email")).toHaveText(email);
  });

  test("can login with existing credentials", async ({ page }) => {
    // First register
    const email = await registerUser(page);

    // Logout
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Login
    await page.getByPlaceholder("you@example.com").fill(email);
    await page.getByPlaceholder("At least 8 characters").fill(TEST_PASSWORD);
    await page.getByRole("button", { name: "Sign In" }).click();
    await expect(page.getByText("Your Boards")).toBeVisible();
  });

  test("shows error for invalid credentials", async ({ page }) => {
    await page.goto("/");
    await page.getByPlaceholder("you@example.com").fill("wrong@example.com");
    await page.getByPlaceholder("At least 8 characters").fill("wrongpassword");
    await page.getByRole("button", { name: "Sign In" }).click();

    // Should show an error message (role="alert" on the error element)
    await expect(page.getByRole("alert")).toBeVisible({ timeout: 5000 });
  });

  test("can logout", async ({ page }) => {
    await registerUser(page);
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
  });

  test("persists auth across page reload", async ({ page }) => {
    await registerUser(page);
    await page.reload();
    // Should still be on the board list (not the login form)
    await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
  });

  test("clears error message when toggling between login and register", async ({ page }) => {
    await page.goto("/");

    // Trigger an error by submitting bad credentials
    await page.getByPlaceholder("you@example.com").fill("wrong@example.com");
    await page.getByPlaceholder("At least 8 characters").fill("wrongpassword");
    await page.getByRole("button", { name: "Sign In" }).click();
    await expect(page.getByRole("alert")).toBeVisible({ timeout: 5000 });

    // Toggle to register mode — error should disappear
    await page.getByRole("button", { name: "Sign up" }).click();
    await expect(page.getByRole("alert")).toBeHidden();
  });

  test("shows error when registering with duplicate email", async ({ page }) => {
    const email = await registerUser(page);

    // Logout
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Try to register again with same email
    await page.getByRole("button", { name: "Sign up" }).click();
    await page.getByPlaceholder("you@example.com").fill(email);
    await page.getByPlaceholder("At least 8 characters").fill(TEST_PASSWORD);
    await page.getByRole("button", { name: "Create Account" }).click();

    // Should show an error
    await expect(page.getByRole("alert")).toBeVisible({ timeout: 5000 });
  });

  test("can login, logout, then login again", async ({ page }) => {
    const email = await registerUser(page);
    await expect(page.getByText("Your Boards")).toBeVisible();

    // Logout
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Login again
    await loginUser(page, email);
    await expect(page.getByText("Your Boards")).toBeVisible();

    // Logout again
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Login one more time
    await loginUser(page, email);
    await expect(page.getByText("Your Boards")).toBeVisible();
  });
});
