/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_2_auth_jwt_and_private_function_proof/allyourbase_dev/ui/browser-tests-unmocked/fixtures/admin.ts.
 */
import type { APIRequestContext } from "@playwright/test";
import { randomUUID } from "crypto";
import { execSQL, sqlLiteral, validateResponse } from "./core";

const TEST_PASSWORD_HASH = "$argon2id$v=19$m=65536,t=3,p=4$dGVzdHNhbHQ$dGVzdGhhc2g";
const OAUTH_CLIENT_DEPENDENCY_TABLES = [
  "_ayb_oauth_tokens",
  "_ayb_oauth_authorization_codes",
  "_ayb_oauth_consents",
] as const;

function assertSafeSQLInteger(value: number, label: string): number {
  if (!Number.isInteger(value) || value < 0) {
    throw new Error(`Unsafe SQL integer for ${label}: ${value}`);
  }
  return value;
}

/**
 * TODO: Document ensureUserByEmail.
 */
export async function ensureUserByEmail(
  request: APIRequestContext,
  token: string,
  email: string,
): Promise<{ id: string; email: string }> {
  const escapedEmail = sqlLiteral(email);
  await execSQL(
    request,
    token,
    `INSERT INTO _ayb_users (email, password_hash)
     VALUES ('${escapedEmail}', '${TEST_PASSWORD_HASH}')
     ON CONFLICT DO NOTHING`,
  );
  const result = await execSQL(
    request,
    token,
    `SELECT id FROM _ayb_users WHERE email = '${escapedEmail}'`,
  );
  const id = result.rows[0]?.[0];
  if (typeof id !== "string") {
    throw new Error(`Expected user id for email ${email}`);
  }
  return { id, email };
}

export async function cleanupUserByEmail(
  request: APIRequestContext,
  token: string,
  email: string,
): Promise<void> {
  const escapedEmail = sqlLiteral(email);
  await execSQL(request, token, `DELETE FROM _ayb_users WHERE email = '${escapedEmail}'`);
}

/**
 * TODO: Document seedApiKey.
 */
export async function seedApiKey(
  request: APIRequestContext,
  token: string,
  options: {
    userId: string;
    name: string;
    keyHash?: string;
    keyPrefix?: string;
    scope?: "*" | "readonly" | "readwrite";
  },
): Promise<{ id: string; name: string }> {
  const keyHash = options.keyHash || `seed-hash-${Date.now()}`;
  const keyPrefix = options.keyPrefix || "ayb_seed";
  const scope = options.scope || "*";
  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_api_keys (user_id, name, key_hash, key_prefix, scope)
     VALUES ('${sqlLiteral(options.userId)}', '${sqlLiteral(options.name)}', '${sqlLiteral(keyHash)}', '${sqlLiteral(keyPrefix)}', '${sqlLiteral(scope)}')
     RETURNING id, name`,
  );
  const id = result.rows[0]?.[0];
  const name = result.rows[0]?.[1];
  if (typeof id !== "string" || typeof name !== "string") {
    throw new Error(`Expected seeded API key id/name for key ${options.name}`);
  }
  return { id, name };
}

export async function cleanupApiKeyByName(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  await execSQL(request, token, `DELETE FROM _ayb_api_keys WHERE name = '${sqlLiteral(name)}'`);
}

/**
 * TODO: Document seedOAuthClient.
 */
export async function seedOAuthClient(
  request: APIRequestContext,
  token: string,
  options: {
    appId: string;
    name: string;
    clientType?: "confidential" | "public";
    redirectUris?: string[];
    scopes?: string[];
  },
): Promise<{ id: string; clientId: string; name: string; clientSecret?: string }> {
  const res = await request.post("/api/admin/oauth/clients", {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: {
      appId: options.appId,
      name: options.name,
      clientType: options.clientType || "confidential",
      redirectUris: options.redirectUris || ["https://example.test/callback"],
      scopes: options.scopes || ["readonly"],
    },
  });
  await validateResponse(res, `Create OAuth client ${options.name}`);
  const body = await res.json();
  const id = body?.client?.id;
  const clientId = body?.client?.clientId;
  const name = body?.client?.name;
  const clientSecret = body?.clientSecret;
  if (typeof id !== "string" || typeof clientId !== "string" || typeof name !== "string") {
    throw new Error(`Expected OAuth client id/clientId/name for ${options.name}`);
  }
  if (clientSecret !== undefined && typeof clientSecret !== "string") {
    throw new Error(`Expected OAuth client secret to be a string when present for ${options.name}`);
  }
  return { id, clientId, name, clientSecret };
}

/**
 * TODO: Document cleanupOAuthClientByName.
 */
export async function cleanupOAuthClientByName(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const safeName = sqlLiteral(name);
  const clientIDQuery = `(SELECT client_id FROM _ayb_oauth_clients WHERE name = '${safeName}')`;
  for (const tableName of OAUTH_CLIENT_DEPENDENCY_TABLES) {
    await execSQL(request, token, `DELETE FROM ${tableName} WHERE client_id IN ${clientIDQuery}`);
  }
  await execSQL(request, token, `DELETE FROM _ayb_oauth_clients WHERE name = '${safeName}'`);
}

/**
 * TODO: Document seedAdminApp.
 */
export async function seedAdminApp(
  request: APIRequestContext,
  token: string,
  options: {
    name: string;
    ownerUserId: string;
    description?: string;
    rateLimitRps?: number;
    rateLimitWindowSeconds?: number;
  },
): Promise<{ id: string; name: string }> {
  const rateLimitRps = assertSafeSQLInteger(options.rateLimitRps ?? 0, "rateLimitRps");
  const rateLimitWindowSeconds = assertSafeSQLInteger(
    options.rateLimitWindowSeconds ?? 60,
    "rateLimitWindowSeconds",
  );
  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_apps (name, description, owner_user_id, rate_limit_rps, rate_limit_window_seconds)
     VALUES ('${sqlLiteral(options.name)}', '${sqlLiteral(options.description || "")}', '${sqlLiteral(options.ownerUserId)}', ${rateLimitRps}, ${rateLimitWindowSeconds})
     RETURNING id, name`,
  );
  const id = result.rows[0]?.[0];
  const name = result.rows[0]?.[1];
  if (typeof id !== "string" || typeof name !== "string") {
    throw new Error(`Expected seeded app id/name for app ${options.name}`);
  }
  return { id, name };
}

