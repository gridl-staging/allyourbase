import {
  test,
  expect,
  probeEndpoint,
  cleanupAuthUser,
  fetchAuthSettings,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Account Linking — Content-Verified
 *
 * Verifies the account linking form renders real interactive controls:
 * "Start Anonymous Session" button, email/password inputs with correct
 * test IDs, and "Link Account" submit button with correct disabled state.
 *
 * NOTE: AccountLinking has no OAuth provider selector (only anonymous
 * session + email/password link form).
 */

test.describe("Smoke: Account Linking", () => {
  const linkedEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (linkedEmails.length > 0) {
      const email = linkedEmails.pop();
      if (!email) continue;
      await cleanupAuthUser(request, adminToken, email).catch(() => {});
    }
  });

  test("account linking starts anonymous session and submits email link flow", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth-settings");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Auth service unavailable (status ${probeStatus})`,
    );
    const settings = await fetchAuthSettings(request, adminToken);
    test.skip(!settings.anonymous_auth_enabled, "Anonymous auth is disabled in this environment");

    const runId = Date.now();
    const email = `smoke-link-${runId}@example.test`;
    const password = `Sm0kePass!${runId}`;
    linkedEmails.push(email);

    await page.addInitScript(() => {
      window.localStorage.removeItem("ayb_auth_token");
    });

    // Act: navigate to Account Linking
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Account Linking$/i }).click();
    await expect(page.getByRole("heading", { name: /Link Your Account/i })).toBeVisible({ timeout: 15_000 });

    // Assert: anonymous session button is present and functional
    const startSessionButton = page.getByRole("button", { name: /Start Anonymous Session/i });
    await expect(startSessionButton).toBeVisible();
    await startSessionButton.click();
    await expect(page.getByText(/Anonymous session started\./i)).toBeVisible({ timeout: 5000 });

    // Assert: email and password inputs with correct test IDs
    const emailInput = page.getByTestId("link-email-input");
    const passwordInput = page.getByTestId("link-password-input");
    await expect(emailInput).toBeVisible();
    await expect(passwordInput).toBeVisible();

    // Assert: form inputs accept text (interactive verification)
    await emailInput.fill(email);
    await passwordInput.fill(password);
    await expect(emailInput).toHaveValue(email);
    await expect(passwordInput).toHaveValue(password);

    // Assert: Link Account button is enabled and submitting triggers linking flow
    const linkButton = page.getByRole("button", { name: /Link Account/i });
    await expect(linkButton).toBeVisible();
    await expect(linkButton).toBeEnabled();
    await linkButton.click();
    await expect(page.getByText(/Account linked successfully/i)).toBeVisible({ timeout: 10000 });
  });
});
