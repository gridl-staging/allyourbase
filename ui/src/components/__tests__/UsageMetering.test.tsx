import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithProviders } from "../../test-utils";
import { UsageMetering } from "../UsageMetering";
import {
  fetchUsageBreakdown,
  fetchUsageLimits,
  fetchUsageList,
  fetchUsageTrends,
} from "../../api_usage";
import type {
  UsageBreakdownResponse,
  UsageLimitsResponse,
  UsageListResponse,
  UsageTrendResponse,
} from "../../types/usage";

vi.mock("../../api_usage", () => ({
  fetchUsageList: vi.fn(),
  fetchUsageTrends: vi.fn(),
  fetchUsageBreakdown: vi.fn(),
  fetchUsageLimits: vi.fn(),
}));

const mockFetchUsageList = vi.mocked(fetchUsageList);
const mockFetchUsageTrends = vi.mocked(fetchUsageTrends);
const mockFetchUsageBreakdown = vi.mocked(fetchUsageBreakdown);
const mockFetchUsageLimits = vi.mocked(fetchUsageLimits);

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function makeListResponse(overrides: Partial<UsageListResponse> = {}): UsageListResponse {
  return {
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
    offset: 0,
    ...overrides,
  };
}

function makeTrendResponse(overrides: Partial<UsageTrendResponse> = {}): UsageTrendResponse {
  return {
    metric: "api_requests",
    granularity: "day",
    items: [
      { timestamp: "2026-03-10T00:00:00Z", value: 40 },
      { timestamp: "2026-03-11T00:00:00Z", value: 60 },
      { timestamp: "2026-03-12T00:00:00Z", value: 100 },
    ],
    ...overrides,
  };
}

function makeBreakdownResponse(
  overrides: Partial<UsageBreakdownResponse> = {},
): UsageBreakdownResponse {
  return {
    metric: "api_requests",
    groupBy: "tenant",
    items: [
      { key: "Tenant One", value: 120 },
      { key: "Tenant Two", value: 80 },
    ],
    ...overrides,
  };
}

function makeLimitsResponse(overrides: Partial<UsageLimitsResponse> = {}): UsageLimitsResponse {
  return {
    plan: "pro",
    metrics: {
      api_requests: { limit: 1000, used: 120, remaining: 880 },
      storage_bytes: { limit: 10_000, used: 2048, remaining: 7952 },
      bandwidth_bytes: { limit: 50_000, used: 4096, remaining: 45_904 },
      function_invocations: { limit: 2000, used: 12, remaining: 1988 },
    },
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockFetchUsageList.mockResolvedValue(makeListResponse());
  mockFetchUsageTrends.mockResolvedValue(makeTrendResponse());
  mockFetchUsageBreakdown.mockResolvedValue(makeBreakdownResponse());
  mockFetchUsageLimits.mockResolvedValue(makeLimitsResponse());
});