export async function cleanupAdminAppByName(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  await execSQL(request, token, `DELETE FROM _ayb_apps WHERE name = '${sqlLiteral(name)}'`);
}

/**
 * TODO: Document seedSupportTicket.
 */
export async function seedSupportTicket(
  request: APIRequestContext,
  token: string,
  options: {
    subject: string;
    priority?: "low" | "normal" | "high" | "urgent";
    status?: "open" | "in_progress" | "waiting_on_customer" | "resolved" | "closed";
    tenantId?: string;
    userId?: string;
    initialMessage?: string;
  },
): Promise<{ id: string; subject: string; priority: string; status: string }> {
  const tenantSQL = options.tenantId ? `'${sqlLiteral(options.tenantId)}'` : "NULL";
  const userSQL = options.userId ? `'${sqlLiteral(options.userId)}'` : "NULL";
  const status = options.status || "open";
  const priority = options.priority || "normal";

  const ticketResult = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_support_tickets (tenant_id, user_id, subject, status, priority)
     VALUES (${tenantSQL}, ${userSQL}, '${sqlLiteral(options.subject)}', '${sqlLiteral(status)}', '${sqlLiteral(priority)}')
     RETURNING id, subject, priority, status`,
  );
  const id = ticketResult.rows[0]?.[0];
  const subject = ticketResult.rows[0]?.[1];
  const returnedPriority = ticketResult.rows[0]?.[2];
  const returnedStatus = ticketResult.rows[0]?.[3];
  if (
    typeof id !== "string" ||
    typeof subject !== "string" ||
    typeof returnedPriority !== "string" ||
    typeof returnedStatus !== "string"
  ) {
    throw new Error(`Expected seeded support ticket fields for subject ${options.subject}`);
  }

  const initialMessage = options.initialMessage || "Initial customer message";
  await execSQL(
    request,
    token,
    `INSERT INTO _ayb_support_messages (ticket_id, sender_type, body)
     VALUES ('${sqlLiteral(id)}', 'customer', '${sqlLiteral(initialMessage)}')`,
  );

  return { id, subject, priority: returnedPriority, status: returnedStatus };
}

export async function cleanupSupportTicketByID(
  request: APIRequestContext,
  token: string,
  ticketID: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_support_tickets WHERE id = '${sqlLiteral(ticketID)}'`,
  );
}

/**
 * TODO: Document seedIncident.
 */
