/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-unmocked/fixtures/usage.ts.
 */
import type { APIRequestContext } from "@playwright/test";
import { execSQL, sqlLiteral } from "./core";

export interface SeededUsageMeteringTenant {
  tenantId: string;
  tenantName: string;
  tenantSlug: string;
}

const USAGE_SMOKE_REQUEST_COUNT_TODAY = 4_000_000_000_000_000;
const USAGE_SMOKE_REQUEST_COUNT_YESTERDAY = 2_000_000_000_000_000;

/**
 * TODO: Document seedUsageMeteringTenantDailyRows.
 */
export async function seedUsageMeteringTenantDailyRows(
  request: APIRequestContext,
  token: string,
  suffix: string,
): Promise<SeededUsageMeteringTenant> {
  const tenantName = `Usage Smoke Tenant ${suffix}`;
  const tenantSlug = `usage-smoke-${suffix.toLowerCase()}`;

  const tenantResult = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_tenants (name, slug, state)
     VALUES ('${sqlLiteral(tenantName)}', '${sqlLiteral(tenantSlug)}', 'active')
     RETURNING id, name, slug`,
  );

  const tenantId = String(tenantResult.rows[0][0]);

  await execSQL(
    request,
    token,
    `INSERT INTO _ayb_tenant_usage_daily (
       tenant_id,
       date,
       request_count,
       db_bytes_used,
       bandwidth_bytes,
       function_invocations,
       realtime_peak_connections,
       job_runs
     )
     VALUES
       ('${tenantId}', CURRENT_DATE, ${USAGE_SMOKE_REQUEST_COUNT_TODAY}, 8192, 16384, 240, 18, 11),
       ('${tenantId}', (CURRENT_DATE - INTERVAL '1 day')::date, ${USAGE_SMOKE_REQUEST_COUNT_YESTERDAY}, 4096, 8192, 120, 12, 6)
     ON CONFLICT (tenant_id, date) DO UPDATE SET
       request_count = EXCLUDED.request_count,
       db_bytes_used = EXCLUDED.db_bytes_used,
       bandwidth_bytes = EXCLUDED.bandwidth_bytes,
       function_invocations = EXCLUDED.function_invocations,
       realtime_peak_connections = EXCLUDED.realtime_peak_connections,
       job_runs = EXCLUDED.job_runs`,
  );

  return { tenantId, tenantName, tenantSlug };
}

export async function cleanupUsageMeteringTenant(
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
