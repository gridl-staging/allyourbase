/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_6_test_verification_and_lint/allyourbase_dev/ui/browser-tests-unmocked/fixtures/infra.ts.
 */
import type { APIRequestContext } from "@playwright/test";
import { execSQL, sqlLiteral, validateResponse } from "./core";

/**
 * TODO: Document seedSite.
 */
export async function seedSite(
  request: APIRequestContext,
  token: string,
  options: { name?: string; slug?: string } = {},
): Promise<{ id: string; name: string; slug: string }> {
  const runID = Date.now();
  const name = options.name || `smoke-site-${runID}`;
  const slug = options.slug || `smoke-site-${runID}`;
  const res = await request.post("/api/admin/sites", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { name, slug },
  });
  await validateResponse(res, `Create site ${name}`);
  const body = await res.json();
  if (
    typeof body?.id !== "string" ||
    typeof body?.name !== "string" ||
    typeof body?.slug !== "string"
  ) {
    throw new Error(`Expected seeded site fields id/name/slug for ${name}`);
  }
  return { id: body.id, name: body.name, slug: body.slug };
}

export async function cleanupSiteByID(
  request: APIRequestContext,
  token: string,
  siteID: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/sites/${encodeURIComponent(siteID)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete site ${siteID}`);
  }
}

/**
 * TODO: Document getSite.
 */
export async function getSite(
  request: APIRequestContext,
  token: string,
  siteID: string,
): Promise<{ id: string; name: string; slug: string; spaMode: boolean }> {
  const res = await request.get(`/api/admin/sites/${encodeURIComponent(siteID)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Get site ${siteID}`);
  const body = await res.json();

  const spaModeValue = body?.spaMode ?? body?.spa_mode;
  if (
    typeof body?.id !== "string" ||
    typeof body?.name !== "string" ||
    typeof body?.slug !== "string" ||
    typeof spaModeValue !== "boolean"
  ) {
    throw new Error(`Expected site payload id/name/slug/spaMode for ${siteID}`);
  }

  return {
    id: body.id,
    name: body.name,
    slug: body.slug,
    spaMode: spaModeValue,
  };
}

export async function getSiteStatus(
  request: APIRequestContext,
  token: string,
  siteID: string,
): Promise<number> {
  const res = await request.get(`/api/admin/sites/${encodeURIComponent(siteID)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  return res.status();
}

/**
 * TODO: Document seedCustomDomain.
 */
export async function seedCustomDomain(
  request: APIRequestContext,
  token: string,
  hostname: string,
  options: { environment?: string } = {},
): Promise<{
  id: string;
  hostname: string;
  status: string;
  environment: string;
  verificationRecord: string;
}> {
  const res = await request.post("/api/admin/domains", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { hostname, environment: options.environment || "staging" },
  });
  await validateResponse(res, `Create custom domain ${hostname}`);
  const body = await res.json();
  if (
    typeof body?.id !== "string" ||
    typeof body?.hostname !== "string" ||
    typeof body?.status !== "string" ||
    typeof body?.environment !== "string" ||
    typeof body?.verificationRecord !== "string"
  ) {
    throw new Error(`Expected seeded custom domain fields for ${hostname}`);
  }
  return {
    id: body.id,
    hostname: body.hostname,
    status: body.status,
    environment: body.environment,
    verificationRecord: body.verificationRecord,
  };
}

export async function cleanupCustomDomain(
  request: APIRequestContext,
  token: string,
  id: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/domains/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete custom domain ${id}`);
  }
}

/**
 * TODO: Document seedLogDrain.
 */
export async function seedLogDrain(
  request: APIRequestContext,
  token: string,
  options: {
    name?: string;
    type?: string;
    url?: string;
    batch_size?: number;
    flush_interval_sec?: number;
  },
): Promise<{ id: string; name: string }> {
  const name = options.name || `test-drain-${Date.now()}`;
  const res = await request.post("/api/admin/logging/drains", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      id: name,
      type: options.type || "http",
      url: options.url || "https://example.com/logs",
      batch_size: options.batch_size || 100,
      flush_interval_seconds: options.flush_interval_sec || 5,
    },
  });
  await validateResponse(res, `Create log drain ${name}`);
  const body = await res.json();
  if (typeof body?.id !== "string" || typeof body?.name !== "string") {
    throw new Error(`Expected seeded log drain fields for ${name}`);
  }
  return { id: body.id, name: body.name };
}

export async function cleanupLogDrain(
  request: APIRequestContext,
  token: string,
  id: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/logging/drains/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete log drain ${id}`);
  }
}

