import { test, expect, seedFile, deleteFile, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Storage - Upload, Download, Delete
 *
 * Critical Path: Navigate to Storage → Upload file → Verify in list → Delete
 */

function untrackSeededFile(seededFileNames: string[], fileName: string): void {
  const trackedIndex = seededFileNames.indexOf(fileName);
  if (trackedIndex >= 0) {
    seededFileNames.splice(trackedIndex, 1);
  }
}

test.describe("Smoke: Storage", () => {
  const seededFileNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededFileNames.length > 0) {
      const fileName = seededFileNames.pop();
      if (!fileName) continue;
      await deleteFile(request, adminToken, "default", fileName).catch(
        () => {},
      );
    }
  });

  test("seeded file renders in storage list", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const fileName = `seed-verify-${runId}.txt`;
    seededFileNames.push(fileName);

    // Arrange: seed a file via API
    try {
      await seedFile(
        request,
        adminToken,
        "default",
        fileName,
        "seed verify content",
      );
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      test.skip(msg.includes("415") || msg.includes("Unsupported"), `Storage upload not available: ${msg}`);
      return;
    }

    // Act: navigate to Storage page
    await page.goto("/admin/");
    await waitForDashboard(page);
    const storageButton = page
      .locator("aside")
      .getByRole("button", { name: /^Storage$/i });
    await storageButton.click();
    await expect(
      page.getByRole("button", { name: "Upload", exact: true }),
    ).toBeVisible({ timeout: 5000 });

    // Assert: seeded file name appears in the list
    await expect(page.getByText(fileName).first()).toBeVisible({
      timeout: 5000,
    });
  });

  test("upload file and delete via storage UI", async ({ page, request, adminToken }) => {
    // Skip if storage uploads aren't available (415 Unsupported Media Type).
    try {
      const probeFile = await seedFile(request, adminToken, "default", `probe-${Date.now()}.txt`, "probe");
      await deleteFile(request, adminToken, "default", probeFile.name).catch(() => {});
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      test.skip(msg.includes("415") || msg.includes("Unsupported"), `Storage upload not available: ${msg}`);
      return;
    }

    const runId = Date.now();

    // Step 1: Navigate to admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    // Step 2: Navigate to Storage section
    const storageButton = page
      .locator("aside")
      .getByRole("button", { name: /^Storage$/i });
    await expect(storageButton).toBeVisible({ timeout: 5000 });
    await storageButton.click();

    // Step 3: Wait for storage view to load
    const uploadButton = page.getByRole("button", {
      name: "Upload",
      exact: true,
    });
    await expect(uploadButton).toBeVisible({ timeout: 5000 });

    // Step 4: Upload a file via the hidden file input
    const fileName = `smoke-test-${runId}.txt`;
    seededFileNames.push(fileName);
    const fileInput = page.locator('input[type="file"]');

    // Wait for any upload processing by listening for network response
    const uploadPromise = page.waitForResponse(
      (resp) =>
        resp.url().includes("/api/storage/") &&
        resp.request().method() === "POST",
      { timeout: 15000 },
    );

    await fileInput.setInputFiles({
      name: fileName,
      mimeType: "text/plain",
      buffer: Buffer.from("This is a smoke test file for upload and delete"),
    });

    // Wait for upload to complete
    await uploadPromise;

    // Step 5: Verify file appears in the list
    await expect(page.getByText(fileName)).toBeVisible({ timeout: 10000 });

    // Step 7: Delete the file
    const fileRow = page.locator("tr").filter({ hasText: fileName }).first();
    await fileRow.getByRole("button", { name: "Delete" }).click();

    // Step 8: Confirm deletion
    await expect(page.getByText("Are you sure")).toBeVisible({ timeout: 3000 });
    await page
      .getByRole("button", { name: "Delete", exact: true })
      .last()
      .click();

    // Step 9: Verify file removed from table (scope to row to avoid toast/dialog text)
    await expect(
      page.locator("tr").filter({ hasText: fileName }),
    ).not.toBeVisible({ timeout: 5000 });
    untrackSeededFile(seededFileNames, fileName);
  });
});
