import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, screen, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { StatsOverview } from "../StatsOverview";

vi.mock("../../api_stats", () => ({
  getStats: vi.fn(),
}));

import * as api from "../../api_stats";

const mockStats = {
  uptime_seconds: 86400,
  go_version: "go1.22.1",
  goroutines: 42,
  memory_alloc: 52428800,
  memory_sys: 104857600,
  gc_cycles: 150,
  db_pool_total: 25,
  db_pool_idle: 15,
  db_pool_in_use: 10,
  db_pool_max: 50,
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.getStats as ReturnType<typeof vi.fn>).mockResolvedValue(mockStats);
});

describe("StatsOverview", () => {
  it("renders stat cards with uptime/goroutines/memory/DB pool values", async () => {
    renderWithProviders(<StatsOverview />);
    await waitFor(() => {
      expect(screen.getByText("42")).toBeInTheDocument();
    });
    expect(screen.getByText("go1.22.1")).toBeInTheDocument();
    expect(screen.getByText("150")).toBeInTheDocument();
    // DB pool values
    expect(screen.getByText("10")).toBeInTheDocument(); // in_use
    expect(screen.getByText("50")).toBeInTheDocument(); // max
  });

  it("shows error state on fetch failure", async () => {
    (api.getStats as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("Server down"));
    renderWithProviders(<StatsOverview />);
    await waitFor(() => {
      expect(screen.getByText("Server down")).toBeInTheDocument();
    });
  });

  it("polls for refreshed stats on the configured interval", async () => {
    vi.useFakeTimers();
    try {
      renderWithProviders(<StatsOverview />);

      await act(async () => {
        await Promise.resolve();
      });
      expect(api.getStats).toHaveBeenCalledTimes(1);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(5000);
      });

      expect(api.getStats).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});
