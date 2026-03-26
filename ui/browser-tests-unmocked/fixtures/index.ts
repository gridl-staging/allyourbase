/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_2_auth_jwt_and_private_function_proof/allyourbase_dev/ui/browser-tests-unmocked/fixtures/index.ts.
 */
import { expect, test as base, type APIRequestContext, type Page, type TestInfo } from "@playwright/test";
import { getBrowserUnmockedSkipReason } from "../browser-preflight";
import { checkAuthEnabled, execSQL, getStoredAdminToken, waitForDashboard } from "./core";
import {
  cleanupAuthUser,
  ensureAuthSettings,
  overrideEmailMFACode,
  promoteSessionToAAL2WithTOTP,
} from "./auth";

export * from "./core";
export * from "./auth";
export * from "./oauth";
export * from "./sms";
export * from "./push";
export * from "./admin";
export * from "./jobs";
export * from "./infra";
export * from "./sites";
export * from "./edge-functions";
export * from "./storage";
export * from "./realtime";
export * from "./usage";
export * from "./tenants";
export * from "./orgs";

const browserSkipReason = getBrowserUnmockedSkipReason();
const SAFE_SQL_IDENTIFIER = /^[A-Za-z_][A-Za-z0-9_]*$/;

export function assertSafeSQLIdentifier(identifier: string, label: string): string {
  if (!SAFE_SQL_IDENTIFIER.test(identifier)) {
    throw new Error(`Unsafe SQL identifier for ${label}: ${identifier}`);
  }
  return identifier;
}

export function buildParallelSafeRunID(
  testInfo: Pick<TestInfo, "parallelIndex" | "repeatEachIndex" | "retry">,
): string {
  return `${Date.now()}_${testInfo.parallelIndex}_${testInfo.repeatEachIndex}_${testInfo.retry}`;
}

export async function dropTableIfExists(
  request: APIRequestContext,
  token: string,
  tableName: string,
  label: string,
): Promise<void> {
  const safeTableName = assertSafeSQLIdentifier(tableName, label);
  await execSQL(request, token, `DROP TABLE IF EXISTS ${safeTableName}`);
}

/**
 * TODO: Document createTableViaSQLEditor.
 */
export async function createTableViaSQLEditor(page: Page, tableName: string): Promise<void> {
  const safeTableName = assertSafeSQLIdentifier(tableName, "table name");
  await page.goto("/admin/");
  await waitForDashboard(page);

  const sidebar = page.getByRole("complementary");
  const sqlEditorButton = sidebar.getByRole("button", {
    name: /^SQL Editor$|^Open SQL Editor$/i,
  });
  await expect(sqlEditorButton).toBeVisible();
  await sqlEditorButton.click();

  const sqlInput = page.getByLabel("SQL query");
  await expect(sqlInput).toBeVisible({ timeout: 5000 });
  await sqlInput.fill(
    `CREATE TABLE ${safeTableName} (id serial PRIMARY KEY, name text NOT NULL, user_id uuid)`,
  );
  await page.getByRole("button", { name: /Execute/i }).click();
  await expect(page.getByText(/Statement executed successfully/i)).toBeVisible({
    timeout: 5000,
  });
  await expect(sidebar.getByText(safeTableName, { exact: true })).toBeVisible({
    timeout: 10000,
  });
}

export const test = base.extend<{
  authStatus: { auth: boolean };
  adminToken: string;
  _browserSkipGuard: void;
  mfaHelpers: {
    overrideEmailMFACode: (knownCode: string) => Promise<void>;
    ensureAuthSettings: (overrides: Record<string, boolean>) => Promise<void>;
    cleanupAuthUser: (email: string) => Promise<void>;
    promoteSessionToAAL2WithTOTP: (
      page: Page,
      email: string,
      password: string,
      totpSecret: string,
    ) => Promise<void>;
  };
}>({
  _browserSkipGuard: [
    async ({}, use, testInfo) => {
      if (browserSkipReason) {
        testInfo.skip(browserSkipReason);
      }
      await use();
    },
    { auto: true },
  ],
  authStatus: async ({ request }, use) => {
    const status = await checkAuthEnabled(request);
    await use(status);
  },
  adminToken: async ({}, use) => {
    const token = await getStoredAdminToken();
    await use(token);
  },
  mfaHelpers: async ({ request, adminToken }, use) => {
    await use({
      overrideEmailMFACode: (knownCode: string) =>
        overrideEmailMFACode(request, adminToken, knownCode),
      ensureAuthSettings: (overrides: Record<string, boolean>) =>
        ensureAuthSettings(request, adminToken, overrides),
      cleanupAuthUser: (email: string) => cleanupAuthUser(request, adminToken, email),
      promoteSessionToAAL2WithTOTP: (
        page: Page,
        email: string,
        password: string,
        totpSecret: string,
      ) =>
        promoteSessionToAAL2WithTOTP(request, page, email, password, totpSecret),
    });
  },
});

export { expect } from "@playwright/test";
