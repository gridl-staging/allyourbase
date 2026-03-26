import { test, expect, bootstrapMockedAdminApp, mockAdminStorageApis } from "./fixtures";

test.describe("Storage Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("list-500: shows inline error", async ({ page }) => {
    await mockAdminStorageApis(page, {
      listResponder: () => ({ status: 500, body: { message: "storage backend unavailable" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Storage$/i }).click();

    await expect(page.getByText(/storage backend unavailable/i)).toBeVisible();
  });

  test("delete-500: shows error toast", async ({ page }) => {
    await mockAdminStorageApis(page, {
      deleteResponder: () => ({ status: 500, body: { message: "delete failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Storage$/i }).click();
    await expect(page.getByText("report.pdf")).toBeVisible();

    await page.getByRole("button", { name: /Delete/i }).click();
    await page.getByRole("button", { name: "Delete", exact: true }).last().click();

    const toast = page.getByTestId("toast").filter({ hasText: "delete failed" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });

  test("upload-500: shows upload failure toast", async ({ page }) => {
    await mockAdminStorageApis(page, {
      uploadResponder: () => ({ status: 500, body: { message: "upload failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Storage$/i }).click();

    await page.getByLabel(/Upload file/i).setInputFiles({
      name: "broken.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("test upload body"),
    });

    const toast = page.getByTestId("toast").filter({ hasText: "upload failed" });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).toHaveClass(/bg-red-50/);
  });

  test("blank-bucket: prompts for bucket name", async ({ page }) => {
    await mockAdminStorageApis(page);
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Storage$/i }).click();

    await page.getByLabel(/Bucket name/i).fill("   ");

    await expect(page.getByText(/Enter a bucket name to browse/i)).toBeVisible();
    await expect(page.getByRole("button", { name: /Upload/i })).toBeDisabled();
  });

  test("empty-bucket: shows no files message", async ({ page }) => {
    await mockAdminStorageApis(page, {
      listResponder: () => ({ status: 200, body: { items: [], totalItems: 0 } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Storage$/i }).click();

    await expect(page.getByText(/No files in/i)).toBeVisible();
  });

  test("404 on list: treats as empty bucket", async ({ page }) => {
    await mockAdminStorageApis(page, {
      listResponder: () => ({ status: 404, body: { message: "bucket not found" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Storage$/i }).click();

    // 404 is treated as empty — no error shown, just empty file list
    await expect(page.getByText(/No files in/i)).toBeVisible();
  });
});