export async function seedIncident(
  request: APIRequestContext,
  token: string,
  options: {
    title: string;
    status?: "investigating" | "identified" | "monitoring" | "resolved";
    affectedServices?: string[];
    initialUpdateMessage?: string;
    initialUpdateStatus?: "investigating" | "identified" | "monitoring" | "resolved";
  },
): Promise<{ id: string; title: string; status: string }> {
  const status = options.status || "investigating";
  const services = options.affectedServices || [];
  const affectedServicesSQL =
    services.length === 0
      ? "ARRAY[]::text[]"
      : `ARRAY[${services.map((serviceName) => `'${sqlLiteral(serviceName)}'`).join(", ")}]`;

  const incidentResult = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_incidents (title, status, affected_services)
     VALUES ('${sqlLiteral(options.title)}', '${sqlLiteral(status)}', ${affectedServicesSQL})
     RETURNING id, title, status`,
  );
  const id = incidentResult.rows[0]?.[0];
  const title = incidentResult.rows[0]?.[1];
  const returnedStatus = incidentResult.rows[0]?.[2];
  if (
    typeof id !== "string" ||
    typeof title !== "string" ||
    typeof returnedStatus !== "string"
  ) {
    throw new Error(`Expected seeded incident fields for title ${options.title}`);
  }

  if (options.initialUpdateMessage) {
    const updateStatus = options.initialUpdateStatus || returnedStatus;
    await execSQL(
      request,
      token,
      `INSERT INTO _ayb_incident_updates (incident_id, message, status)
       VALUES ('${sqlLiteral(id)}', '${sqlLiteral(options.initialUpdateMessage)}', '${sqlLiteral(updateStatus)}')`,
    );
  }

  return { id, title, status: returnedStatus };
}

export async function cleanupIncidentByID(
  request: APIRequestContext,
  token: string,
  incidentID: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_incidents WHERE id = '${sqlLiteral(incidentID)}'`,
  );
}

export async function cleanupNotificationsByTitle(
  request: APIRequestContext,
  token: string,
  title: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_notifications WHERE title = '${sqlLiteral(title)}'`,
  );
}

interface SeedRequestLogEntryOptions {
  method: string;
  path: string;
  statusCode: number;
  durationMs: number;
  timestampISO?: string;
  requestSize?: number;
  responseSize?: number;
  requestID?: string;
  ipAddress?: string;
}

/**
 * TODO: Document seedRequestLogEntry.
 */
export async function seedRequestLogEntry(
  request: APIRequestContext,
  token: string,
  options: SeedRequestLogEntryOptions,
): Promise<{ id: string; path: string; method: string; statusCode: number; durationMs: number }> {
  const statusCodeValue = assertSafeSQLInteger(options.statusCode, "statusCode");
  const durationMsValue = assertSafeSQLInteger(options.durationMs, "durationMs");
  const requestSizeValue = assertSafeSQLInteger(options.requestSize ?? 0, "requestSize");
  const responseSizeValue = assertSafeSQLInteger(options.responseSize ?? 0, "responseSize");
  const timestampSQL = options.timestampISO
    ? `'${sqlLiteral(options.timestampISO)}'::timestamptz`
    : "NOW()";
  const requestIDSQL = options.requestID
    ? `'${sqlLiteral(options.requestID)}'`
    : "NULL";
  const ipAddressSQL = options.ipAddress
    ? `'${sqlLiteral(options.ipAddress)}'::inet`
    : "NULL";

  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_request_logs (
       timestamp, method, path, status_code, duration_ms, request_size, response_size, request_id, ip_address
     )
     VALUES (
       ${timestampSQL},
       '${sqlLiteral(options.method)}',
       '${sqlLiteral(options.path)}',
       ${statusCodeValue},
       ${durationMsValue},
       ${requestSizeValue},
       ${responseSizeValue},
       ${requestIDSQL},
       ${ipAddressSQL}
     )
     RETURNING id::text, path, method, status_code, duration_ms`,
  );

  const id = result.rows[0]?.[0];
  const path = result.rows[0]?.[1];
  const method = result.rows[0]?.[2];
  const statusCode = result.rows[0]?.[3];
  const durationMs = result.rows[0]?.[4];
  if (
    typeof id !== "string" ||
    typeof path !== "string" ||
    typeof method !== "string" ||
    typeof statusCode !== "number" ||
    typeof durationMs !== "number"
  ) {
    throw new Error(`Expected seeded request log fields for path ${options.path}`);
  }

  return { id, path, method, statusCode, durationMs };
}

