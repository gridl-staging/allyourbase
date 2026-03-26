import { test, expect, probeEndpoint, fetchAuthHooksConfig, waitForDashboard } from "../fixtures";

/**
 * FULL E2E TEST: Auth Hooks Lifecycle
 *
 * Critical Path: Navigate to Auth Hooks → verify all 6 hook labels render →
 * verify each hook shows either a function reference or "Not configured" →
 * verify hook config reflects server state via API cross-check
 *
 * Note: Auth Hooks UI is read-only (config is server-side, no mutations in UI).
 * This lifecycle test verifies that the rendered state matches the API response
 * and that all hook slots are displayed correctly.
 */

test.describe("Auth Hooks Lifecycle (Full E2E)", () => {
  const HOOK_LABELS = [
    "Before Sign Up",
    "After Sign Up",
    "Custom Access Token",
    "Before Password Reset",
    "Send Email",
    "Send SMS",
  ];

  const HOOK_KEY_MAP: Record<string, string> = {
    "Before Sign Up": "before_sign_up",
    "After Sign Up": "after_sign_up",
    "Custom Access Token": "custom_access_token",
    "Before Password Reset": "before_password_reset",
    "Send Email": "send_email",
    "Send SMS": "send_sms",
  };

  test("verify all auth hook labels render with correct config state", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth/hooks");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Auth hooks service unavailable (status ${probeStatus})`,
    );

    // Fetch hook config via fixture for cross-check
    const hookConfig = await fetchAuthHooksConfig(request, adminToken);

    // Navigate to Auth Hooks
    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Auth Hooks$/i }).click();
    await expect(page.getByRole("heading", { name: /Auth Hooks/i })).toBeVisible({ timeout: 5000 });

    // Verify all 6 hook labels render
    for (const label of HOOK_LABELS) {
      await expect(page.getByText(label, { exact: true })).toBeVisible({ timeout: 3000 });
    }

    // Cross-check: each hook must show "Not configured" or its function reference
    // All hooks display one of these two states — assert both are present as a set
    const notConfiguredCount = HOOK_LABELS.filter(
      (label) => !hookConfig[HOOK_KEY_MAP[label]],
    ).length;
    const configuredCount = HOOK_LABELS.length - notConfiguredCount;

    // Verify at least the expected count of "Not configured" labels appear
    const notConfiguredLocators = page.getByText("Not configured");
    await expect(notConfiguredLocators.first()).toBeVisible({ timeout: 3000 });
    await expect(notConfiguredLocators).toHaveCount(notConfiguredCount, { timeout: 3000 });

    // Verify configured hooks show their function references
    const configuredLabels = HOOK_LABELS.filter(
      (label) => !!hookConfig[HOOK_KEY_MAP[label]],
    );
    for (const label of configuredLabels) {
      const funcRef = hookConfig[HOOK_KEY_MAP[label]];
      await expect(page.getByText(funcRef)).toBeVisible({ timeout: 3000 });
    }

    // Summary assertion: total hook slots match
    expect(notConfiguredCount + configuredCount).toBe(6);
  });
});