/**
 * TODO: Document seedSAMLProvider.
 */
export async function seedSAMLProvider(
  request: APIRequestContext,
  token: string,
  options: {
    name?: string;
    entity_id?: string;
    idp_metadata_xml?: string;
    idp_metadata_url?: string;
    metadata_url?: string;
  },
): Promise<{ name: string; entity_id: string }> {
  const name = options.name || `test-saml-${Date.now()}`;
  const entityId = options.entity_id || `urn:test:${name}`;
  const idpMetadataXML =
    options.idp_metadata_xml ||
    `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="${entityId}">
  <IDPSSODescriptor>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.test/${name}/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`;
  const idpMetadataURL = options.idp_metadata_url || options.metadata_url;
  const res = await request.post("/api/admin/auth/saml", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      name,
      entity_id: entityId,
      idp_metadata_xml: idpMetadataXML,
      ...(idpMetadataURL ? { idp_metadata_url: idpMetadataURL } : {}),
    },
  });
  await validateResponse(res, `Create SAML provider ${name}`);
  return { name, entity_id: entityId };
}

export async function cleanupSAMLProvider(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/auth/saml/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete SAML provider ${name}`);
  }
}

export async function seedSecret(
  request: APIRequestContext,
  token: string,
  name: string,
  value: string,
): Promise<void> {
  const res = await request.post("/api/admin/secrets", {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: { name, value },
  });
  await validateResponse(res, `Create secret ${name}`);
}

export async function cleanupSecret(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/secrets/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete secret ${name}`);
  }
}

export async function seedBranch(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<{ name: string }> {
  const res = await request.post("/api/admin/branches/", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { name },
  });
  await validateResponse(res, `Create branch ${name}`);
  return { name };
}

export async function cleanupBranch(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/branches/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete branch ${name}`);
  }
}

/**
 * TODO: Document seedReplica.
 */
export async function seedReplica(
  request: APIRequestContext,
  token: string,
  options: {
    name: string;
    host: string;
    port?: number;
    database: string;
    ssl_mode?: string;
    weight?: number;
    max_lag_bytes?: number;
  },
): Promise<{ name: string }> {
  const res = await request.post("/api/admin/replicas", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      name: options.name,
      host: options.host,
      port: options.port ?? 5432,
      database: options.database,
      ssl_mode: options.ssl_mode || "disable",
      weight: options.weight ?? 100,
      max_lag_bytes: options.max_lag_bytes ?? 0,
    },
  });
  await validateResponse(res, `Add replica ${options.name}`);
  return { name: options.name };
}

/**
 * TODO: Document fetchReplicaStatuses.
 */
export async function fetchReplicaStatuses(
  request: APIRequestContext,
  token: string,
): Promise<{ replicas: Array<{ url: string; state: string }> }> {
  const res = await request.get("/api/admin/replicas", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "List replicas");
  const body = await res.json();
  if (!Array.isArray(body?.replicas)) {
    throw new Error("Expected replicas array from /api/admin/replicas");
  }
  return {
    replicas: body.replicas.map((replica: unknown) => {
      if (
        !replica ||
        typeof replica !== "object" ||
        typeof (replica as { url?: unknown }).url !== "string" ||
        typeof (replica as { state?: unknown }).state !== "string"
      ) {
        throw new Error("Expected replica status entries with url/state fields");
      }
      return {
        url: (replica as { url: string }).url,
        state: (replica as { state: string }).state,
      };
    }),
  };
}

export async function cleanupReplicaByName(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/replicas/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Remove replica ${name}`);
  }
}

export async function enableExtension(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.post("/api/admin/extensions", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { name },
  });
  await validateResponse(res, `Enable extension ${name}`);
}

