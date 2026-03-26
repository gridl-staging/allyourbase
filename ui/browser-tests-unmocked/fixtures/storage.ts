/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar22_01_dashboard_journey_e2e_tests/allyourbase_dev/ui/browser-tests-unmocked/fixtures/storage.ts.
 */
import type { APIRequestContext } from "@playwright/test";
import { validateResponse } from "./core";

/**
 * TODO: Document seedWebhook.
 */
export async function seedWebhook(
  request: APIRequestContext,
  token: string,
  url: string,
): Promise<{ id: number; url: string }> {
  const res = await request.post("/api/webhooks", {
    headers: { Authorization: `Bearer ${token}` },
    data: { url, events: ["create"], enabled: true },
  });
  await validateResponse(res, `Create webhook for ${url}`);
  const body = await res.json();
  if (!body.id) {
    throw new Error("Webhook created but no ID in response");
  }
  return { id: body.id, url: body.url };
}

export async function deleteWebhook(
  request: APIRequestContext,
  token: string,
  id: number,
): Promise<void> {
  const res = await request.delete(`/api/webhooks/${id}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Delete webhook ${id}`);
}

/**
 * TODO: Document seedFile.
 */
export async function seedFile(
  request: APIRequestContext,
  token: string,
  bucket: string,
  fileName: string,
  content: string,
): Promise<{ name: string }> {
  const res = await request.post(`/api/storage/${encodeURIComponent(bucket)}`, {
    headers: { Authorization: `Bearer ${token}` },
    multipart: {
      file: {
        name: fileName,
        mimeType: "text/plain",
        buffer: Buffer.from(content),
      },
    },
  });
  await validateResponse(res, `Upload file ${fileName} to bucket ${bucket}`);
  const body = await res.json();
  if (!body.name) {
    throw new Error("File upload succeeded but no name in response");
  }
  return { name: body.name };
}

export async function deleteFile(
  request: APIRequestContext,
  token: string,
  bucket: string,
  fileName: string,
): Promise<void> {
  const res = await request.delete(
    `/api/storage/${encodeURIComponent(bucket)}/${encodeURIComponent(fileName)}`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  await validateResponse(res, `Delete file ${fileName} from bucket ${bucket}`);
}

export async function seedRecord(
  request: APIRequestContext,
  token: string,
  table: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const res = await request.post(`/api/collections/${encodeURIComponent(table)}`, {
    headers: { Authorization: `Bearer ${token}` },
    data,
  });
  await validateResponse(res, `Create record in table ${table}`);
  return await res.json();
}

export async function listRecords(
  request: APIRequestContext,
  token: string,
  table: string,
): Promise<Record<string, unknown>[]> {
  const res = await request.get(`/api/collections/${encodeURIComponent(table)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `List records in table ${table}`);
  const body = await res.json();
  if (!body || !Array.isArray(body.items)) {
    throw new Error(`List records in table ${table} returned invalid response shape`);
  }
  return body.items as Record<string, unknown>[];
}
