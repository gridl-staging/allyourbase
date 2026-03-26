import { beforeEach, describe, expect, it, vi } from "vitest";
import { callRpc, executeApiExplorer, getRealtimeInspectorSnapshot } from "../api";

describe("admin API request helpers", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it("uses the shared admin request path for callRpc while preserving 204 responses", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await expect(callRpc("sync_users", { dryRun: true })).resolves.toEqual({
      status: 204,
      data: null,
    });

    expect(fetchMock).toHaveBeenCalledWith("/api/rpc/sync_users", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: JSON.stringify({ dryRun: true }),
    });
  });

  it("preserves unauthorized handling for callRpc failures", async () => {
    const unauthorizedListener = vi.fn();
    window.addEventListener("ayb:unauthorized", unauthorizedListener);
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ message: "rpc denied" }), {
        status: 401,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(callRpc("sync_users")).rejects.toThrow("rpc denied");

    expect(localStorage.getItem("ayb_admin_token")).toBeNull();
    expect(unauthorizedListener).toHaveBeenCalledTimes(1);
    window.removeEventListener("ayb:unauthorized", unauthorizedListener);
  });

  it("uses the shared admin request path for Api Explorer while keeping raw response handling", async () => {
    const nowSpy = vi.spyOn(performance, "now");
    nowSpy.mockReturnValueOnce(100).mockReturnValueOnce(145);
    fetchMock.mockResolvedValueOnce(
      new Response("raw text response", {
        status: 418,
        statusText: "I'm a teapot",
        headers: { "X-Trace": "trace-123" },
      }),
    );

    await expect(
      executeApiExplorer("POST", "/api/admin/sql/", '{"query":"select 1"}'),
    ).resolves.toEqual({
      status: 418,
      statusText: "I'm a teapot",
      headers: expect.objectContaining({
        "content-type": "text/plain;charset=UTF-8",
        "x-trace": "trace-123",
      }),
      body: "raw text response",
      durationMs: 45,
    });

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/sql/", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: '{"query":"select 1"}',
    });
    nowSpy.mockRestore();
  });

  it("calls /api/admin/realtime/stats and normalizes the live snapshot", async () => {
    const rawSnapshot = {
      version: "v1",
      timestamp: "2026-03-15T00:00:00Z",
      connections: { sse: 3, ws: 7, total: 10 },
      subscriptions: {
        tables: { public_posts: 4 },
        channels: {
          broadcast: { "room:lobby": 2 },
          presence: { "room:lobby": 1 },
        },
      },
      counters: { dropped_messages: 5, heartbeat_failures: 1 },
    };
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(rawSnapshot), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const result = await getRealtimeInspectorSnapshot();

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/realtime/stats", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(result).toEqual({
      version: "v1",
      timestamp: "2026-03-15T00:00:00Z",
      connections: { sse: 3, ws: 7, total: 10 },
      subscriptions: [
        { name: "public_posts", type: "table", count: 4 },
        { name: "room:lobby", type: "broadcast", count: 2 },
        { name: "room:lobby", type: "presence", count: 1 },
      ],
      counters: { droppedMessages: 5, heartbeatFailures: 1 },
    });
  });
});
