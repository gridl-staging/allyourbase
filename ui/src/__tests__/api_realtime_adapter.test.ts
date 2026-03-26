import { beforeEach, describe, expect, it, vi } from "vitest";
import { getRealtimeInspectorSnapshot } from "../api_admin";
import { request } from "../api_client";

vi.mock("../api_client", async () => {
  const actual = await vi.importActual<typeof import("../api_client")>("../api_client");
  return {
    ...actual,
    request: vi.fn(),
  };
});

type RequestFn = typeof request;

describe("getRealtimeInspectorSnapshot", () => {
  const requestMock = vi.mocked(request as RequestFn);

  beforeEach(() => {
    requestMock.mockReset();
  });

  it("normalizes backend counters and flattens subscription maps", async () => {
    requestMock.mockResolvedValueOnce({
      version: "v1",
      timestamp: "2026-03-15T22:00:00Z",
      connections: { sse: 3, ws: 5, total: 8 },
      subscriptions: {
        tables: { users: 2, posts: 1 },
        channels: {
          broadcast: { "room:lobby": 4, "room:game": 2 },
          presence: { "room:lobby": 1 },
        },
      },
      counters: { dropped_messages: 5, heartbeat_failures: 2 },
    });

    const result = await getRealtimeInspectorSnapshot();

    expect(requestMock).toHaveBeenCalledWith("/api/admin/realtime/stats");
    expect(result.version).toBe("v1");
    expect(result.timestamp).toBe("2026-03-15T22:00:00Z");
    expect(result.connections).toEqual({ sse: 3, ws: 5, total: 8 });
    expect(result.counters).toEqual({ droppedMessages: 5, heartbeatFailures: 2 });
    expect(result.subscriptions).toEqual(
      expect.arrayContaining([
        { name: "users", type: "table", count: 2 },
        { name: "posts", type: "table", count: 1 },
        { name: "room:lobby", type: "broadcast", count: 4 },
        { name: "room:game", type: "broadcast", count: 2 },
        { name: "room:lobby", type: "presence", count: 1 },
      ]),
    );
  });

  it("returns empty subscriptions when backend maps are empty", async () => {
    requestMock.mockResolvedValueOnce({
      version: "v1",
      timestamp: "2026-03-15T22:00:00Z",
      connections: { sse: 0, ws: 0, total: 0 },
      subscriptions: {
        tables: {},
        channels: { broadcast: {}, presence: {} },
      },
      counters: { dropped_messages: 0, heartbeat_failures: 0 },
    });

    const result = await getRealtimeInspectorSnapshot();

    expect(result.subscriptions).toEqual([]);
    expect(result.counters).toEqual({ droppedMessages: 0, heartbeatFailures: 0 });
  });
});