export async function cleanupRequestLogsByPath(
  request: APIRequestContext,
  token: string,
  path: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_request_logs WHERE path = '${sqlLiteral(path)}'`,
  );
}

interface SeedAuditLogEntryOptions {
  tableName: string;
  operation: "INSERT" | "UPDATE" | "DELETE";
  recordID?: unknown;
  oldValues?: unknown;
  newValues?: unknown;
  timestampISO?: string;
  ipAddress?: string;
}

/**
 * TODO: Document seedAuditLogEntry.
 */
export async function seedAuditLogEntry(
  request: APIRequestContext,
  token: string,
  options: SeedAuditLogEntryOptions,
): Promise<{ id: string; tableName: string; operation: "INSERT" | "UPDATE" | "DELETE" }> {
  const timestampSQL = options.timestampISO
    ? `'${sqlLiteral(options.timestampISO)}'::timestamptz`
    : "NOW()";
  const recordIDSQL = options.recordID === undefined
    ? "NULL"
    : `'${sqlLiteral(JSON.stringify(options.recordID))}'::jsonb`;
  const oldValuesSQL = options.oldValues === undefined
    ? "NULL"
    : `'${sqlLiteral(JSON.stringify(options.oldValues))}'::jsonb`;
  const newValuesSQL = options.newValues === undefined
    ? "NULL"
    : `'${sqlLiteral(JSON.stringify(options.newValues))}'::jsonb`;
  const ipAddressSQL = options.ipAddress
    ? `'${sqlLiteral(options.ipAddress)}'::inet`
    : "NULL";

  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_audit_log (
       timestamp, table_name, operation, record_id, old_values, new_values, ip_address
     )
     VALUES (
       ${timestampSQL},
       '${sqlLiteral(options.tableName)}',
       '${sqlLiteral(options.operation)}',
       ${recordIDSQL},
       ${oldValuesSQL},
       ${newValuesSQL},
       ${ipAddressSQL}
     )
     RETURNING id::text, table_name, operation`,
  );

  const id = result.rows[0]?.[0];
  const tableName = result.rows[0]?.[1];
  const operation = result.rows[0]?.[2];
  if (
    typeof id !== "string" ||
    typeof tableName !== "string" ||
    (operation !== "INSERT" && operation !== "UPDATE" && operation !== "DELETE")
  ) {
    throw new Error(`Expected seeded audit log fields for table ${options.tableName}`);
  }

  return { id, tableName, operation };
}

