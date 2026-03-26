import type { TestInfo } from "@playwright/test";
import {
  buildParallelSafeRunID,
  cleanupAuthUser,
  createTableViaSQLEditor,
  createLinkedEmailAuthSessionToken,
  dropTableIfExists,
  ensureAuthSettings,
  expect,
  fetchAuthSettings,
  getAuthSettingsUnavailableSkipReason,
  listRecords,
  resolveAuthUserIdByEmail,
  seedRecord,
  test,
} from "../fixtures";

interface CleanupState {
  anonymousAuthEnabled?: boolean;
  emailA?: string;
  emailB?: string;
  tableName?: string;
}

const ownerMatchExpression = "(user_id = current_setting('ayb.user_id', true)::uuid)";

test.describe("Dashboard RLS Denial Journey (Full E2E)", () => {
  const cleanupByTestID = new Map<string, CleanupState>();

  test.afterEach(async ({ request, adminToken }, testInfo: TestInfo) => {
    const cleanup = cleanupByTestID.get(testInfo.testId);
    if (!cleanup) {
      return;
    }

    if (cleanup.emailA) {
      await cleanupAuthUser(request, adminToken, cleanup.emailA).catch(() => {});
    }
    if (cleanup.emailB) {
      await cleanupAuthUser(request, adminToken, cleanup.emailB).catch(() => {});
    }
    if (cleanup.tableName) {
      await dropTableIfExists(
        request,
        adminToken,
        cleanup.tableName,
        "dashboard RLS cleanup table",
      ).catch(() => {});
    }
    if (typeof cleanup.anonymousAuthEnabled === "boolean") {
      await ensureAuthSettings(request, adminToken, {
        anonymous_auth_enabled: cleanup.anonymousAuthEnabled,
      }).catch(() => {});
    }
    cleanupByTestID.delete(testInfo.testId);
  });

  test("owner-match RLS allows owner row and denies cross-user insert", async (
    { page, request, adminToken },
    testInfo: TestInfo,
  ) => {
    const authSettingsSkipReason = await getAuthSettingsUnavailableSkipReason(request, adminToken);
    test.skip(Boolean(authSettingsSkipReason), authSettingsSkipReason ?? "");

    const runID = buildParallelSafeRunID(testInfo);
    const tableName = `dashboard_rls_${runID}`;
    const policyName = `owner_only_${runID}`;
    const emailA = `dashboard-rls-a-${runID}@example.com`;
    const emailB = `dashboard-rls-b-${runID}@example.com`;
    const passwordA = `TestPassA!${runID}`;
    const passwordB = `TestPassB!${runID}`;

    const originalAuthSettings = await fetchAuthSettings(request, adminToken);
    cleanupByTestID.set(testInfo.testId, {
      anonymousAuthEnabled: originalAuthSettings.anonymous_auth_enabled,
      emailA,
      emailB,
      tableName,
    });

    await ensureAuthSettings(request, adminToken, {
      anonymous_auth_enabled: true,
    });

    await createTableViaSQLEditor(page, tableName);

    // Navigate to RLS Policies and enable RLS on the test table
    const sidebar = page.getByRole("complementary");
    await sidebar.getByRole("button", { name: /^RLS Policies$/i }).click();
    await expect(page.getByText("Tables").first()).toBeVisible({ timeout: 5000 });

    const rlsTableButton = page.locator("main").getByRole("button", { name: tableName });
    await expect(rlsTableButton).toBeVisible({ timeout: 5000 });
    await rlsTableButton.click();

    const enableButton = page.getByRole("button", { name: /enable rls/i });
    await expect(enableButton).toBeVisible({ timeout: 5000 });
    await enableButton.click();
    await expect(page.getByText(/RLS enabled on/i).first()).toBeVisible({ timeout: 5000 });

    // Create owner-match policy via modal
    const createPolicyButton = page.getByRole("button", { name: /create policy|new policy|add/i }).first();
    await expect(createPolicyButton).toBeVisible({ timeout: 5000 });
    await createPolicyButton.click();

    await page.getByLabel("Policy name").fill(policyName);
    await page.getByLabel("Command").selectOption("ALL");
    await page.getByLabel("USING expression").fill(ownerMatchExpression);
    await page.getByLabel("WITH CHECK expression").fill(ownerMatchExpression);
    await page.getByRole("button", { name: /^create policy$|^create$|^save$/i }).click();
    await expect(page.getByRole("heading", { name: "Create RLS Policy" })).toHaveCount(0);

    // Assert policy card shows name, command, and owner-match expression text.
    // Postgres normalizes the input expression (adds ::text cast, extra parens),
    // so match key fragments rather than the exact input string.
    await expect(page.getByText(policyName).first()).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("ALL").first()).toBeVisible();
    await expect(page.getByText("USING:").first()).toBeVisible();
    await expect(page.getByText("WITH CHECK:").first()).toBeVisible();
    await expect(page.locator("main")).toContainText("current_setting('ayb.user_id'");
    await expect(page.locator("main")).toContainText("::uuid)");

    // Create two client users and resolve User A's ID for RLS assertions
    const tokenA = await createLinkedEmailAuthSessionToken(request, emailA, passwordA);
    const tokenB = await createLinkedEmailAuthSessionToken(request, emailB, passwordB);
    const userAIdForInsert = await resolveAuthUserIdByEmail(request, adminToken, emailA);

    // User A seeds a record with their own user_id
    const insertedRecordA = await seedRecord(request, tokenA, tableName, {
      name: "owned-by-a",
      user_id: userAIdForInsert,
    });
    const userAId = insertedRecordA["user_id"];
    expect(typeof userAId).toBe("string");
    expect((userAId as string).length).toBeGreaterThan(0);

    // User A sees their own row
    const userAItems = await listRecords(request, tokenA, tableName);
    expect(userAItems).toHaveLength(1);
    expect(userAItems[0]?.["name"]).toBe("owned-by-a");

    // User B gets empty result set (RLS filters on SELECT, not 403)
    const userBItems = await listRecords(request, tokenB, tableName);
    expect(userBItems).toHaveLength(0);

    // User B's insert with wrong user_id is rejected with 403
    await expect(
      seedRecord(request, tokenB, tableName, { name: "stolen", user_id: userAId as string }),
    ).rejects.toThrow(/status 403|insufficient permissions/i);
  });
});
