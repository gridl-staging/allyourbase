import { test, expect, bootstrapMockedAdminApp, mockAdminApiKeyApis } from "./fixtures";

test.describe("API Keys Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("list-500: shows error state with retry button", async ({ page }) => {
    await mockAdminApiKeyApis(page, {
      listResponder: () => ({ status: 500, body: { message: "database connection failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /API Keys/i }).click();

    await expect(page.getByText(/database connection failed/i)).toBeVisible();
    await expect(page.getByRole("button", { name: /Retry/i })).toBeVisible();
  });

  test("create-400: shows error toast with validation message", async ({ page }) => {
    await mockAdminApiKeyApis(page, {
      createResponder: () => ({ status: 400, body: { message: "user not found" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /API Keys/i }).click();
    await expect(page.getByRole("heading", { name: /API Keys/i })).toBeVisible();

    await page.getByRole("button", { name: /Create Key/i }).click();
    await page.getByLabel(/Name/i).fill("Bad Key");
    await page.getByLabel(/User/i).selectOption("user-001");
    await page.getByRole("button", { name: "Create", exact: true }).click();

    const toast = page.getByTestId("toast").filter({ hasText: "user not found" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });

  test("revoke-500: shows error toast", async ({ page }) => {
    await mockAdminApiKeyApis(page, {
      revokeResponder: () => ({ status: 500, body: { message: "revoke failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /API Keys/i }).click();
    await expect(page.getByRole("cell", { name: "Service Key" })).toBeVisible();

    await page.getByRole("button", { name: "Revoke key" }).click();
    await page.getByRole("button", { name: "Revoke", exact: true }).click();

    const toast = page.getByTestId("toast").filter({ hasText: "revoke failed" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });

  test("empty-state: shows no keys message", async ({ page }) => {
    await mockAdminApiKeyApis(page, {
      listResponder: () => ({
        status: 200,
        body: { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 },
      }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /API Keys/i }).click();

    await expect(page.getByText(/No API keys created yet/i)).toBeVisible();
  });
});
