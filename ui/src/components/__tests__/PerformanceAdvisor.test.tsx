import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { PerformanceAdvisorReport } from "../../types";
import { PerformanceAdvisor } from "../PerformanceAdvisor";
import { toPanelError } from "../advisors/panelError";

vi.mock("../../api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../api")>();
  return { ...actual, getPerformanceAdvisorReport: vi.fn() };
});

import { ApiError, getPerformanceAdvisorReport } from "../../api";

const mockedGet = vi.mocked(getPerformanceAdvisorReport);

function makeReport(): PerformanceAdvisorReport {
  return {
    generatedAt: "2026-02-28T00:00:00Z",
    stale: false,
    range: "1h",
    queries: [
      {
        fingerprint: "fp1",
        normalizedQuery: "select * from posts where author_id = $1",
        meanMs: 52.5,
        totalMs: 2042,
        calls: 39,
        rows: 845,
        endpoints: ["GET /api/collections/posts"],
        trend: "up",
      },
    ],
  };
}

describe("PerformanceAdvisor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockedGet.mockResolvedValue(makeReport());
  });

  it("renders slow-query table and query details", async () => {
    const user = userEvent.setup();
    render(<PerformanceAdvisor />);

    await screen.findByText("Performance Advisor");
    expect(screen.getByText(/fp1/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /fp1/i }));
    expect(screen.getByText(/select \* from posts/i)).toBeInTheDocument();
    expect(screen.getByText(/GET \/api\/collections\/posts/i)).toBeInTheDocument();
  });

  it("updates time range and refetches", async () => {
    const user = userEvent.setup();
    render(<PerformanceAdvisor />);

    await screen.findByText("Performance Advisor");
    await user.selectOptions(screen.getByLabelText(/Time range/i), "24h");

    expect(mockedGet).toHaveBeenCalledWith(expect.objectContaining({ range: "24h" }));
  });

  it("renders empty and stale states", async () => {
    mockedGet.mockResolvedValueOnce({ ...makeReport(), stale: true, queries: [] });
    render(<PerformanceAdvisor />);

    await screen.findByText(/No slow queries/i);
    expect(screen.getByText(/Telemetry may be stale/i)).toBeInTheDocument();
  });

  it("shows shared panel error copy when the performance report request fails", async () => {
    const apiError = new ApiError(500, "query insights unavailable");
    mockedGet.mockRejectedValueOnce(apiError);
    render(<PerformanceAdvisor />);

    await screen.findByText(toPanelError(apiError));
  });
});
