import { test, expect, probeEndpoint, fetchAuthHooksConfig, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Auth Hooks — Content-Verified
 *
 * Fetches the live auth hook configuration via admin API, then asserts each
 * rendered hook card reflects the actual configured value or "Not configured".
 */

const HOOK_LABELS: { key: string; label: string }[] = [
  { key: "before_sign_up", label: "Before Sign Up" },
  { key: "after_sign_up", label: "After Sign Up" },
  { key: "custom_access_token", label: "Custom Access Token" },
  { key: "before_password_reset", label: "Before Password Reset" },
  { key: "send_email", label: "Send Email" },
  { key: "send_sms", label: "Send SMS" },
];

test.describe("Smoke: Auth Hooks", () => {
  test("auth hooks page renders hook cards matching live API config", async ({ page, request, adminToken }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/auth/hooks");
    test.skip(
      status === 501 || status === 404,
      `Auth hooks endpoint not available (status ${status})`,
    );

    // Arrange: fetch live hook config via fixture helper
    const hooksConfig = await fetchAuthHooksConfig(request, adminToken);

    // Act: navigate to Auth Hooks view
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /Auth Hooks/i }).click();
    await expect(page.getByRole("heading", { name: /Auth Hooks/i })).toBeVisible({ timeout: 15_000 });

    // Assert: verify each hook card shows the correct value from the API
    for (const { key, label } of HOOK_LABELS) {
      const expectedValue = hooksConfig[key] || "Not configured";
      const card = page.getByTestId(`auth-hook-card-${key}`);
      await expect(card).toBeVisible({ timeout: 3000 });
      await expect(card.getByText(label)).toBeVisible();
      await expect(page.getByTestId(`auth-hook-value-${key}`)).toContainText(expectedValue);
    }
  });
});
