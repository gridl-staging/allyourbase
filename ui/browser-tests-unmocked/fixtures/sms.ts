/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-unmocked/fixtures/sms.ts.
 */
import type { APIRequestContext, TestInfo } from "@playwright/test";
import { execSQL, sqlLiteral } from "./core";

const SMS_TEST_USER_ID = "00000000-0000-0000-0000-000000000099";
const DEFAULT_SMS_PROVIDER_SKIP_REASON =
  "SMS provider not configured — skipping SMS Health smoke";

function escapeLikePattern(value: string): string {
  return sqlLiteral(value).replace(/[\\%_]/g, "\\$&");
}

async function ensureSMSTestUser(
  request: APIRequestContext,
  token: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    `INSERT INTO _ayb_users (id, email, password_hash)
     VALUES ('${SMS_TEST_USER_ID}', 'sms-fixture-test@example.com', 'noop')
     ON CONFLICT (id) DO NOTHING`,
  );
}

/**
 * TODO: Document seedSMSMessage.
 */
export async function seedSMSMessage(
  request: APIRequestContext,
  token: string,
  overrides: {
    to_phone?: string;
    body?: string;
    provider?: string;
    status?: string;
    error_message?: string;
  } = {},
): Promise<{ id: string; to_phone: string; body: string; status: string }> {
  await ensureSMSTestUser(request, token);
  const toPhone = overrides.to_phone || "+15551234567";
  const body = overrides.body || "Test SMS message";
  const provider = overrides.provider || "log";
  const status = overrides.status || "delivered";
  const errorMessage = overrides.error_message || "";
  const safeToPhone = sqlLiteral(toPhone);
  const safeBody = sqlLiteral(body);
  const safeProvider = sqlLiteral(provider);
  const safeStatus = sqlLiteral(status);
  const safeErrorMessage = sqlLiteral(errorMessage);
  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_sms_messages (user_id, to_phone, body, provider, status, error_message)
     VALUES ('${SMS_TEST_USER_ID}', '${safeToPhone}', '${safeBody}', '${safeProvider}', '${safeStatus}', '${safeErrorMessage}')
     RETURNING id, to_phone, body, status`,
  );
  return {
    id: result.rows[0][0] as string,
    to_phone: result.rows[0][1] as string,
    body: result.rows[0][2] as string,
    status: result.rows[0][3] as string,
  };
}

export async function cleanupSMSMessages(
  request: APIRequestContext,
  token: string,
  bodyPattern: string,
): Promise<void> {
  const safeBodyPattern = escapeLikePattern(bodyPattern);
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_sms_messages WHERE body LIKE '%${safeBodyPattern}%' ESCAPE '\\'`,
  );
}

/**
 * TODO: Document seedSMSDailyCounts.
 */
export async function seedSMSDailyCounts(
  request: APIRequestContext,
  token: string,
  overrides: {
    count?: number;
    confirm_count?: number;
    fail_count?: number;
  } = {},
): Promise<void> {
  const count = overrides.count ?? 10;
  const confirm = overrides.confirm_count ?? 5;
  const fail = overrides.fail_count ?? 2;
  await execSQL(
    request,
    token,
    `INSERT INTO _ayb_sms_daily_counts (date, count, confirm_count, fail_count)
     VALUES (CURRENT_DATE, ${count}, ${confirm}, ${fail})
     ON CONFLICT (date) DO UPDATE SET
       count = EXCLUDED.count,
       confirm_count = EXCLUDED.confirm_count,
       fail_count = EXCLUDED.fail_count`,
  );
}

export async function cleanupSMSDailyCounts(
  request: APIRequestContext,
  token: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    "DELETE FROM _ayb_sms_daily_counts WHERE date = CURRENT_DATE",
  );
}

export async function cleanupSMSDailyCountsAll(
  request: APIRequestContext,
  token: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    "DELETE FROM _ayb_sms_daily_counts WHERE date >= CURRENT_DATE - INTERVAL '29 days'",
  );
}

/**
 * TODO: Document seedSMSMessageBatch.
 */
export async function seedSMSMessageBatch(
  request: APIRequestContext,
  token: string,
  count: number,
  bodyPrefix: string,
): Promise<void> {
  await ensureSMSTestUser(request, token);
  const safeBodyPrefix = sqlLiteral(bodyPrefix);
  await execSQL(
    request,
    token,
    `INSERT INTO _ayb_sms_messages (user_id, to_phone, body, provider, status)
     SELECT '${SMS_TEST_USER_ID}',
            '+1555' || LPAD(g::text, 7, '0'),
            '${safeBodyPrefix}' || g,
            'log',
            'delivered'
     FROM generate_series(1, ${count}) g`,
  );
}

export async function isSMSProviderConfigured(
  request: APIRequestContext,
  token: string,
): Promise<boolean> {
  const res = await request.post("/api/admin/sms/send", {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: { to: "", body: "" },
  });
  return res.status() !== 404;
}

export async function skipUnlessSMSProviderConfigured(
  request: APIRequestContext,
  token: string,
  testInfo: TestInfo,
  reason = DEFAULT_SMS_PROVIDER_SKIP_REASON,
): Promise<void> {
  if (!(await isSMSProviderConfigured(request, token))) {
    testInfo.skip(reason);
  }
}
