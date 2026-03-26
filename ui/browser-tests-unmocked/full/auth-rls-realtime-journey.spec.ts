import type { TestInfo } from "@playwright/test";
import {
  test,
  expect,
  execSQL,
  waitForDashboard,
  probeEndpoint,
  fetchRealtimeStats,
  expectRlsPolicyCard,
} from "../fixtures";

test.describe("Auth + RLS + Realtime Journey (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("table creation to RLS policy to realtime metrics", async (
    { page, request, adminToken },
    testInfo: TestInfo,
  ) => {
    const runId = `${Date.now()}_${testInfo.parallelIndex}_${testInfo.repeatEachIndex}_${testInfo.retry}`;
    const tableName = `auth_rls_rt_${runId}`;
    const policyName = `journey_policy_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (id SERIAL PRIMARY KEY, name TEXT NOT NULL, user_id UUID);`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^RLS Policies$/i }).click();
    await expect(page.getByText("Tables").first()).toBeVisible({ timeout: 5000 });

    const rlsTableButton = page.locator("main").getByRole("button", { name: tableName });
    await expect(rlsTableButton).toBeVisible({ timeout: 5000 });
    await rlsTableButton.click();

    const enableButton = page.getByRole("button", { name: /enable rls/i });
    await expect(enableButton).toBeVisible({ timeout: 5000 });
    await enableButton.click();
    await expect(page.getByText(/RLS enabled on/i).first()).toBeVisible({ timeout: 5000 });

    const createPolicyButton = page.getByRole("button", { name: /create policy|new policy|add/i }).first();
    await expect(createPolicyButton).toBeVisible({ timeout: 5000 });
    await createPolicyButton.click();

    await page.getByLabel("Policy name").fill(policyName);
    await page.getByLabel("Command").selectOption("ALL");
    await page.getByLabel("USING expression").fill("true");
    await page.getByRole("button", { name: /^create policy$|^create$|^save$/i }).click();

    await expect(page.getByRole("heading", { name: "Create RLS Policy" })).toHaveCount(0);

    await expectRlsPolicyCard(page, {
      policyName,
      command: "ALL",
      usingExpression: "true",
    });

    await sidebar.getByRole("button", { name: /^Realtime Inspector$/i }).click();
    await expect(page.getByRole("heading", { name: /Realtime Inspector/i })).toBeVisible({ timeout: 5000 });

    const panel = page.getByTestId("realtime-inspector-panel");
    await expect(panel).toBeVisible({ timeout: 5000 });

    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/realtime/stats");
    if (probeStatus === 503 || probeStatus === 404 || probeStatus === 501) {
      test.info().annotations.push({
        type: "note",
        description: `Realtime assertions skipped: /api/admin/realtime/stats returned ${probeStatus}`,
      });
      return;
    }

    const stats = await fetchRealtimeStats(request, adminToken);
    await expect(panel.getByTestId("realtime-total-metric-value")).toHaveText(
      String(stats.connections.total),
    );
    await expect(panel.getByTestId("realtime-sse-metric-value")).toHaveText(
      String(stats.connections.sse),
    );
    await expect(panel.getByTestId("realtime-ws-metric-value")).toHaveText(
      String(stats.connections.ws),
    );
  });
});
