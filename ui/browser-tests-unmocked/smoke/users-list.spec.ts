import { test, expect, execSQL } from "../fixtures";

/**
 * SMOKE TEST: Users - List View
 *
 * Critical Path: Navigate to Users → Verify list loads with user data
 */

test.describe("Smoke: Users List", () => {
  test("seeded user renders in users list", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const testEmail = `seed-verify-${runId}@test.com`;

    // Arrange: seed a user via SQL
    await execSQL(
      request,
      adminToken,
      `INSERT INTO _ayb_users (email, password_hash) VALUES ('${testEmail}', '$argon2id$v=19$m=65536,t=3,p=4$dGVzdHNhbHQ$dGVzdGhhc2g') ON CONFLICT DO NOTHING;`,
    );

    // Act: navigate to Users page
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    const usersButton = page.locator("aside").getByRole("button", { name: /^Users$/i });
    await usersButton.click();
    await expect(page.getByRole("heading", { name: /Users/i })).toBeVisible({ timeout: 5000 });

    // Assert: seeded user email appears in the list
    await expect(page.getByText(testEmail).first()).toBeVisible({ timeout: 5000 });

    // Assert: search input is present (page fully loaded)
    await expect(page.getByPlaceholder(/search/i)).toBeVisible({ timeout: 3000 });

    // Cleanup
    await execSQL(request, adminToken, `DELETE FROM _ayb_users WHERE email = '${testEmail}';`);
  });
});
