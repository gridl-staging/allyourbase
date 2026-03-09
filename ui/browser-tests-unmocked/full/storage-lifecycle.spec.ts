import { test, expect, seedFile, deleteFile } from "../fixtures";

/**
 * FULL E2E TEST: Storage Lifecycle
 *
 * Tests complete storage management:
 * - Upload text file and image file
 * - Verify files appear in list
 * - Preview image file
 * - Generate signed URL
 * - Download file
 * - Delete files
 */

test.describe("Storage Lifecycle (Full E2E)", () => {
  // Track files created during tests for cleanup on failure
  const pendingFileCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const fileName of pendingFileCleanup) {
      await deleteFile(request, adminToken, "default", fileName).catch(() => {});
    }
    pendingFileCleanup.length = 0;
  });

  test("seeded file renders in storage list", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const fileName = `lifecycle-verify-${runId}.txt`;

    // Register cleanup early so afterEach runs it even on failure
    pendingFileCleanup.push(fileName);

    // Arrange: seed a file via API
    await seedFile(request, adminToken, "default", fileName, "lifecycle verify content");

    // Act: navigate to Storage page
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    const storageButton = page.locator("aside").getByRole("button", { name: /^Storage$/i });
    await storageButton.click();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible({ timeout: 5000 });

    // Assert: seeded file name appears in the list
    await expect(page.getByText(fileName).first()).toBeVisible({ timeout: 5000 });

    // Cleanup handled by afterEach
  });

  test("upload, preview, signed URL, download, and delete files", async ({ page }) => {
    const runId = Date.now();
    const textFileName = `lifecycle-text-${runId}.txt`;
    const imgFileName = `lifecycle-img-${runId}.png`;

    // Register cleanup early so afterEach removes files if test fails partway
    pendingFileCleanup.push(textFileName, imgFileName);

    // ============================================================
    // Setup: Navigate to Storage
    // ============================================================
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    const storageButton = page.locator("aside").getByRole("button", { name: /^Storage$/i });
    await expect(storageButton).toBeVisible({ timeout: 5000 });
    await storageButton.click();

    // Wait for storage view
    const uploadButton = page.getByRole("button", { name: "Upload", exact: true });
    await expect(uploadButton).toBeVisible({ timeout: 5000 });

    // ============================================================
    // UPLOAD: Text file
    // ============================================================
    const fileInput = page.locator('input[type="file"]');
    await fileInput.setInputFiles({
      name: textFileName,
      mimeType: "text/plain",
      buffer: Buffer.from("Storage lifecycle test content"),
    });

    // Verify text file appears in the list
    await expect(page.getByText(textFileName)).toBeVisible({ timeout: 10000 });

    // ============================================================
    // UPLOAD: Image file (1x1 red PNG)
    // ============================================================
    // Minimal valid 1x1 red PNG
    const pngBuffer = Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg==",
      "base64"
    );
    await fileInput.setInputFiles({
      name: imgFileName,
      mimeType: "image/png",
      buffer: pngBuffer,
    });

    // Verify image file uploaded
    await expect(page.getByText(imgFileName)).toBeVisible({ timeout: 10000 });

    // ============================================================
    // PREVIEW: Preview the image file
    // ============================================================
    const imgRow = page.locator("tr").filter({ hasText: imgFileName }).first();
    const previewButton = imgRow.getByRole("button", { name: "Preview" });

    await expect(previewButton).toBeVisible({ timeout: 2000 });
    await previewButton.click();

    // Verify preview modal shows image
    await expect(page.getByRole("img").first()).toBeVisible({ timeout: 3000 });

    // Close preview
    const closePreviewBtn = page.getByRole("button", { name: "Close" });
    await expect(closePreviewBtn.first()).toBeVisible({ timeout: 1000 });
    await closePreviewBtn.first().click();

    // ============================================================
    // SIGNED URL: Generate signed URL for text file
    // ============================================================
    const textRow = page.locator("tr").filter({ hasText: textFileName }).first();
    const signedUrlButton = textRow.getByRole("button", { name: "Copy signed URL" });

    await expect(signedUrlButton).toBeVisible({ timeout: 2000 });
    await signedUrlButton.click();

    // Verify signed URL copy toast
    await expect(page.getByText(/copied/i).first()).toBeVisible({ timeout: 3000 });

    // ============================================================
    // DOWNLOAD: Verify download link exists
    // ============================================================
    const downloadLink = textRow.getByRole("link", { name: "Download" });
    await expect(downloadLink).toBeVisible({ timeout: 2000 });

    // ============================================================
    // DELETE: Remove text file
    // ============================================================
    const deleteTextBtn = textRow.getByRole("button", { name: "Delete" });
    await expect(deleteTextBtn).toBeVisible({ timeout: 3000 });
    await deleteTextBtn.click();

    // Wait for "Delete File" confirmation dialog
    await expect(page.getByText("Are you sure")).toBeVisible({ timeout: 3000 });
    // Click the dialog's Delete button
    await page.getByRole("button", { name: "Delete", exact: true }).last().click();

    // Verify text file removed from table (use row selector to avoid matching toast text)
    await expect(page.locator("tr").filter({ hasText: textFileName })).not.toBeVisible({ timeout: 5000 });

    // ============================================================
    // DELETE: Remove image file
    // ============================================================
    const imgRow2 = page.locator("tr").filter({ hasText: imgFileName }).first();
    const deleteImgBtn = imgRow2.getByRole("button", { name: "Delete" });
    await expect(deleteImgBtn).toBeVisible({ timeout: 3000 });
    await deleteImgBtn.click();

    // Wait for confirmation dialog and confirm
    await expect(page.getByText("Are you sure")).toBeVisible({ timeout: 3000 });
    await page.getByRole("button", { name: "Delete", exact: true }).last().click();

    // Verify image file removed from table (use row selector to avoid matching toast text)
    await expect(page.locator("tr").filter({ hasText: imgFileName })).not.toBeVisible({ timeout: 5000 });

  });
});
