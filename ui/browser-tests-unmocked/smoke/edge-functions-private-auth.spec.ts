import {
  test,
  expect,
  probeEndpoint,
  seedEdgeFunction,
  deleteEdgeFunction,
  invokePublicEdgeFunctionGET,
  fetchAuthSettings,
  getAuthSettingsUnavailableSkipReason,
  createLinkedEmailAuthSessionToken,
  cleanupAuthUser,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Edge Functions - Private auth on public invoke route
 *
 * Critical Path: Seed private function -> verify dashboard surface -> exercise public invoke route
 */

test.describe("Smoke: Edge Functions Private Auth", () => {
  const functionIDs: string[] = [];
  const linkedEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (functionIDs.length > 0) {
      const functionID = functionIDs.pop();
      if (!functionID) continue;
      await deleteEdgeFunction(request, adminToken, functionID).catch(() => {});
    }

    while (linkedEmails.length > 0) {
      const email = linkedEmails.pop();
      if (!email) continue;
      await cleanupAuthUser(request, adminToken, email).catch(() => {});
    }
  });

  test("private function rejects unauthenticated public invoke and accepts linked bearer token", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/functions");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Edge functions service unavailable (status ${probeStatus})`,
    );

    const authSettingsSkipReason = await getAuthSettingsUnavailableSkipReason(request, adminToken);
    test.skip(Boolean(authSettingsSkipReason), authSettingsSkipReason ?? "");

    const settings = await fetchAuthSettings(request, adminToken);
    test.skip(!settings.anonymous_auth_enabled, "Anonymous auth is disabled in this environment");

    const runId = Date.now();
    const marker = `private-proof-${runId}`;
    const fnName = `private-proof-${runId}`;
    const fn = await seedEdgeFunction(request, adminToken, {
      name: fnName,
      public: false,
      source: `export default function handler(req) {\n  return {\n    statusCode: 200,\n    body: JSON.stringify({ ok: true, marker: "${marker}" }),\n    headers: { "Content-Type": "application/json" },\n  };\n}`,
    });
    functionIDs.push(fn.id);

    const email = `smoke-private-${runId}@example.test`;
    linkedEmails.push(email);
    const authToken = await createLinkedEmailAuthSessionToken(
      request,
      email,
      `Sm0kePass!${runId}`,
    );
    expect(authToken.length).toBeGreaterThan(0);

    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Edge Functions$/i }).click();
    await expect(page.getByRole("heading", { name: "Edge Functions" })).toBeVisible();
    await expect(page.getByRole("cell", { name: fnName })).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId(`fn-public-${fn.id}`)).toHaveText("Private");

    await page.getByRole("cell", { name: fnName }).click();
    await expect(page.getByRole("heading", { name: fnName })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText("Private", { exact: true })).toBeVisible();

    const unauthenticatedInvoke = await invokePublicEdgeFunctionGET(request, fnName);
    expect(unauthenticatedInvoke.status()).toBe(401);
    await expect(unauthenticatedInvoke.text()).resolves.toContain("authentication required");

    const authenticatedInvoke = await invokePublicEdgeFunctionGET(request, fnName, authToken);
    expect(authenticatedInvoke.status()).toBe(200);
    await expect(authenticatedInvoke.text()).resolves.toContain(marker);
  });
});
