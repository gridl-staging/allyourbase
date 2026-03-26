import { beforeEach, describe, expect, it, vi } from "vitest";
import { listAdminLogs } from "../api_logs";

describe("admin logs API request paths", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it("calls GET /api/admin/logs without filter query params and includes admin auth header", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ entries: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await listAdminLogs();

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/logs", {
      headers: {
        Authorization: "Bearer admin-token",
      },
    });
  });

  it("normalizes backend payload into UI-safe entries and buffering state", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          entries: [
            {
              time: "2026-03-15T12:00:00Z",
              level: "INFO",
              message: "request",
              attrs: { path: "/api/admin/logs", status: 200 },
            },
            {
              time: "not-a-timestamp",
              level: "TRACE",
              message: "odd-level",
            },
          ],
          message: "log buffering not enabled",
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const result = await listAdminLogs();

    expect(result.message).toBe("log buffering not enabled");
    expect(result.bufferingEnabled).toBe(false);
    expect(result.entries).toHaveLength(2);

    expect(result.entries[0]).toEqual(
      expect.objectContaining({
        time: "2026-03-15T12:00:00Z",
        parsedTimeMs: Date.parse("2026-03-15T12:00:00Z"),
        level: "info",
        levelLabel: "INFO",
        message: "request",
        attrs: { path: "/api/admin/logs", status: 200 },
      }),
    );

    expect(result.entries[1]).toEqual(
      expect.objectContaining({
        time: "not-a-timestamp",
        parsedTimeMs: null,
        level: "unknown",
        levelLabel: "TRACE",
        message: "odd-level",
        attrs: {},
      }),
    );
  });

  it("derives stable unique row identity from backend fields, not poll cycle index", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          entries: [
            {
              time: "2026-03-15T12:00:00Z",
              level: "WARN",
              message: "duplicate",
              attrs: { code: 1 },
            },
            {
              time: "2026-03-15T12:00:00Z",
              level: "WARN",
              message: "duplicate",
              attrs: { code: 1 },
            },
          ],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const first = await listAdminLogs();

    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          entries: [
            {
              time: "2026-03-15T12:00:00Z",
              level: "WARN",
              message: "duplicate",
              attrs: { code: 1 },
            },
            {
              time: "2026-03-15T12:00:00Z",
              level: "WARN",
              message: "duplicate",
              attrs: { code: 1 },
            },
          ],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const second = await listAdminLogs();

    expect(first.entries.map((entry) => entry.id)).toEqual(second.entries.map((entry) => entry.id));
    expect(new Set(first.entries.map((entry) => entry.id)).size).toBe(2);
  });

  it("keeps stable row identity when attrs key order changes across polls", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        '{"entries":[{"time":"2026-03-15T12:00:00Z","level":"INFO","message":"request","attrs":{"path":"/api/admin/logs","status":200}}]}',
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const first = await listAdminLogs();

    fetchMock.mockResolvedValueOnce(
      new Response(
        '{"entries":[{"time":"2026-03-15T12:00:00Z","level":"INFO","message":"request","attrs":{"status":200,"path":"/api/admin/logs"}}]}',
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const second = await listAdminLogs();

    expect(first.entries).toHaveLength(1);
    expect(second.entries).toHaveLength(1);
    expect(first.entries[0].id).toBe(second.entries[0].id);
    expect(first.entries[0].attrsText).toBe(second.entries[0].attrsText);
  });

  it("treats malformed payloads as an empty normalized result instead of crashing", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response("null", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(listAdminLogs()).resolves.toEqual({
      entries: [],
      message: undefined,
      bufferingEnabled: true,
    });
  });

  it("treats malformed entry items as empty rows instead of crashing normalization", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          entries: [
            null,
            "bad-entry",
            {
              time: "2026-03-15T12:00:00Z",
              level: "INFO",
              message: "request",
              attrs: { path: "/api/admin/logs" },
            },
          ],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const result = await listAdminLogs();

    expect(result.entries).toHaveLength(3);
    expect(result.entries[0]).toEqual(
      expect.objectContaining({
        time: "",
        parsedTimeMs: null,
        level: "unknown",
        message: "",
        attrs: {},
      }),
    );
    expect(result.entries[1]).toEqual(
      expect.objectContaining({
        time: "",
        parsedTimeMs: null,
        level: "unknown",
        message: "",
        attrs: {},
      }),
    );
    expect(result.entries[2]).toEqual(
      expect.objectContaining({
        time: "2026-03-15T12:00:00Z",
        parsedTimeMs: Date.parse("2026-03-15T12:00:00Z"),
        level: "info",
        message: "request",
        attrs: { path: "/api/admin/logs" },
      }),
    );
  });
});
