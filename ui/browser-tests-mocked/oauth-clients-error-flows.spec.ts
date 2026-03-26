import { test, expect, bootstrapMockedAdminApp, mockAdminOAuthClientApis } from "./fixtures";

test.describe("OAuth Clients Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("list-500: shows error state with retry button", async ({ page }) => {
    await mockAdminOAuthClientApis(page, {
      listResponder: () => ({ status: 500, body: { message: "database connection failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /OAuth Clients/i }).click();

    await expect(page.getByText(/database connection failed/i)).toBeVisible();
    await expect(page.getByRole("button", { name: /Retry/i })).toBeVisible();
  });

  test("create-400: shows error toast with validation message", async ({ page }) => {
    await mockAdminOAuthClientApis(page, {
      createResponder: () => ({ status: 400, body: { message: "invalid redirect URI" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /OAuth Clients/i }).click();
    await expect(page.getByRole("heading", { name: /OAuth Clients/i })).toBeVisible();

    await page.getByRole("button", { name: /Register Client/i }).click();
    await page.getByLabel(/Client name/i).fill("Bad Client");
    await page.getByLabel("App").selectOption("app-001");
    await page.getByRole("button", { name: "Register", exact: true }).click();

    const toast = page.getByTestId("toast").filter({ hasText: "invalid redirect URI" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });

  test("revoke-500: shows error toast", async ({ page }) => {
    await mockAdminOAuthClientApis(page, {
      revokeResponder: () => ({ status: 500, body: { message: "revoke failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /OAuth Clients/i }).click();
    await expect(page.getByRole("cell", { name: "Portal Client" })).toBeVisible();

    await page.getByRole("button", { name: "Revoke client" }).click();
    await page.getByRole("button", { name: "Revoke", exact: true }).click();

    const toast = page.getByTestId("toast").filter({ hasText: "revoke failed" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });

  test("rotate-secret-500: shows error toast", async ({ page }) => {
    await mockAdminOAuthClientApis(page, {
      rotateResponder: () => ({ status: 500, body: { message: "rotation failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /OAuth Clients/i }).click();
    await expect(page.getByRole("cell", { name: "Portal Client" })).toBeVisible();

    await page.getByRole("button", { name: "Rotate secret" }).click();
    await page.getByRole("button", { name: "Rotate", exact: true }).click();

    const toast = page.getByTestId("toast").filter({ hasText: "rotation failed" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });

  test("empty-state: shows no clients message", async ({ page }) => {
    await mockAdminOAuthClientApis(page, {
      listResponder: () => ({
        status: 200,
        body: { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 },
      }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /OAuth Clients/i }).click();

    await expect(page.getByText(/No OAuth clients registered yet/i)).toBeVisible();
  });
});
