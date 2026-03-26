import { test, expect, probeEndpoint, fetchAuthSettings, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Auth Settings — Content-Verified
 *
 * Fetches the live auth settings via admin API, then asserts the rendered
 * toggle checkbox states match the actual boolean config values.
 */

const TOGGLE_KEYS = [
  "totp_enabled",
  "anonymous_auth_enabled",
  "email_mfa_enabled",
  "sms_enabled",
  "magic_link_enabled",
] as const;

test.describe("Smoke: Auth Settings", () => {
  test("auth settings toggles reflect live API config state", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth-settings");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Auth settings service unavailable (status ${probeStatus})`,
    );

    // Arrange: fetch live auth settings via fixture helper
    const settings = await fetchAuthSettings(request, adminToken);

    // Act: navigate to Auth Settings
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Auth Settings$/i }).click();
    await expect(page.getByRole("heading", { name: /Auth Settings/i })).toBeVisible({ timeout: 15_000 });

    // Assert: each toggle checkbox state matches the API boolean
    for (const key of TOGGLE_KEYS) {
      const toggle = page.getByTestId(`toggle-${key}`);
      await expect(toggle).toBeVisible({ timeout: 3000 });
      await expect(toggle).toBeChecked({ checked: Boolean(settings[key]) });
    }

    // Assert: OAuth Providers section renders
    await expect(page.getByRole("heading", { name: /OAuth Providers/i })).toBeVisible();
  });
});
