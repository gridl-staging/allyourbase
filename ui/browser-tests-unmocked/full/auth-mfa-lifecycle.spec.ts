import { test, expect, probeEndpoint, generateTOTPCode, waitForDashboard } from "../fixtures";

/**
 * FULL E2E TEST: Auth MFA Lifecycle
 *
 * Tests the complete MFA vertical flow through the dashboard with real server + DB:
 *   1. Auth settings: verify and enable TOTP, anonymous auth, email MFA
 *   2. Anonymous session: create via Account Linking page
 *   3. Account linking: set email + password on anonymous account
 *   4. TOTP enrollment: set up authenticator, confirm with generated code
 *   5. Backup codes: generate, verify display, close
 *   6. Email MFA enrollment: trigger, confirm with overridden code
 *   7. Final state: verify all enrolled factors and backup code count
 *
 * Single long test (journey-style) to preserve localStorage across steps.
 */

test.describe("Auth MFA Lifecycle (Full E2E)", () => {
  const userEmails: string[] = [];

  test.afterEach(async ({ mfaHelpers }) => {
    while (userEmails.length > 0) {
      const email = userEmails.pop();
      if (!email) continue;
      await mfaHelpers.cleanupAuthUser(email).catch(() => {});
    }
  });

  test("anonymous session → link → TOTP → backup codes → email MFA", async ({
    page,
    request,
    adminToken,
    mfaHelpers,
  }) => {
    test.setTimeout(120_000);
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth-settings");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501 || probeStatus === 500,
      `Auth settings service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const testEmail = `mfa-lifecycle-${runId}@example.com`;
    const testPassword = "TestP@ss123!";
    userEmails.push(testEmail);

    // ── Arrange: ensure auth features are enabled via API ──
    await mfaHelpers.ensureAuthSettings({
      totp_enabled: true,
      anonymous_auth_enabled: true,
      email_mfa_enabled: true,
    });

    // ── Step 1: Auth Settings page — verify toggles render correctly ──
    await test.step("Auth Settings: verify feature toggles", async () => {
      await page.goto("/admin/");
      await waitForDashboard(page);
      await page.locator("aside").getByRole("button", { name: /Auth Settings/i }).click();
      await expect(page.getByRole("heading", { name: /Auth Settings/i })).toBeVisible({ timeout: 5000 });

      // Verify all 3 required toggles are checked
      await expect(page.getByTestId("toggle-totp_enabled")).toBeChecked();
      await expect(page.getByTestId("toggle-anonymous_auth_enabled")).toBeChecked();
      await expect(page.getByTestId("toggle-email_mfa_enabled")).toBeChecked();
    });

    // ── Step 2: Create anonymous session ──
    await test.step("Account Linking: start anonymous session", async () => {
      await page.locator("aside").getByRole("button", { name: /Account Linking/i }).click();
      await expect(page.getByRole("heading", { name: /Link Your Account/i })).toBeVisible({ timeout: 5000 });

      await page.getByRole("button", { name: /Start Anonymous Session/i }).click();
      await expect(page.getByText("Anonymous session started")).toBeVisible({ timeout: 10000 });
    });

    // ── Step 3: Link to email account ──
    await test.step("Account Linking: link email and password", async () => {
      await page.getByLabel("Email").fill(testEmail);
      await page.getByLabel("Password").fill(testPassword);
      await page.getByRole("button", { name: /Link Account/i }).click();
      await expect(page.getByText("Account linked successfully")).toBeVisible({ timeout: 10000 });
    });

    // ── Step 4: TOTP enrollment ──
    let totpSecret = "";

    await test.step("MFA Management: enroll TOTP", async () => {
      await page.locator("aside").getByRole("button", { name: /MFA Management/i }).click();
      await expect(page.getByRole("heading", { name: /Multi-Factor Authentication/i })).toBeVisible({ timeout: 5000 });

      // Should show no MFA methods initially
      await expect(page.getByText("No MFA methods enrolled")).toBeVisible();

      // Start TOTP enrollment
      await page.getByRole("button", { name: /Set Up Authenticator/i }).click();
      await expect(page.getByText("Set Up Authenticator App")).toBeVisible({ timeout: 5000 });

      // Verify TOTP URI is present (web-first assertion)
      const totpUriLocator = page.getByTestId("totp-uri");
      await expect(totpUriLocator).toContainText("otpauth://");

      // Capture URI text for TOTP code generation (data extraction, not assertion)
      const totpUri = await totpUriLocator.textContent();
      const url = new URL(totpUri!);
      totpSecret = url.searchParams.get("secret")!;

      // Generate valid TOTP code and enter it
      const code = generateTOTPCode(totpSecret);
      await page.getByTestId("totp-confirm-code").fill(code);
      await page.getByRole("button", { name: /Verify Code/i }).click();
      await expect(page.getByText("TOTP MFA enrolled successfully")).toBeVisible({ timeout: 10000 });

      // Verify TOTP now appears in enrolled methods (label text can vary by backend formatting)
      await expect(page.getByTestId("mfa-factor-totp")).toBeVisible();
    });

    // ── Arrange for AAL2-gated actions: promote session using enrolled TOTP factor ──
    await test.step("MFA Management: step up session to AAL2", async () => {
      await mfaHelpers.promoteSessionToAAL2WithTOTP(page, testEmail, testPassword, totpSecret);
    });

    // ── Step 5: Generate backup codes ──
    await test.step("MFA Management: generate backup codes", async () => {
      await page.getByRole("button", { name: /Generate Backup Codes/i }).click();
      await expect(page.getByText("Save Your Backup Codes")).toBeVisible({ timeout: 5000 });
      await expect(page.getByText("Each code can only be used once")).toBeVisible();

      // Verify at least one backup code is displayed (format: xxxxx-xxxxx)
      await expect(page.getByText(/^[a-z0-9]{5}-[a-z0-9]{5}$/i).first()).toBeVisible();

      // Close the dialog
      await page.getByRole("button", { name: "Done" }).click();

      // Verify backup code count is shown
      await expect(page.getByText("10 backup codes remaining")).toBeVisible({ timeout: 5000 });
    });

    // ── Step 6: Email MFA enrollment ──
    await test.step("MFA Management: enroll email MFA", async () => {
      await page.getByRole("button", { name: /Set Up Email MFA/i }).click();
      await expect(page.getByRole("heading", { name: /Confirm Email MFA/i })).toBeVisible({
        timeout: 10000,
      });
      await expect(page.getByTestId("email-mfa-confirm-code")).toBeVisible();

      // Override the email challenge code in the database with a known value
      const knownCode = "123456";
      await mfaHelpers.overrideEmailMFACode(knownCode);

      // Enter the known code and confirm
      await page.getByTestId("email-mfa-confirm-code").fill(knownCode);
      await page.getByRole("button", { name: /Confirm Email MFA/i }).click();
      await expect(page.getByText("Email MFA enrolled successfully")).toBeVisible({ timeout: 10000 });
    });

    // ── Step 7: Verify final state ──
    await test.step("MFA Management: verify enrolled factors and backup count", async () => {
      const enrolledMethods = page.getByTestId("mfa-enrolled-methods");

      // Both TOTP and Email should appear in enrolled methods (email label includes masked address)
      await expect(enrolledMethods.getByTestId("mfa-factor-totp")).toBeVisible();
      await expect(enrolledMethods.getByTestId("mfa-factor-email")).toBeVisible();

      // Backup codes should still show count
      await expect(page.getByText("10 backup codes remaining")).toBeVisible();

      // Regenerate button should be available
      await expect(page.getByRole("button", { name: "Regenerate" })).toBeVisible();
    });
  });
});