describe("UsageMetering", () => {
  it("renders first-load state while usage data is pending", () => {
    mockFetchUsageList.mockReturnValue(new Promise(() => {}));
    mockFetchUsageTrends.mockReturnValue(new Promise(() => {}));
    mockFetchUsageBreakdown.mockReturnValue(new Promise(() => {}));

    renderWithProviders(<UsageMetering />);

    expect(screen.getByText(/loading usage metering/i)).toBeInTheDocument();
  });

  it("renders aggregate table, trend chart, breakdown chart, and tenant limits", async () => {
    renderWithProviders(<UsageMetering />);

    await expect(screen.findAllByText("Tenant One")).resolves.toHaveLength(2);
    expect(screen.getAllByText("Tenant Two")).toHaveLength(2);
    expect(screen.getByRole("heading", { name: "Usage Trend" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Usage Breakdown" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Tenant Limits" })).toBeInTheDocument();
    await expect(screen.findByText("Plan: pro")).resolves.toBeInTheDocument();
    expect(screen.getAllByText("API Requests").length).toBeGreaterThanOrEqual(1);

    expect(mockFetchUsageLimits).toHaveBeenCalledWith(
      expect.objectContaining({ selectedTenantId: "tenant-1" }),
    );
  });

  it("renders empty state when aggregate usage returns no rows", async () => {
    mockFetchUsageList.mockResolvedValueOnce(makeListResponse({ items: [], total: 0 }));
    mockFetchUsageTrends.mockResolvedValueOnce(makeTrendResponse({ items: [] }));
    mockFetchUsageBreakdown.mockResolvedValueOnce(makeBreakdownResponse({ items: [] }));

    renderWithProviders(<UsageMetering />);

    await expect(screen.findByText(/no tenant usage rows/i)).resolves.toBeInTheDocument();
    expect(screen.getByText(/no trend data/i)).toBeInTheDocument();
    expect(screen.getByText(/no breakdown data/i)).toBeInTheDocument();
    expect(screen.getByText(/select a tenant to view limits/i)).toBeInTheDocument();
    expect(mockFetchUsageLimits).not.toHaveBeenCalled();
  });

  it("renders error state when initial load fails", async () => {
    mockFetchUsageList.mockRejectedValueOnce(new Error("usage list failed"));

    renderWithProviders(<UsageMetering />);

    await expect(screen.findByText("usage list failed")).resolves.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /retry usage data/i })).toBeInTheDocument();
  });

  it("keeps filters in sync and enforces backend-safe combinations", async () => {
    const user = userEvent.setup();

    renderWithProviders(<UsageMetering />);
    await expect(screen.findAllByText("Tenant One")).resolves.toHaveLength(2);

    await user.selectOptions(screen.getByLabelText("Granularity"), "hour");
    await user.selectOptions(screen.getByLabelText("Breakdown"), "status_code");

    await waitFor(() => {
      expect(mockFetchUsageTrends).toHaveBeenLastCalledWith(
        expect.objectContaining({ metric: "api_requests", granularity: "hour" }),
      );
      expect(mockFetchUsageBreakdown).toHaveBeenLastCalledWith(
        expect.objectContaining({ metric: "api_requests", groupBy: "status_code" }),
      );
    });

    await user.selectOptions(screen.getByLabelText("Metric"), "storage_bytes");

    await waitFor(() => {
      expect(screen.getByLabelText("Granularity")).toHaveValue("day");
      expect(screen.getByLabelText("Breakdown")).toHaveValue("tenant");
      expect(mockFetchUsageTrends).toHaveBeenLastCalledWith(
        expect.objectContaining({ metric: "storage_bytes", granularity: "day" }),
      );
      expect(mockFetchUsageBreakdown).toHaveBeenLastCalledWith(
        expect.objectContaining({ metric: "storage_bytes", groupBy: "tenant" }),
      );
    });
  });

  it("renders distinct hour labels for hourly api request trends", async () => {
    const user = userEvent.setup();
    mockFetchUsageTrends.mockImplementation(async (query) => {
      if (query.granularity === "hour") {
        return makeTrendResponse({
          granularity: "hour",
          items: [
            { timestamp: "2026-03-10T00:00:00Z", value: 40 },
            { timestamp: "2026-03-10T01:00:00Z", value: 60 },
          ],
        });
      }
      return makeTrendResponse();
    });

    renderWithProviders(<UsageMetering />);
    await expect(screen.findAllByText("Tenant One")).resolves.toHaveLength(2);

    await user.selectOptions(screen.getByLabelText("Granularity"), "hour");

    await waitFor(() => {
      expect(mockFetchUsageTrends).toHaveBeenLastCalledWith(
        expect.objectContaining({ granularity: "hour" }),
      );
      expect(screen.getByText("Mar 10, 12:00 AM UTC")).toBeInTheDocument();
      expect(screen.getByText("Mar 10, 1:00 AM UTC")).toBeInTheDocument();
    });
  });

  it("updates list sort and pagination through shared query state", async () => {
    const user = userEvent.setup();
    renderWithProviders(<UsageMetering />);

    await expect(screen.findAllByText("Tenant One")).resolves.toHaveLength(2);

    await user.click(screen.getByRole("button", { name: /sort by tenant name/i }));
    await waitFor(() => {
      expect(mockFetchUsageList).toHaveBeenLastCalledWith(
        expect.objectContaining({
          sort: {
            column: "tenant_name",
            direction: "asc",
          },
        }),
      );
    });

    await user.click(screen.getByRole("button", { name: /next page/i }));
    await waitFor(() => {
      expect(mockFetchUsageList).toHaveBeenLastCalledWith(
        expect.objectContaining({
          pagination: {
            limit: 50,
            offset: 50,
          },
        }),
      );
    });

    await user.click(screen.getByRole("button", { name: /previous page/i }));
    await waitFor(() => {
      expect(mockFetchUsageList).toHaveBeenLastCalledWith(
        expect.objectContaining({
          pagination: {
            limit: 50,
            offset: 0,
          },
        }),
      );
    });

    await user.click(screen.getByRole("button", { name: /sort by tenant name/i }));
    await waitFor(() => {
      expect(mockFetchUsageList).toHaveBeenLastCalledWith(
        expect.objectContaining({
          sort: {
            column: "tenant_name",
            direction: "desc",
          },
        }),
      );
    });
  });

  it("applies period filter across all usage endpoints", async () => {
    const user = userEvent.setup();
    renderWithProviders(<UsageMetering />);

    await expect(screen.findAllByText("Tenant One")).resolves.toHaveLength(2);

    await user.selectOptions(screen.getByLabelText("Period"), "week");

    await waitFor(() => {
      expect(mockFetchUsageList).toHaveBeenLastCalledWith(
        expect.objectContaining({ period: "week" }),
      );
      expect(mockFetchUsageTrends).toHaveBeenLastCalledWith(
        expect.objectContaining({ period: "week" }),
      );
      expect(mockFetchUsageBreakdown).toHaveBeenLastCalledWith(
        expect.objectContaining({ period: "week" }),
      );
      expect(mockFetchUsageLimits).toHaveBeenLastCalledWith(
        expect.objectContaining({ period: "week" }),
      );
    });
  });

  it("re-fetches all usage data when the Refresh button is clicked", async () => {
    const user = userEvent.setup();
    renderWithProviders(<UsageMetering />);

    await expect(screen.findAllByText("Tenant One")).resolves.toHaveLength(2);
    await expect(screen.findByText("Plan: pro")).resolves.toBeInTheDocument();

    const listCallsBefore = mockFetchUsageList.mock.calls.length;
    const trendCallsBefore = mockFetchUsageTrends.mock.calls.length;
    const breakdownCallsBefore = mockFetchUsageBreakdown.mock.calls.length;
    const limitsCallsBefore = mockFetchUsageLimits.mock.calls.length;

    await user.click(screen.getByRole("button", { name: /refresh/i }));

    await waitFor(() => {
      expect(mockFetchUsageList.mock.calls.length).toBe(listCallsBefore + 1);
      expect(mockFetchUsageTrends.mock.calls.length).toBe(trendCallsBefore + 1);
      expect(mockFetchUsageBreakdown.mock.calls.length).toBe(breakdownCallsBefore + 1);
      expect(mockFetchUsageLimits.mock.calls.length).toBe(limitsCallsBefore + 1);
    });
  });

  it("falls back to the first available tenant when selected tenant leaves the filtered page", async () => {
    const user = userEvent.setup();
    const [tenantOne, tenantTwo] = makeListResponse().items;

    mockFetchUsageList.mockImplementation(async (query) => {
      if (query.period === "week") {
        return makeListResponse({
          items: [tenantOne],
          total: 1,
        });
      }
      return makeListResponse({
        items: [tenantOne, tenantTwo],
        total: 2,
      });
    });

    mockFetchUsageLimits.mockImplementation(async (query) =>
      makeLimitsResponse({
        plan: query.selectedTenantId === "tenant-2" ? "tenant-two-plan" : "tenant-one-plan",
      }),
    );

    renderWithProviders(<UsageMetering />);

    await expect(screen.findByText("Plan: tenant-one-plan")).resolves.toBeInTheDocument();
    await user.click(screen.getByRole("cell", { name: "Tenant Two" }));
    await expect(screen.findByText("Plan: tenant-two-plan")).resolves.toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText("Period"), "week");

    await waitFor(() => {
      expect(mockFetchUsageLimits).toHaveBeenLastCalledWith(
        expect.objectContaining({
          selectedTenantId: "tenant-1",
          period: "week",
        }),
      );
    });
    await expect(screen.findByText("Plan: tenant-one-plan")).resolves.toBeInTheDocument();
  });

  it("clears stale tenant limits while a different tenant is loading", async () => {
    const user = userEvent.setup();
    const deferredTenantTwoLimits = createDeferred<UsageLimitsResponse>();

    mockFetchUsageLimits.mockImplementation((query) => {
      if (query.selectedTenantId === "tenant-2") {
        return deferredTenantTwoLimits.promise;
      }
      return Promise.resolve(makeLimitsResponse({ plan: "starter" }));
    });

    renderWithProviders(<UsageMetering />);

    await expect(screen.findByText("Plan: starter")).resolves.toBeInTheDocument();

    await user.click(screen.getByRole("cell", { name: "Tenant Two" }));

    await waitFor(() => {
      expect(mockFetchUsageLimits).toHaveBeenLastCalledWith(
        expect.objectContaining({ selectedTenantId: "tenant-2" }),
      );
      expect(screen.getByText("Loading tenant limits...")).toBeInTheDocument();
      expect(screen.queryByText("Plan: starter")).not.toBeInTheDocument();
    });

    deferredTenantTwoLimits.resolve(makeLimitsResponse({ plan: "enterprise" }));

    await expect(screen.findByText("Plan: enterprise")).resolves.toBeInTheDocument();
  });
});
