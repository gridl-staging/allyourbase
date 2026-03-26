import { describe, expect, it } from "vitest";
import {
  asRecord,
  encodePathSegment,
  encodePathWithSlashes,
  normalizeAuthResponse,
  normalizeRealtimeEvent,
  normalizeStorageListResponse,
} from "./helpers";

describe("sdk helpers", () => {
  it("encodes URL path segments safely", () => {
    expect(encodePathSegment("posts/admin users")).toBe("posts%2Fadmin%20users");
    expect(encodePathWithSlashes("public/posts and comments")).toBe("public/posts%20and%20comments");
  });

  it("normalizes auth response and user fields", () => {
    const normalized = normalizeAuthResponse({
      token: "t",
      refreshToken: "r",
      user: {
        id: 7,
        email: "demo@example.com",
        email_verified: true,
        created_at: "2026-03-01T00:00:00Z",
      } as unknown,
    } as unknown);

    expect(normalized.token).toBe("t");
    expect(normalized.refreshToken).toBe("r");
    expect(normalized.user.id).toBe("7");
    expect(normalized.user.emailVerified).toBe(true);
    expect(normalized.user.createdAt).toBe("2026-03-01T00:00:00Z");
  });

  it("normalizes storage list defaults and realtime old_record", () => {
    const storage = normalizeStorageListResponse({
      items: [{ id: "1", bucket: "b", name: "n", size: 9, content_type: "text/plain" }] as never,
      totalItems: undefined as never,
    });
    expect(storage.totalItems).toBe(1);
    expect(storage.items[0].contentType).toBe("text/plain");

    const event = normalizeRealtimeEvent({
      action: "update",
      table: "posts",
      record: { id: "2" },
      old_record: { id: "1" },
    } as unknown);
    expect(event.oldRecord).toEqual({ id: "1" });
  });

  it("asRecord returns undefined for non-object values", () => {
    expect(asRecord(null)).toBeUndefined();
    expect(asRecord("x")).toBeUndefined();
    expect(asRecord({ ok: true })).toEqual({ ok: true });
  });
});
