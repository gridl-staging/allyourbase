import { test, expect, bootstrapMockedAdminApp, mockMFAApis } from "./fixtures";

test.describe("Auth + MFA Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("load-and-verify: seeded MFA factors render in management view", async ({ page }) => {
    await mockMFAApis(page, {
      factorsResponse: {
        status: 200,
        body: { factors: [{ id: "f-totp-1", method: "totp", enabled: true }] },
      },
    });

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^MFA Management$/i }).click();

    await expect(page.getByRole("heading", { name: "Multi-Factor Authentication" })).toBeVisible();
    await expect(page.getByText("Authenticator App")).toBeVisible();
    await expect(page.getByText("5 backup codes remaining")).toBeVisible();
  });

  test("shows backend mailer failure when starting email MFA enrollment", async ({ page }) => {
    await mockMFAApis(page, {
      enrollEmailResponse: {
        status: 500,
        body: { message: "failed to send mfa email" },
      },
    });

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^MFA Management$/i }).click();
    await expect(page.getByRole("heading", { name: "Multi-Factor Authentication" })).toBeVisible();

    await page.getByRole("button", { name: /set up email mfa/i }).click();
    await expect(page.getByText("failed to send mfa email")).toBeVisible({ timeout: 5000 });
  });

  test("shows expired TOTP challenge error during enrollment confirmation", async ({ page }) => {
    await mockMFAApis(page, {
      totpEnrollConfirmResponse: {
        status: 400,
        body: { message: "totp challenge expired" },
      },
    });

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^MFA Management$/i }).click();
    await expect(page.getByRole("heading", { name: "Multi-Factor Authentication" })).toBeVisible();

    await page.getByRole("button", { name: /set up authenticator/i }).click();
    await expect(page.getByTestId("totp-confirm-code")).toBeVisible();

    await page.getByTestId("totp-confirm-code").fill("123456");
    await page.getByRole("button", { name: /verify code/i }).click();

    await expect(page.getByText(/challenge expired/i)).toBeVisible({ timeout: 5000 });
  });

  test("shows lockout response from backend on email MFA confirm", async ({ page }) => {
    await mockMFAApis(page, {
      enrollEmailResponse: {
        status: 200,
        body: { message: "verification code sent to your email" },
      },
      emailEnrollConfirmResponse: {
        status: 429,
        body: { message: "too many failed attempts, try again in 30 minutes" },
      },
    });

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^MFA Management$/i }).click();
    await expect(page.getByRole("heading", { name: "Multi-Factor Authentication" })).toBeVisible();

    await page.getByRole("button", { name: /set up email mfa/i }).click();
    await expect(page.getByTestId("email-mfa-confirm-code")).toBeVisible();

    await page.getByTestId("email-mfa-confirm-code").fill("123456");
    await page.getByRole("button", { name: /confirm email mfa/i }).click();

    await expect(page.getByText(/too many failed attempts/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/30 minutes/i)).toBeVisible();
  });

  test("shows linking conflict message when email belongs to another account", async ({ page }) => {
    const apis = await mockMFAApis(page, {
      linkEmailResponse: {
        status: 409,
        body: { message: "email already belongs to another account" },
      },
    });

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Account Linking$/i }).click();
    await expect(page.getByRole("heading", { name: "Link Your Account" })).toBeVisible();

    await page.getByRole("button", { name: /start anonymous session/i }).click();
    await page.getByTestId("link-email-input").fill("taken@example.com");
    await page.getByTestId("link-password-input").fill("secure-password");
    await page.getByRole("button", { name: /link account/i }).click();

    await expect.poll(() => apis.linkEmailCalls, { timeout: 5000 }).toBe(1);
    await expect(page.getByText(/already belongs to another account/i)).toBeVisible();
  });

  test("shows anonymous session expiry message when linking with stale session", async ({ page }) => {
    const apis = await mockMFAApis(page, {
      linkEmailResponse: {
        status: 401,
        body: { message: "anonymous session expired, start a new session" },
      },
    });

    await page.goto("/admin/");
    await page.getByRole("button", { name: /^Account Linking$/i }).click();
    await expect(page.getByRole("heading", { name: "Link Your Account" })).toBeVisible();

    await page.getByRole("button", { name: /start anonymous session/i }).click();
    await page.getByTestId("link-email-input").fill("expired@example.com");
    await page.getByTestId("link-password-input").fill("secure-password");
    await page.getByRole("button", { name: /link account/i }).click();

    await expect.poll(() => apis.linkEmailCalls, { timeout: 5000 }).toBe(1);
    await expect(page.getByText(/anonymous session expired/i)).toBeVisible();
  });
});
