import { test, expect, bootstrapMockedAdminApp, mockAdminSecretApis } from "./fixtures";

test.describe("Secrets Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("list-500: shows error text", async ({ page }) => {
    await mockAdminSecretApis(page, {
      listResponder: () => ({ status: 500, body: { message: "vault unreachable" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Secrets$/i }).click();

    await expect(page.getByText(/vault unreachable/i)).toBeVisible();
  });

  test("create-500: shows error", async ({ page }) => {
    await mockAdminSecretApis(page, {
      createResponder: () => ({ status: 500, body: { message: "encryption failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Secrets$/i }).click();
    await expect(page.getByText("DATABASE_URL")).toBeVisible();

    await page.getByRole("button", { name: /Create/i }).click();
    await page.getByLabel(/Name/i).fill("NEW_SECRET");
    await page.getByLabel(/Value/i).fill("secret-value");
    await page.getByRole("button", { name: "Create", exact: true }).click();

    await expect(page.getByText(/encryption failed/i)).toBeVisible();
  });

  test("rotate-500: shows error", async ({ page }) => {
    await mockAdminSecretApis(page, {
      rotateResponder: () => ({ status: 500, body: { message: "rotation failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Secrets$/i }).click();
    await expect(page.getByText("DATABASE_URL")).toBeVisible();

    await page.getByRole("button", { name: "Rotate JWT Secret" }).click();
    await page.getByRole("button", { name: "Rotate", exact: true }).click();

    await expect(page.getByText(/rotation failed/i)).toBeVisible();
  });

  test("empty-state: shows no secrets message", async ({ page }) => {
    await mockAdminSecretApis(page, {
      listResponder: () => ({ status: 200, body: [] }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Secrets$/i }).click();

    await expect(page.getByText(/No secrets configured/i)).toBeVisible();
  });
});
