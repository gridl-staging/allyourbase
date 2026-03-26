/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-unmocked/fixtures/push.ts.
 */
import type { APIRequestContext } from "@playwright/test";
import { execSQL, sqlLiteral } from "./core";

const PUSH_TEST_USER_ID = "00000000-0000-0000-0000-000000000098";
const PUSH_TEST_APP_ID = "00000000-0000-0000-0000-000000000098";

/**
 * TODO: Document ensurePushTestUserAndApp.
 */
async function ensurePushTestUserAndApp(
  request: APIRequestContext,
  token: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    `INSERT INTO _ayb_users (id, email, password_hash)
     VALUES ('${PUSH_TEST_USER_ID}', 'push-fixture-test@example.com', 'noop')
     ON CONFLICT (id) DO NOTHING`,
  );
  await execSQL(
    request,
    token,
    `INSERT INTO _ayb_apps (id, name, description, owner_user_id)
     VALUES ('${PUSH_TEST_APP_ID}', 'push-test-app', 'Push test application', '${PUSH_TEST_USER_ID}')
     ON CONFLICT (id) DO NOTHING`,
  );
}

/**
 * TODO: Document isPushEnabled.
 */
export async function isPushEnabled(
  request: APIRequestContext,
  token: string,
): Promise<boolean> {
  const res = await request.get("/api/admin/push/devices", {
    headers: { Authorization: `Bearer ${token}` },
  });
  const status = res.status();
  if (status === 200) {
    return true;
  }
  if (status === 503) {
    return false;
  }
  let body = "";
  try {
    body = (await res.text()).trim();
  } catch {
    // Ignore parse errors and still throw a status-based error.
  }
  const suffix = body ? `: ${body}` : "";
  throw new Error(`Push enablement check failed with status ${status}${suffix}`);
}

/**
 * TODO: Document seedPushDeviceToken.
 */
export async function seedPushDeviceToken(
  request: APIRequestContext,
  token: string,
  overrides: {
    tokenValue?: string;
    provider?: string;
    platform?: string;
    deviceName?: string;
    isActive?: boolean;
  } = {},
): Promise<{ id: string; token: string; provider: string; platform: string }> {
  await ensurePushTestUserAndApp(request, token);
  const tokenValue = sqlLiteral(overrides.tokenValue || `test-push-token-${Date.now()}`);
  const provider = sqlLiteral(overrides.provider || "fcm");
  const platform = sqlLiteral(overrides.platform || "android");
  const deviceName = sqlLiteral(overrides.deviceName || "Test Device");
  const isActive = overrides.isActive !== false;
  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_device_tokens (app_id, user_id, provider, platform, token, device_name, is_active)
     VALUES ('${PUSH_TEST_APP_ID}', '${PUSH_TEST_USER_ID}', '${provider}', '${platform}', '${tokenValue}', '${deviceName}', ${isActive})
     ON CONFLICT (app_id, provider, token) DO UPDATE SET device_name = EXCLUDED.device_name, is_active = EXCLUDED.is_active
     RETURNING id, token, provider, platform`,
  );
  return {
    id: result.rows[0][0] as string,
    token: result.rows[0][1] as string,
    provider: result.rows[0][2] as string,
    platform: result.rows[0][3] as string,
  };
}

/**
 * TODO: Document seedPushDelivery.
 */
export async function seedPushDelivery(
  request: APIRequestContext,
  token: string,
  overrides: {
    tokenValue?: string;
    provider?: string;
    platform?: string;
    deviceName?: string;
    title?: string;
    body?: string;
    status?: "pending" | "sent" | "failed" | "invalid_token";
    dataPayload?: Record<string, string>;
  } = {},
): Promise<{ id: string; title: string; status: string; device_token_id: string }> {
  const seededToken = await seedPushDeviceToken(request, token, {
    tokenValue: overrides.tokenValue,
    provider: overrides.provider,
    platform: overrides.platform,
    deviceName: overrides.deviceName,
  });

  const title = sqlLiteral(overrides.title || `seeded-push-delivery-${Date.now()}`);
  const body = sqlLiteral(overrides.body || "Seeded push delivery body");
  const rawStatus = overrides.status || "sent";
  const status = sqlLiteral(rawStatus);
  const dataPayload = sqlLiteral(
    JSON.stringify(overrides.dataPayload || { source: "browser-unmocked" }),
  );
  const sentAtSQL = rawStatus === "sent" ? "NOW()" : "NULL";

  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_push_deliveries (device_token_id, app_id, user_id, provider, title, body, data_payload, status, sent_at)
     VALUES ('${seededToken.id}', '${PUSH_TEST_APP_ID}', '${PUSH_TEST_USER_ID}', '${seededToken.provider}', '${title}', '${body}', '${dataPayload}'::jsonb, '${status}', ${sentAtSQL})
     RETURNING id, title, status, device_token_id`,
  );
  return {
    id: result.rows[0][0] as string,
    title: result.rows[0][1] as string,
    status: result.rows[0][2] as string,
    device_token_id: result.rows[0][3] as string,
  };
}

/**
 * TODO: Document cleanupPushTestData.
 */
export async function cleanupPushTestData(
  request: APIRequestContext,
  token: string,
  tokenPattern: string,
): Promise<void> {
  const escapedPattern = sqlLiteral(tokenPattern);
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_push_deliveries WHERE device_token_id IN (
       SELECT id FROM _ayb_device_tokens WHERE token LIKE '%${escapedPattern}%'
     )`,
  );
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_device_tokens WHERE token LIKE '%${escapedPattern}%'`,
  );
}

export { PUSH_TEST_APP_ID, PUSH_TEST_USER_ID };
