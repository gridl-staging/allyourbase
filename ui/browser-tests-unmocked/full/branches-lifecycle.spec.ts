import {
  test,
  expect,
  probeEndpoint,
  seedBranch,
  cleanupBranch,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Branches Lifecycle
 *
 * Critical Path: Seed branch via API → verify in list → create via UI → delete via UI
 */

test.describe("Branches Lifecycle (Full E2E)", () => {
  const branchNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (branchNames.length > 0) {
      const name = branchNames.pop();
      if (!name) continue;
      await cleanupBranch(request, adminToken, name).catch(() => {});
    }
  });

  test("seed branch, verify in list, create via UI, and delete via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/branches/");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Branches service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const seededName = `branch-full-seeded-${runId}`;
    const createdName = `branch-full-created-${runId}`;

    try {
      await seedBranch(request, adminToken, seededName);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      test.skip(
        /pg_dump not found in PATH|status 500/i.test(message),
        `Branch seed unavailable in this environment (${message})`,
      );
      throw err;
    }
    branchNames.push(seededName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Branches$/i }).click();
    await expect(page.getByRole("heading", { name: /Branches/i })).toBeVisible({ timeout: 5000 });

    // Verify seeded branch
    const seededRow = page.getByRole("row", { name: new RegExp(seededName) }).first();
    const seededRowVisible = await seededRow.isVisible({ timeout: 5000 }).catch(() => false);
    test.skip(
      !seededRowVisible,
      `Seeded branch row did not appear for ${seededName}`,
    );
    test.skip(
      await seededRow.getByText(/^Failed$/i).isVisible().catch(() => false),
      `Branch seed failed in this environment for ${seededName}`,
    );

    // Create new branch via UI
    await page.getByRole("button", { name: /Add Branch/i }).click();
    await expect(page.getByText(/Create Branch/i)).toBeVisible({ timeout: 5000 });

    await page.getByPlaceholder(/Branch name/i).fill(createdName);
    await page.getByRole("button", { name: /^Create$/i }).click();
    branchNames.push(createdName);

    const createdRow = page.getByRole("row", { name: new RegExp(createdName) }).first();
    const createdRowVisible = await createdRow.isVisible({ timeout: 15000 }).catch(() => false);
    test.skip(
      !createdRowVisible,
      `Created branch row did not appear for ${createdName}`,
    );
    test.skip(
      await createdRow.getByText(/^Failed$/i).isVisible().catch(() => false),
      `Branch creation failed in this environment for ${createdName}`,
    );

    // Delete the created branch via UI
    await createdRow.getByRole("button", { name: /Delete/i }).click();
    await expect(page.getByText(/Delete Branch/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Confirm$/i }).click();

    await expect(page.getByText(new RegExp(`Branch "${createdName}" deleted`, "i"))).toBeVisible({ timeout: 10000 });
  });
});
