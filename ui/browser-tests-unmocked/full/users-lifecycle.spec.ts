import {
  test,
  expect,
  probeEndpoint,
  ensureUserByEmail,
  cleanupUserByEmail,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Users Lifecycle
 *
 * Critical Path: Seed user → verify in list → search → delete via UI
 */

test.describe("Users Lifecycle (Full E2E)", () => {
  const userEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (userEmails.length > 0) {
      const email = userEmails.pop();
      if (!email) continue;
      await cleanupUserByEmail(request, adminToken, email).catch(() => {});
    }
  });

  test("load-and-verify seeded user, search, and delete via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/users/");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Users service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const seededEmail = `user-full-seeded-${runId}@example.test`;
    const deletableEmail = `user-full-delete-${runId}@example.test`;

    await ensureUserByEmail(request, adminToken, seededEmail);
    userEmails.push(seededEmail);
    await ensureUserByEmail(request, adminToken, deletableEmail);
    userEmails.push(deletableEmail);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Users$/i }).click();
    await expect(page.getByRole("heading", { name: /^Users$/i })).toBeVisible({ timeout: 5000 });

    // Search for the seeded user
    const searchInput = page.getByPlaceholder("Search by email...");
    await searchInput.fill(seededEmail);
    await page.getByRole("button", { name: /Search/i }).click();

    const seededRow = page.getByRole("row", { name: new RegExp(seededEmail) }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });

    // Clear search and find the deletable user
    await searchInput.clear();
    await searchInput.fill(deletableEmail);
    await page.getByRole("button", { name: /Search/i }).click();

    const deletableRow = page.getByRole("row", { name: new RegExp(deletableEmail) }).first();
    await expect(deletableRow).toBeVisible({ timeout: 5000 });

    // Delete the user via UI
    await deletableRow.getByRole("button", { name: /Delete/i }).click();
    await expect(page.getByText(/Delete User/i)).toBeVisible({ timeout: 5000 });
    await page.getByRole("button", { name: /^Delete$/i }).click();

    await expect(page.getByText(new RegExp(`User ${deletableEmail} deleted`, "i"))).toBeVisible({ timeout: 5000 });

    // Remove from cleanup list since it was deleted via UI
    const idx = userEmails.indexOf(deletableEmail);
    if (idx !== -1) userEmails.splice(idx, 1);
  });
});
