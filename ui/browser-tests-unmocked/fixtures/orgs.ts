/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-unmocked/fixtures/orgs.ts.
 */
import type { APIRequestContext } from "@playwright/test";
import { execSQL, sqlLiteral } from "./core";

export interface SeededOrganizationDashboardOrg {
  orgId: string;
  orgName: string;
  orgSlug: string;
}

/**
 * TODO: Document seedOrganizationDashboardSmokeOrg.
 */
export async function seedOrganizationDashboardSmokeOrg(
  request: APIRequestContext,
  token: string,
  suffix: string,
): Promise<SeededOrganizationDashboardOrg> {
  const orgName = `Organizations Smoke Org ${suffix}`;
  const orgSlug = `organizations-smoke-${suffix.toLowerCase()}`;

  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_organizations (name, slug, plan_tier)
     VALUES ('${sqlLiteral(orgName)}', '${sqlLiteral(orgSlug)}', 'free')
     RETURNING id, name, slug`,
  );

  return {
    orgId: String(result.rows[0][0]),
    orgName: String(result.rows[0][1]),
    orgSlug: String(result.rows[0][2]),
  };
}

export async function cleanupOrganizationDashboardSmokeOrg(
  request: APIRequestContext,
  token: string,
  orgId: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_organizations WHERE id = '${sqlLiteral(orgId)}'`,
  );
}

// Fixture helper: fetch an organization by ID via the admin API.
// Extracted from spec files to comply with eslint no-restricted-syntax rule.
export async function getOrganizationById(
  request: APIRequestContext,
  adminToken: string,
  orgId: string,
): Promise<{ status: number; body: unknown }> {
  const res = await request.get(`/api/admin/orgs/${orgId}`, {
    headers: { Authorization: `Bearer ${adminToken}` },
  });
  const body = res.ok() ? await res.json().catch(() => null) : null;
  return { status: res.status(), body };
}