export async function cleanupAuditLogsByTable(
  request: APIRequestContext,
  token: string,
  tableName: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_audit_log WHERE table_name = '${sqlLiteral(tableName)}'`,
  );
}

export interface AdminStatsSnapshot {
  uptime_seconds: number;
  go_version: string;
  goroutines: number;
  memory_alloc: number;
  memory_sys: number;
  gc_cycles: number;
  db_pool_total?: number;
  db_pool_idle?: number;
  db_pool_in_use?: number;
  db_pool_max?: number;
}

/**
 * TODO: Document fetchAdminStatsSnapshot.
 */
export async function fetchAdminStatsSnapshot(
  request: APIRequestContext,
  token: string,
): Promise<AdminStatsSnapshot> {
  const res = await request.get("/api/admin/stats", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch admin stats snapshot");

  const body = await res.json();
  if (
    typeof body?.uptime_seconds !== "number" ||
    typeof body?.go_version !== "string" ||
    typeof body?.goroutines !== "number" ||
    typeof body?.memory_alloc !== "number" ||
    typeof body?.memory_sys !== "number" ||
    typeof body?.gc_cycles !== "number"
  ) {
    throw new Error("Expected numeric/string runtime metrics from /api/admin/stats");
  }

  return body as AdminStatsSnapshot;
}

/**
 * TODO: Document seedEmailTemplate.
 */
export async function seedEmailTemplate(
  request: APIRequestContext,
  token: string,
  options: {
    key: string;
    subjectTemplate: string;
    htmlTemplate: string;
  },
): Promise<{
  templateKey: string;
  subjectTemplate: string;
  htmlTemplate: string;
  enabled: boolean;
}> {
  const res = await request.put(`/api/admin/email/templates/${encodeURIComponent(options.key)}`, {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: {
      subjectTemplate: options.subjectTemplate,
      htmlTemplate: options.htmlTemplate,
    },
  });
  await validateResponse(res, `Seed email template ${options.key}`);

  const body = await res.json();
  if (
    typeof body?.templateKey !== "string" ||
    typeof body?.subjectTemplate !== "string" ||
    typeof body?.htmlTemplate !== "string" ||
    typeof body?.enabled !== "boolean"
  ) {
    throw new Error(`Expected seeded email template fields for key ${options.key}`);
  }

  return {
    templateKey: body.templateKey,
    subjectTemplate: body.subjectTemplate,
    htmlTemplate: body.htmlTemplate,
    enabled: body.enabled,
  };
}

export async function cleanupEmailTemplate(
  request: APIRequestContext,
  token: string,
  key: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/email/templates/${encodeURIComponent(key)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() === 404 || res.status() === 400) {
    return;
  }
  await validateResponse(res, `Delete email template ${key}`);
}

export interface AuthSettingsSnapshot {
  totp_enabled: boolean;
  anonymous_auth_enabled: boolean;
  email_mfa_enabled: boolean;
  sms_enabled: boolean;
  magic_link_enabled: boolean;
  [key: string]: unknown;
}

export async function fetchAuthSettings(
  request: APIRequestContext,
  token: string,
): Promise<AuthSettingsSnapshot> {
  const res = await request.get("/api/admin/auth-settings", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch auth settings");
  return await res.json();
}

export interface RealtimeStatsSnapshot {
  version: string;
  timestamp: string;
  connections: { sse: number; ws: number; total: number };
  counters: { dropped_messages: number; heartbeat_failures: number };
}

export async function fetchRealtimeStats(
  request: APIRequestContext,
  token: string,
): Promise<RealtimeStatsSnapshot> {
  const res = await request.get("/api/admin/realtime/stats", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch realtime stats");
  return await res.json();
}

export interface SecurityAdvisorSnapshot {
  evaluatedAt: string;
  stale: boolean;
  findings: Array<{
    id: string;
    severity: "critical" | "high" | "medium" | "low";
    category: string;
    status: "open" | "accepted" | "resolved";
    title: string;
    description: string;
    remediation: string;
  }>;
}

/**
 * TODO: Document fetchSecurityAdvisorReport.
 */
export async function fetchSecurityAdvisorReport(
  request: APIRequestContext,
  token: string,
): Promise<SecurityAdvisorSnapshot> {
  const res = await request.get("/api/admin/advisors/security", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch security advisor report");
  const body = await res.json();
  if (
    typeof body?.evaluatedAt !== "string" ||
    typeof body?.stale !== "boolean" ||
    !Array.isArray(body?.findings)
  ) {
    throw new Error("Expected security advisor report shape from /api/admin/advisors/security");
  }
  return body as SecurityAdvisorSnapshot;
}

export interface PerformanceAdvisorSnapshot {
  generatedAt: string;
  stale: boolean;
  range: "15m" | "1h" | "6h" | "24h" | "7d";
  queries: Array<{
    fingerprint: string;
    normalizedQuery: string;
    meanMs: number;
    totalMs: number;
    calls: number;
    rows: number;
    endpoints: string[];
    trend: "up" | "down" | "flat";
  }>;
}

/**
 * TODO: Document fetchPerformanceAdvisorReport.
 */
export async function fetchPerformanceAdvisorReport(
  request: APIRequestContext,
  token: string,
  range: "15m" | "1h" | "6h" | "24h" | "7d" = "1h",
): Promise<PerformanceAdvisorSnapshot> {
  const res = await request.get(`/api/admin/advisors/performance?range=${encodeURIComponent(range)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch performance advisor report");
  const body = await res.json();
  if (
    typeof body?.generatedAt !== "string" ||
    typeof body?.stale !== "boolean" ||
    typeof body?.range !== "string" ||
    !Array.isArray(body?.queries)
  ) {
    throw new Error("Expected performance advisor report shape from /api/admin/advisors/performance");
  }
  return body as PerformanceAdvisorSnapshot;
}

/** Trigger an admin stats request with a unique request ID for smoke-test log assertions. */
export async function triggerAdminStatsRequest(
  request: APIRequestContext,
  token: string,
): Promise<string> {
  const requestId = `admin-logs-${randomUUID()}`;
  const res = await request.get("/api/admin/stats", {
    headers: {
      Authorization: `Bearer ${token}`,
      "X-Request-Id": requestId,
    },
  });
  await validateResponse(res, "Trigger admin stats request");
  return requestId;
}
