/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-unmocked/fixtures/tenants.ts.
 */
import type { APIRequestContext } from "@playwright/test";
import { execSQL, sqlLiteral } from "./core";

export interface SeededTenantDashboardTenant {
  tenantId: string;
  tenantName: string;
  tenantSlug: string;
}

/**
 * TODO: Document seedTenantDashboardSmokeTenant.
 */
export async function seedTenantDashboardSmokeTenant(
  request: APIRequestContext,
  token: string,
  suffix: string,
): Promise<SeededTenantDashboardTenant> {
  const tenantName = `Tenants Smoke Tenant ${suffix}`;
  const tenantSlug = `tenants-smoke-${suffix.toLowerCase()}`;

  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_tenants (name, slug, state)
     VALUES ('${sqlLiteral(tenantName)}', '${sqlLiteral(tenantSlug)}', 'active')
     RETURNING id, name, slug`,
  );

  return {
    tenantId: String(result.rows[0][0]),
    tenantName: String(result.rows[0][1]),
    tenantSlug: String(result.rows[0][2]),
  };
}

export async function cleanupTenantDashboardSmokeTenant(
  request: APIRequestContext,
  token: string,
  tenantId: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_tenants WHERE id = '${sqlLiteral(tenantId)}'`,
  );
}
