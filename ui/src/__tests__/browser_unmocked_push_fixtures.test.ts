import { describe, it, expect, vi } from "vitest";
import type { APIRequestContext } from "@playwright/test";
import {
  cleanupPushTestData,
  isPushEnabled,
  seedPushDelivery,
  seedPushDeviceToken,
} from "../../browser-tests-unmocked/fixtures";

function okResponse(body: unknown) {
  return {
    ok: () => true,
    status: () => 200,
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

function buildSQLRequestMock() {
  const queries: string[] = [];

  const request = {
    post: vi.fn(async (path: string, init?: { data?: { query?: string } }) => {
      expect(path).toBe("/api/admin/sql");
      const query = init?.data?.query || "";
      queries.push(query);
      if (query.includes("RETURNING id, token, provider, platform")) {
        return okResponse({
          columns: ["id", "token", "provider", "platform"],
          rows: [["device-001", "tok-001", "fcm", "android"]],
          rowCount: 1,
        });
      }
      if (query.includes("RETURNING id, title, status, device_token_id")) {
        return okResponse({
          columns: ["id", "title", "status", "device_token_id"],
          rows: [["delivery-001", "Push fixture lifecycle", "sent", "device-001"]],
          rowCount: 1,
        });
      }
      return okResponse({ columns: [], rows: [], rowCount: 0 });
    }),
  };

  return { request: request as unknown as APIRequestContext, queries };
}

describe("browser-unmocked push fixture helpers", () => {
  it("seedPushDeviceToken uses _ayb_device_tokens and app owner_user_id seed", async () => {
    const { request, queries } = buildSQLRequestMock();

    await seedPushDeviceToken(request, "admin-token", { tokenValue: "fixture-token-1" });

    expect(queries).toHaveLength(3);
    expect(queries[1]).toContain("INSERT INTO _ayb_apps");
    expect(queries[1]).toContain("owner_user_id");
    expect(queries[2]).toContain("INSERT INTO _ayb_device_tokens");
    expect(queries[2]).not.toContain("_ayb_push_device_tokens");
  });

  it("cleanupPushTestData deletes via _ayb_device_tokens references", async () => {
    const { request, queries } = buildSQLRequestMock();

    await cleanupPushTestData(request, "admin-token", "fixture-token");

    expect(queries).toHaveLength(2);
    expect(queries[0]).toContain("SELECT id FROM _ayb_device_tokens");
    expect(queries[1]).toContain("DELETE FROM _ayb_device_tokens");
    expect(queries[1]).not.toContain("_ayb_push_device_tokens");
  });

  it("isPushEnabled only treats 200 as enabled and 503 as disabled", async () => {
    const request401 = {
      get: vi.fn(async () => ({
        status: () => 401,
        text: async () => "unauthorized",
      })),
    } as unknown as APIRequestContext;
    await expect(isPushEnabled(request401, "bad-token")).rejects.toThrow(
      "Push enablement check failed with status 401",
    );

    const request503 = {
      get: vi.fn(async () => ({
        status: () => 503,
        text: async () => "Push service is not enabled",
      })),
    } as unknown as APIRequestContext;
    await expect(isPushEnabled(request503, "admin-token")).resolves.toBe(false);

    const request200 = {
      get: vi.fn(async () => ({
        status: () => 200,
        text: async () => '{"items":[]}',
      })),
    } as unknown as APIRequestContext;
    await expect(isPushEnabled(request200, "admin-token")).resolves.toBe(true);
  });

  it("escapes SQL literals in push fixture helpers", async () => {
    const { request, queries } = buildSQLRequestMock();

    await seedPushDeviceToken(request, "admin-token", {
      tokenValue: "abc'def",
      deviceName: "Stu's iPhone",
    });
    await cleanupPushTestData(request, "admin-token", "abc'def");

    expect(queries[2]).toContain("abc''def");
    expect(queries[2]).toContain("Stu''s iPhone");
    expect(queries[2]).not.toContain("abc'def");
    expect(queries[3]).toContain("abc''def");
    expect(queries[4]).toContain("abc''def");
  });

  it("seedPushDelivery inserts push delivery linked to seeded device token", async () => {
    const { request, queries } = buildSQLRequestMock();

    await seedPushDelivery(request, "admin-token", {
      tokenValue: "delivery-fixture-token",
      title: "Push fixture lifecycle",
      body: "Fixture seeded delivery body",
      status: "sent",
    });

    expect(queries).toHaveLength(4);
    expect(queries[2]).toContain("INSERT INTO _ayb_device_tokens");
    expect(queries[3]).toContain("INSERT INTO _ayb_push_deliveries");
    expect(queries[3]).toContain("device_token_id, app_id, user_id, provider");
    expect(queries[3]).toContain("'Push fixture lifecycle'");
    expect(queries[3]).toContain("'Fixture seeded delivery body'");
  });

  it("seedPushDelivery uses NOW() for sent status and NULL for non-sent", async () => {
    const { request: reqSent, queries: qSent } = buildSQLRequestMock();
    await seedPushDelivery(reqSent, "admin-token", {
      tokenValue: "sent-token",
      status: "sent",
    });
    expect(qSent[3]).toContain("NOW()");
    expect(qSent[3]).not.toContain("NULL");

    const { request: reqFailed, queries: qFailed } = buildSQLRequestMock();
    await seedPushDelivery(reqFailed, "admin-token", {
      tokenValue: "failed-token",
      status: "failed",
    });
    expect(qFailed[3]).toContain("NULL");
    expect(qFailed[3]).not.toContain("NOW()");
  });
});
