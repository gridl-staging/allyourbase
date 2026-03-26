import { test, expect, bootstrapMockedAdminApp, mockAdminAppsApis } from "./fixtures";

test.describe("Apps Toast Outcomes (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("load-and-verify: seeded apps render in default list view", async ({ page }) => {
    await mockAdminAppsApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Apps$/i }).click();

    await expect(page.getByRole("heading", { name: "Applications" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "Starter App" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("owner@example.com")).toBeVisible();
  });

  test("create app success: mocked 201 response shows success toast", async ({ page }) => {
    const apis = await mockAdminAppsApis(page);

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Apps$/i }).click();
    await expect(page.getByRole("heading", { name: "Applications" })).toBeVisible();

    await page.getByRole("button", { name: "Create App" }).click();
    await expect(page.getByRole("heading", { name: "Create Application" })).toBeVisible();

    await page.getByLabel("App name").fill("Alpha App");
    await page.getByLabel("Description").fill("Created from browser mocked test");
    await page.getByLabel("Owner").selectOption("user-001");
    await page.getByRole("button", { name: "Create", exact: true }).click();

    await expect.poll(() => apis.createAppCalls, { timeout: 5000 }).toBe(1);
    expect(apis.lastCreateBody?.name).toBe("Alpha App");
    expect(apis.lastCreateBody?.ownerUserId).toBe("user-001");

    const toast = page.getByTestId("toast").filter({ hasText: 'App "Alpha App" created' });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-green-50/);

    await expect(page.getByRole("cell", { name: "Alpha App" })).toBeVisible({ timeout: 5000 });
  });

  test("create app error: mocked 400 response shows error toast", async ({ page }) => {
    const apis = await mockAdminAppsApis(page, {
      createResponder: () => ({
        status: 400,
        body: { message: "owner user not found" },
      }),
    });

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Apps$/i }).click();
    await expect(page.getByRole("heading", { name: "Applications" })).toBeVisible();

    await page.getByRole("button", { name: "Create App" }).click();
    await expect(page.getByRole("heading", { name: "Create Application" })).toBeVisible();

    await page.getByLabel("App name").fill("Broken App");
    await page.getByLabel("Description").fill("This should fail");
    await page.getByLabel("Owner").selectOption("user-001");
    await page.getByRole("button", { name: "Create", exact: true }).click();

    await expect.poll(() => apis.createAppCalls, { timeout: 5000 }).toBe(1);

    const toast = page.getByTestId("toast").filter({ hasText: "owner user not found" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);

    await expect(page.getByRole("heading", { name: "Create Application" })).toBeVisible();
  });
});