export async function disableExtension(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/extensions/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Disable extension ${name}`);
  }
}

/**
 * TODO: Document seedBackup.
 */
export async function seedBackup(
  request: APIRequestContext,
  token: string,
  options: {
    dbName: string;
    status?: string;
    backupType?: string;
    triggeredBy?: string;
    sizeBytes?: number;
  },
): Promise<{ id: string; dbName: string }> {
  const dbName = sqlLiteral(options.dbName);
  const status = sqlLiteral(options.status || "completed");
  const backupType = sqlLiteral(options.backupType || "logical");
  const triggeredBy = sqlLiteral(options.triggeredBy || "smoke-test");
  const sizeBytes = options.sizeBytes ?? 1048576;
  if (!Number.isSafeInteger(sizeBytes) || sizeBytes < 0) {
    throw new Error("sizeBytes must be a non-negative safe integer");
  }
  const result = await execSQL(
    request,
    token,
    `INSERT INTO _ayb_backups (db_name, status, backup_type, triggered_by, size_bytes, started_at, completed_at)
     VALUES ('${dbName}', '${status}', '${backupType}', '${triggeredBy}', ${sizeBytes}, NOW() - INTERVAL '5 minutes', NOW())
     RETURNING id`,
  );
  const id = result.rows[0]?.[0];
  if (typeof id !== "string") {
    throw new Error(`Expected backup id for db_name ${options.dbName}`);
  }
  return { id, dbName: options.dbName };
}

export async function cleanupBackupsByDbName(
  request: APIRequestContext,
  token: string,
  dbName: string,
): Promise<void> {
  await execSQL(
    request,
    token,
    `DELETE FROM _ayb_backups WHERE db_name = '${sqlLiteral(dbName)}'`,
  );
}

/**
 * TODO: Document seedFDWServer.
 */
export async function seedFDWServer(
  request: APIRequestContext,
  token: string,
  options: {
    name: string;
    fdwType?: string;
    host?: string;
    port?: number;
    dbname?: string;
    filename?: string;
    user?: string;
    password?: string;
  },
): Promise<{ name: string; fdwType: string }> {
  const fdwType = options.fdwType || "postgres_fdw";
  const serverOptions: Record<string, string> =
    fdwType === "postgres_fdw"
      ? {
          host: options.host || "localhost",
          port: String(options.port ?? 5432),
          dbname: options.dbname || "postgres",
        }
      : { filename: options.filename || "/dev/null" };
  const res = await request.post("/api/admin/fdw/servers", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      name: options.name,
      fdw_type: fdwType,
      options: serverOptions,
      ...(fdwType === "postgres_fdw"
        ? {
            user_mapping: {
              user: options.user || "postgres",
              password: options.password || "postgres",
            },
          }
        : {}),
    },
  });
  await validateResponse(res, `Create FDW server ${options.name}`);
  return { name: options.name, fdwType };
}

export async function cleanupFDWServer(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(
    `/api/admin/fdw/servers/${encodeURIComponent(name)}?cascade=true`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  if (res.status() !== 404) {
    await validateResponse(res, `Drop FDW server ${name}`);
  }
}

export async function seedAIPrompt(
  request: APIRequestContext,
  token: string,
  options: { name: string; template: string },
): Promise<{ id: string; name: string }> {
  const res = await request.post("/api/admin/ai/prompts", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { name: options.name, template: options.template },
  });
  await validateResponse(res, `Create AI prompt ${options.name}`);
  const body = await res.json();
  return { id: body.id, name: body.name };
}

/**
 * TODO: Document cleanupAIPromptByName.
 */
export async function cleanupAIPromptByName(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const listRes = await request.get("/api/admin/ai/prompts?perPage=100", {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!listRes.ok()) {
    return;
  }
  const body = await listRes.json();
  const prompts = body.prompts || body.items || [];
  for (const prompt of prompts) {
    if (prompt.name === name && prompt.id) {
      const delRes = await request.delete(`/api/admin/ai/prompts/${encodeURIComponent(prompt.id)}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (delRes.status() !== 404) {
        await validateResponse(delRes, `Delete AI prompt ${prompt.id}`);
      }
    }
  }
}

// Fixture helper: list backups via the admin API.
// Extracted from spec files to comply with eslint no-restricted-syntax rule.
export async function listBackups(
  request: APIRequestContext,
  adminToken: string,
): Promise<{ ok: boolean; backups: Array<{ db_name?: string; [key: string]: unknown }> }> {
  const res = await request.get("/api/admin/backups", {
    headers: { Authorization: `Bearer ${adminToken}` },
  });
  if (!res.ok()) {
    return { ok: false, backups: [] };
  }
  const body = await res.json();
  const backups = Array.isArray(body?.backups) ? body.backups : [];
  return { ok: true, backups };
}
