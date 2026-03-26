import type { TestInfo } from "@playwright/test";
import {
  cleanupAuthUser,
  createLinkedEmailAuthSessionToken,
  ensureAuthSettings,
  expect,
  fetchAuthSettings,
  getAuthSettingsUnavailableSkipReason,
  getAuthMeWithToken,
  loginEmailAuthSessionToken,
  probeEndpoint,
  test,
  waitForDashboard,
} from "../fixtures";

interface CleanupState {
  anonymousAuthEnabled?: boolean;
  email?: string;
}

test.describe("Secrets JWT Rotation (Full E2E)", () => {
  const cleanupByTestID = new Map<string, CleanupState>();

  test.afterEach(async ({ request, adminToken }, testInfo: TestInfo) => {
    const cleanup = cleanupByTestID.get(testInfo.testId);
    if (!cleanup) {
      return;
    }

    if (cleanup.email) {
      await cleanupAuthUser(request, adminToken, cleanup.email).catch(() => {});
    }

    if (typeof cleanup.anonymousAuthEnabled === "boolean") {
      await ensureAuthSettings(request, adminToken, {
        anonymous_auth_enabled: cleanup.anonymousAuthEnabled,
      }).catch(() => {});
    }

    cleanupByTestID.delete(testInfo.testId);
  });

  test("stale linked-user token fails on /api/auth/me after JWT rotation", async (
    { page, request, adminToken },
    testInfo: TestInfo,
  ) => {
    const secretsProbeStatus = await probeEndpoint(request, adminToken, "/api/admin/secrets");
    test.skip(
      secretsProbeStatus === 503 || secretsProbeStatus === 404 || secretsProbeStatus === 501,
      `Secrets service unavailable (status ${secretsProbeStatus})`,
    );

    const authSettingsSkipReason = await getAuthSettingsUnavailableSkipReason(request, adminToken);
    test.skip(Boolean(authSettingsSkipReason), authSettingsSkipReason ?? "");

    const runID = `${testInfo.testId}-${testInfo.parallelIndex}-${testInfo.repeatEachIndex}-${testInfo.retry}`
      .replace(/[^a-zA-Z0-9]+/g, "-")
      .toLowerCase()
      .slice(-64);
    const email = `secrets-rotate-${runID}@example.com`;
    const password = `RotatePass!${runID}`;

    const originalAuthSettings = await fetchAuthSettings(request, adminToken);
    cleanupByTestID.set(testInfo.testId, {
      anonymousAuthEnabled: originalAuthSettings.anonymous_auth_enabled,
      email,
    });

    // Deterministic test IDs make reruns reuse the same auth email, so clear any
    // leaked user from a prior aborted run before minting the linked session.
    await cleanupAuthUser(request, adminToken, email).catch(() => {});
    await ensureAuthSettings(request, adminToken, {
      anonymous_auth_enabled: true,
    });

    const preRotationToken = await createLinkedEmailAuthSessionToken(request, email, password);

    const preRotationMe = await getAuthMeWithToken(request, preRotationToken);
    expect(preRotationMe.status()).toBe(200);

    const preRotationUser = (await preRotationMe.json()) as { id?: string };
    expect(typeof preRotationUser.id).toBe("string");

    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Secrets$/i }).click();
    await expect(page.getByRole("heading", { level: 2, name: "Secrets", exact: true })).toBeVisible({
      timeout: 5000,
    });

    await page.getByRole("button", { name: /Rotate JWT Secret/i }).click();
    const rotateDialog = page.getByRole("dialog", { name: /Rotate JWT Secret/i });
    await expect(rotateDialog).toBeVisible({ timeout: 5000 });

    const rotateResponsePromise = page.waitForResponse(
      (response) =>
        response.url().includes("/api/admin/secrets/rotate") &&
        response.request().method() === "POST",
    );
    await rotateDialog.getByRole("button", { name: /^Rotate$/i }).click();

    const rotateResponse = await rotateResponsePromise;
    expect(rotateResponse.status()).toBe(200);
    await expect(rotateDialog).toHaveCount(0, { timeout: 5000 });

    const staleTokenMe = await getAuthMeWithToken(request, preRotationToken);
    expect(staleTokenMe.status()).toBe(401);

    const postRotationToken = await loginEmailAuthSessionToken(request, email, password);
    const postRotationMe = await getAuthMeWithToken(request, postRotationToken);
    expect(postRotationMe.status()).toBe(200);

    const postRotationUser = (await postRotationMe.json()) as { id?: string };
    expect(postRotationUser.id).toBe(preRotationUser.id);
  });
});
