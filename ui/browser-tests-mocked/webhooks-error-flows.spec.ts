import { test, expect, bootstrapMockedAdminApp, mockAdminWebhookApis } from "./fixtures";

test.describe("Webhooks Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("list-500: shows error state with retry button", async ({ page }) => {
    await mockAdminWebhookApis(page, {
      listResponder: () => ({ status: 500, body: { message: "database connection failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Webhooks$/i }).click();

    await expect(page.getByText(/database connection failed/i)).toBeVisible();
    await expect(page.getByRole("button", { name: /Retry/i })).toBeVisible();
  });

  test("delete-500: shows error toast", async ({ page }) => {
    await mockAdminWebhookApis(page, {
      deleteResponder: () => ({ status: 500, body: { message: "delete failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Webhooks$/i }).click();
    await expect(page.getByText("hooks.example.com/events")).toBeVisible();

    await page.getByRole("button", { name: /Delete/i }).click();
    await page.getByRole("button", { name: "Delete", exact: true }).last().click();

    const toast = page.getByTestId("toast").filter({ hasText: "delete failed" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });

  test("test-webhook failure: shows status-code failure toast", async ({ page }) => {
    await mockAdminWebhookApis(page, {
      testResponder: () => ({
        status: 200,
        body: { success: false, statusCode: 503, durationMs: 120, error: null },
      }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Webhooks$/i }).click();
    await expect(page.getByText("hooks.example.com/events")).toBeVisible();

    await page.getByRole("button", { name: /Test/i }).click();

    const toast = page.getByTestId("toast").filter({ hasText: "Test failed (503)" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });

  test("empty-state: shows no webhooks message", async ({ page }) => {
    await mockAdminWebhookApis(page, {
      listResponder: () => ({ status: 200, body: { items: [] } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Webhooks$/i }).click();

    await expect(page.getByText(/No webhooks configured yet/i)).toBeVisible();
  });
});
