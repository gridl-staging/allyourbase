import { randomUUID } from "crypto";
import { test, expect, execSQL, waitForDashboard } from "../fixtures";
import type { APIRequestContext, Page } from "@playwright/test";

const TEST_PASSWORD_HASH = "$argon2id$v=19$m=65536,t=3,p=4$dGVzdHNhbHQ$dGVzdGhhc2g";

function sqlLiteral(value: string): string {
  return value.replace(/'/g, "''");
}

async function createOwnerUser(
  request: APIRequestContext,
  adminToken: string,
  email: string,
): Promise<string> {
  const escapedEmail = sqlLiteral(email);
  const result = await execSQL(
    request,
    adminToken,
    `DELETE FROM _ayb_users WHERE email = '${escapedEmail}';
     INSERT INTO _ayb_users (email, password_hash)
     VALUES ('${escapedEmail}', '${TEST_PASSWORD_HASH}')
     RETURNING id`,
  );
  const userID = result.rows[0]?.[0];
  if (typeof userID !== "string") {
    throw new Error(`Expected owner user id for email ${email}`);
  }
  return userID;
}

async function openTenantsPage(page: Page): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.locator("aside").getByRole("button", { name: /^Tenants$/i }).click();
  await expect(page.getByTestId("tenants-view")).toBeVisible({ timeout: 5000 });
  await expect(page.getByRole("button", { name: "Create Tenant" })).toBeVisible();
}

async function expectTenantRecord(options: {
  request: APIRequestContext;
  adminToken: string;
  slug: string;
  expectedName: string;
  expectedState?: string;
  expectedOwnerUserID?: string | null;
}): Promise<void> {
  const tenantResult = await execSQL(
    options.request,
    options.adminToken,
    `SELECT name, slug, state
     FROM _ayb_tenants
     WHERE slug = '${sqlLiteral(options.slug)}'`,
  );
  expect(tenantResult.rowCount).toBe(1);
  expect(tenantResult.rows[0]?.[0]).toBe(options.expectedName);
  expect(tenantResult.rows[0]?.[1]).toBe(options.slug);
  if (options.expectedState) {
    expect(tenantResult.rows[0]?.[2]).toBe(options.expectedState);
  }
  if (Object.prototype.hasOwnProperty.call(options, "expectedOwnerUserID")) {
    const ownerMembershipResult = await execSQL(
      options.request,
      options.adminToken,
      `SELECT m.user_id::text
       FROM _ayb_tenant_memberships m
       INNER JOIN _ayb_tenants t ON t.id = m.tenant_id
       WHERE t.slug = '${sqlLiteral(options.slug)}'
         AND m.role = 'owner'`,
    );
    expect(ownerMembershipResult.rowCount).toBe(1);
    expect(ownerMembershipResult.rows[0]?.[0]).toBe(options.expectedOwnerUserID);
  }
}

async function setTenantState(options: {
  request: APIRequestContext;
  adminToken: string;
  slug: string;
  state: string;
}): Promise<void> {
  await execSQL(
    options.request,
    options.adminToken,
    `UPDATE _ayb_tenants
     SET state = '${sqlLiteral(options.state)}'
     WHERE slug = '${sqlLiteral(options.slug)}'`,
  );
}

test.describe("Tenants Lifecycle (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("create, update, suspend/resume, and delete tenant", async ({
    page,
    request,
    adminToken,
  }) => {
    const runID = randomUUID().replace(/-/g, "").slice(0, 10);
    const ownerEmail = `tenant-owner-${runID}@example.com`;
    const tenantName = `Lifecycle Tenant ${runID}`;
    const tenantSlug = `tenant-lifecycle-${runID}`;
    const updatedTenantName = `Lifecycle Tenant Updated ${runID}`;

    pendingCleanup.push(
      `DELETE FROM _ayb_tenants WHERE slug = '${sqlLiteral(tenantSlug)}'`,
      `DELETE FROM _ayb_users WHERE email = '${sqlLiteral(ownerEmail)}'`,
    );

    const ownerUserID = await createOwnerUser(request, adminToken, ownerEmail);

    await openTenantsPage(page);

    await page.getByRole("button", { name: "Create Tenant" }).click();
    await page.getByLabel("Tenant Name").fill(tenantName);
    await page.getByLabel("Slug").fill(tenantSlug);
    const ownerInput = page.getByLabel("Owner User ID");
    await ownerInput.fill(ownerUserID);
    await expect(ownerInput).toHaveValue(ownerUserID);
    await page.getByRole("button", { name: "Create", exact: true }).click();

    await expect(page.getByRole("heading", { name: tenantName })).toBeVisible({ timeout: 5000 });
    await expectTenantRecord({
      request,
      adminToken,
      slug: tenantSlug,
      expectedName: tenantName,
      expectedOwnerUserID: ownerUserID,
    });
    await setTenantState({
      request,
      adminToken,
      slug: tenantSlug,
      state: "active",
    });
    await openTenantsPage(page);
    await page.getByRole("button", { name: new RegExp(tenantSlug, "i") }).click();
    await expect(page.getByRole("heading", { name: tenantName })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: "Suspend" })).toBeVisible({ timeout: 5000 });

    await page.getByLabel("Tenant Name").fill(updatedTenantName);
    await page.getByRole("button", { name: "Save Info" }).click();
    await expect(page.getByRole("heading", { name: updatedTenantName })).toBeVisible({ timeout: 5000 });

    const updatedTenantResult = await execSQL(
      request,
      adminToken,
      `SELECT name, state FROM _ayb_tenants WHERE slug = '${sqlLiteral(tenantSlug)}'`,
    );
    expect(updatedTenantResult.rows[0]?.[0]).toBe(updatedTenantName);
    expect(updatedTenantResult.rows[0]?.[1]).toBe("active");

    await page.getByRole("button", { name: "Suspend" }).click();
    await expect(page.getByRole("button", { name: "Resume" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: "Resume" }).click();
    await expect(page.getByRole("button", { name: "Suspend" })).toBeVisible({ timeout: 5000 });

    await page.getByRole("button", { name: "Delete" }).click();

    const deletedTenantResult = await execSQL(
      request,
      adminToken,
      `SELECT state FROM _ayb_tenants WHERE slug = '${sqlLiteral(tenantSlug)}'`,
    );
    expect(deletedTenantResult.rows[0]?.[0]).toBe("deleting");
  });

  test("create tenant with empty owner user id opens detail view", async ({
    page,
    request,
    adminToken,
  }) => {
    const runID = randomUUID().replace(/-/g, "").slice(0, 10);
    const tenantName = `Ownerless Tenant ${runID}`;
    const tenantSlug = `tenant-ownerless-${runID}`;

    pendingCleanup.push(`DELETE FROM _ayb_tenants WHERE slug = '${sqlLiteral(tenantSlug)}'`);

    await openTenantsPage(page);

    await page.getByRole("button", { name: "Create Tenant" }).click();
    await page.getByLabel("Tenant Name").fill(tenantName);
    await page.getByLabel("Slug").fill(tenantSlug);

    const ownerInput = page.getByLabel("Owner User ID");
    await expect(ownerInput).toHaveValue("");
    await page.getByRole("button", { name: "Create", exact: true }).click();

    await expect(page.getByRole("heading", { name: tenantName })).toBeVisible({ timeout: 5000 });
    await expectTenantRecord({
      request,
      adminToken,
      slug: tenantSlug,
      expectedName: tenantName,
    });
  });
});
