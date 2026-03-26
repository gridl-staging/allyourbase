import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  fetchUsageBreakdown,
  fetchUsageList,
  fetchUsageLimits,
  fetchUsageTrends,
} from "../api_usage";
import type { UsageQueryState } from "../types/usage";

function createDefaultQueryState(): UsageQueryState {
  return {
    period: "month",
    from: null,
    to: null,
    metric: "api_requests",
    granularity: "day",
    groupBy: "tenant",
    sort: {
      column: "request_count",
      direction: "desc",
    },
    pagination: {
      limit: 50,
      offset: 0,
    },
    selectedTenantId: "tenant-default",
  };
}

describe("usage API request paths", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it("calls GET /api/admin/usage with default query params and admin auth header", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          items: [
            {
              tenantId: "tenant-1",
              tenantName: "Tenant One",
              requestCount: 12,
              storageBytesUsed: 2048,
              bandwidthBytes: 512,
              functionInvocations: 7,
              realtimePeakConnections: 2,
              jobRuns: 3,
            },
          ],
          total: 1,
          limit: 50,
          offset: 0,
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const result = await fetchUsageList(createDefaultQueryState());

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/usage?period=month&sort=request_count%3Adesc&limit=50&offset=0",
      {
        headers: {
          Authorization: "Bearer admin-token",
        },
      },
    );
    expect(result).toEqual({
      items: [
        {
          tenantId: "tenant-1",
          tenantName: "Tenant One",
          requestCount: 12,
          storageBytesUsed: 2048,
          bandwidthBytes: 512,
          functionInvocations: 7,
          realtimePeakConnections: 2,
          jobRuns: 3,
        },
      ],
      total: 1,
      limit: 50,
      offset: 0,
    });
  });

  it("serializes trends request params from the shared query state", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          metric: "api_requests",
          granularity: "hour",
          items: [{ timestamp: "2026-03-10T00:00:00Z", value: 4 }],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const query = {
      ...createDefaultQueryState(),
      granularity: "hour",
      from: "2026-03-10",
      to: "2026-03-12",
    } satisfies UsageQueryState;

    const result = await fetchUsageTrends(query);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/usage/trends?metric=api_requests&granularity=hour&from=2026-03-10&to=2026-03-12",
      {
        headers: {
          Authorization: "Bearer admin-token",
        },
      },
    );
    expect(result).toEqual({
      metric: "api_requests",
      granularity: "hour",
      items: [{ timestamp: "2026-03-10T00:00:00Z", value: 4 }],
    });
  });

  it("serializes breakdown request params from the shared query state", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          metric: "api_requests",
          groupBy: "status_code",
          items: [{ key: "200", value: 10 }],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const query = {
      ...createDefaultQueryState(),
      groupBy: "status_code",
      pagination: { limit: 25, offset: 100 },
      period: "week",
    } satisfies UsageQueryState;

    const result = await fetchUsageBreakdown(query);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/usage/breakdown?metric=api_requests&group_by=status_code&period=week&limit=25",
      {
        headers: {
          Authorization: "Bearer admin-token",
        },
      },
    );
    expect(result).toEqual({
      metric: "api_requests",
      groupBy: "status_code",
      items: [{ key: "200", value: 10 }],
    });
  });

  it("serializes tenant usage limits path and query params from the shared query state", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          plan: "pro",
          metrics: {
            api_requests: { limit: 1000, used: 12, remaining: 988 },
          },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const query = {
      ...createDefaultQueryState(),
      selectedTenantId: "tenant/with space",
    } satisfies UsageQueryState;

    const result = await fetchUsageLimits(query);

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/usage/tenant%2Fwith%20space/limits?period=month", {
      headers: {
        Authorization: "Bearer admin-token",
      },
    });
    expect(result).toEqual({
      plan: "pro",
      metrics: {
        api_requests: { limit: 1000, used: 12, remaining: 988 },
        storage_bytes: { limit: 0, used: 0, remaining: 0 },
        bandwidth_bytes: { limit: 0, used: 0, remaining: 0 },
        function_invocations: { limit: 0, used: 0, remaining: 0 },
      },
    });
  });

  it("throws before making request when tenant usage limits are requested without selected tenant", async () => {
    const query = {
      ...createDefaultQueryState(),
      selectedTenantId: null,
    } satisfies UsageQueryState;

    await expect(fetchUsageLimits(query)).rejects.toThrow(
      "selectedTenantId is required to fetch usage limits",
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("propagates backend validation errors from list endpoint", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ code: 400, message: "invalid sort" }),
        {
          status: 400,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(fetchUsageList(createDefaultQueryState())).rejects.toThrow("invalid sort");
  });

  it("propagates backend validation errors from trends endpoint", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ code: 400, message: "invalid granularity" }),
        {
          status: 400,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(fetchUsageTrends(createDefaultQueryState())).rejects.toThrow("invalid granularity");
  });

  it("propagates backend validation errors from breakdown endpoint", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ code: 400, message: "invalid metric/group_by combination" }),
        {
          status: 400,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(fetchUsageBreakdown(createDefaultQueryState())).rejects.toThrow(
      "invalid metric/group_by combination",
    );
  });

  it("propagates backend validation errors from limits endpoint", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ code: 400, message: "invalid period or date range" }),
        {
          status: 400,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(fetchUsageLimits(createDefaultQueryState())).rejects.toThrow("invalid period or date range");
  });

  it("normalizes malformed list payloads into safe defaults", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response("null", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(fetchUsageList(createDefaultQueryState())).resolves.toEqual({
      items: [],
      total: 0,
      limit: 50,
      offset: 0,
    });
  });

  it("preserves explicit zero limit and offset values from list payloads", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          items: [],
          total: 0,
          limit: 0,
          offset: 0,
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(fetchUsageList(createDefaultQueryState())).resolves.toEqual({
      items: [],
      total: 0,
      limit: 0,
      offset: 0,
    });
  });

  it("normalizes malformed trends payloads into safe defaults", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ metric: 42, granularity: 88, items: [null] }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(fetchUsageTrends(createDefaultQueryState())).resolves.toEqual({
      metric: "api_requests",
      granularity: "day",
      items: [{ timestamp: "", value: 0 }],
    });
  });

  it("normalizes malformed breakdown payloads into safe defaults", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ metric: null, groupBy: [], items: [null] }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(fetchUsageBreakdown(createDefaultQueryState())).resolves.toEqual({
      metric: "api_requests",
      groupBy: "tenant",
      items: [{ key: "", value: 0 }],
    });
  });

  it("normalizes malformed limits payloads into safe defaults", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ plan: 123, metrics: { api_requests: null } }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(fetchUsageLimits(createDefaultQueryState())).resolves.toEqual({
      plan: "",
      metrics: {
        api_requests: { limit: 0, used: 0, remaining: 0 },
        storage_bytes: { limit: 0, used: 0, remaining: 0 },
        bandwidth_bytes: { limit: 0, used: 0, remaining: 0 },
        function_invocations: { limit: 0, used: 0, remaining: 0 },
      },
    });
  });
});
