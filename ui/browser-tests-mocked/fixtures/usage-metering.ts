/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/usage-metering.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute } from "./core";

interface UsageQuerySnapshot {
  period: string;
  metric: string;
  granularity: string;
  group_by: string;
  from: string;
  to: string;
  sort: string;
  limit: string;
  offset: string;
}

export interface UsageMeteringMockState {
  listCalls: UsageQuerySnapshot[];
  trendCalls: UsageQuerySnapshot[];
  breakdownCalls: UsageQuerySnapshot[];
  limitsCalls: Array<{ tenantId: string; query: UsageQuerySnapshot }>;
}

function captureUsageQuery(url: URL): UsageQuerySnapshot {
  return {
    period: url.searchParams.get("period") ?? "",
    metric: url.searchParams.get("metric") ?? "",
    granularity: url.searchParams.get("granularity") ?? "",
    group_by: url.searchParams.get("group_by") ?? "",
    from: url.searchParams.get("from") ?? "",
    to: url.searchParams.get("to") ?? "",
    sort: url.searchParams.get("sort") ?? "",
    limit: url.searchParams.get("limit") ?? "",
    offset: url.searchParams.get("offset") ?? "",
  };
}

/**
 * TODO: Document mockUsageMeteringApis.
 */
export async function mockUsageMeteringApis(
  page: Page,
): Promise<UsageMeteringMockState> {
  const state: UsageMeteringMockState = {
    listCalls: [],
    trendCalls: [],
    breakdownCalls: [],
    limitsCalls: [],
  };

  await page.route("**/api/**", async (route) => {
    const method = route.request().method();
    const url = new URL(route.request().url());
    const path = url.pathname;

    if (method === "GET" && path === "/api/admin/usage") {
      state.listCalls.push(captureUsageQuery(url));
      return json(route, 200, {
        items: [
          {
            tenantId: "tenant-1",
            tenantName: "Tenant One",
            requestCount: 120,
            storageBytesUsed: 2048,
            bandwidthBytes: 4096,
            functionInvocations: 12,
            realtimePeakConnections: 8,
            jobRuns: 3,
          },
          {
            tenantId: "tenant-2",
            tenantName: "Tenant Two",
            requestCount: 80,
            storageBytesUsed: 1024,
            bandwidthBytes: 2048,
            functionInvocations: 6,
            realtimePeakConnections: 4,
            jobRuns: 1,
          },
        ],
        total: 120,
        limit: 50,
        offset: Number(url.searchParams.get("offset") ?? "0"),
      });
    }

    if (method === "GET" && path === "/api/admin/usage/trends") {
      const query = captureUsageQuery(url);
      state.trendCalls.push(query);
      const metric = url.searchParams.get("metric") ?? "api_requests";
      const storageSeries = [
        { timestamp: "2026-03-10T00:00:00Z", value: 1024 },
        { timestamp: "2026-03-11T00:00:00Z", value: 2048 },
      ];
      const requestSeries = [
        { timestamp: "2026-03-10T00:00:00Z", value: 40 },
        { timestamp: "2026-03-11T00:00:00Z", value: 60 },
      ];
      return json(route, 200, {
        metric,
        granularity: url.searchParams.get("granularity") ?? "day",
        items: metric === "storage_bytes" ? storageSeries : requestSeries,
      });
    }

    if (method === "GET" && path === "/api/admin/usage/breakdown") {
      const query = captureUsageQuery(url);
      state.breakdownCalls.push(query);
      const metric = url.searchParams.get("metric") ?? "api_requests";
      const storageBreakdown = [
        { key: "Tenant One", value: 2048 },
        { key: "Tenant Two", value: 1024 },
      ];
      const requestBreakdown = [
        { key: "Tenant One", value: 120 },
        { key: "Tenant Two", value: 80 },
      ];
      return json(route, 200, {
        metric,
        groupBy: url.searchParams.get("group_by") ?? "tenant",
        items: metric === "storage_bytes" ? storageBreakdown : requestBreakdown,
      });
    }

    const limitsMatch = path.match(/^\/api\/admin\/usage\/([^/]+)\/limits$/);
    if (method === "GET" && limitsMatch) {
      const tenantId = decodeURIComponent(limitsMatch[1]);
      state.limitsCalls.push({ tenantId, query: captureUsageQuery(url) });
      return json(route, 200, {
        plan: "pro",
        metrics: {
          api_requests: { limit: 1000, used: 120, remaining: 880 },
          storage_bytes: { limit: 10000, used: 2048, remaining: 7952 },
          bandwidth_bytes: { limit: 50000, used: 4096, remaining: 45904 },
          function_invocations: { limit: 2000, used: 12, remaining: 1988 },
        },
      });
    }

    if (await handleCommonAdminRoutes(route, method, path)) return;
    return unhandledMockedApiRoute(route, method, path);
  });

  return state;
}
