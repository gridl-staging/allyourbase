import { vi, describe, it, expect, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import userEvent from "@testing-library/user-event";
import { Jobs } from "../Jobs";
import {
  listJobs,
  getQueueStats,
  retryJob,
  cancelJob,
} from "../../api";
import type { JobListResponse, JobResponse, QueueStats } from "../../types";

vi.mock("../../api", () => ({
  listJobs: vi.fn(),
  getQueueStats: vi.fn(),
  retryJob: vi.fn(),
  cancelJob: vi.fn(),
}));

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
}));

const mockListJobs = vi.mocked(listJobs);
const mockGetQueueStats = vi.mocked(getQueueStats);
const mockRetryJob = vi.mocked(retryJob);
const mockCancelJob = vi.mocked(cancelJob);

function makeJob(overrides: Partial<JobResponse> = {}): JobResponse {
  return {
    id: "j1",
    type: "webhook_delivery_prune",
    payload: {},
    state: "queued",
    runAt: "2026-02-22T10:00:00Z",
    leaseUntil: null,
    workerId: null,
    attempts: 0,
    maxAttempts: 3,
    lastError: null,
    lastRunAt: null,
    idempotencyKey: null,
    scheduleId: null,
    createdAt: "2026-02-22T09:00:00Z",
    updatedAt: "2026-02-22T09:00:00Z",
    completedAt: null,
    canceledAt: null,
    ...overrides,
  };
}

function makeListResponse(items: JobResponse[]): JobListResponse {
  return { items, count: items.length };
}

function makeStats(overrides: Partial<QueueStats> = {}): QueueStats {
  return {
    queued: 1,
    running: 0,
    completed: 0,
    failed: 0,
    canceled: 0,
    oldestQueuedAgeSec: 12,
    ...overrides,
  };
}

describe("Jobs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetQueueStats.mockResolvedValue(makeStats());
    mockRetryJob.mockResolvedValue(makeJob({ state: "queued" }));
    mockCancelJob.mockResolvedValue(makeJob({ state: "canceled" }));
  });

  it("shows loading state", () => {
    mockListJobs.mockReturnValue(new Promise(() => {}));
    renderWithProviders(<Jobs />);
    expect(screen.getByText("Loading jobs...")).toBeInTheDocument();
  });

  it("renders jobs table and queue stats", async () => {
    mockListJobs.mockResolvedValueOnce(
      makeListResponse([
        makeJob({ id: "j1", state: "failed", lastError: "boom" }),
        makeJob({ id: "j2", state: "queued" }),
      ]),
    );

    renderWithProviders(<Jobs />);

    await waitFor(() => {
      expect(screen.getByText("Job Queue")).toBeInTheDocument();
      expect(screen.getAllByText("failed").length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText("queued").length).toBeGreaterThanOrEqual(1);
      expect(screen.getByText("boom")).toBeInTheDocument();
      expect(screen.getByText("Queued: 1")).toBeInTheDocument();
    });
  });

  it("keeps rendering jobs when queue stats fails", async () => {
    mockListJobs.mockResolvedValueOnce(
      makeListResponse([makeJob({ id: "j1", state: "failed", lastError: "boom" })]),
    );
    mockGetQueueStats.mockRejectedValueOnce(new Error("stats unavailable"));

    renderWithProviders(<Jobs />);

    await waitFor(() => {
      expect(screen.getByText("Job Queue")).toBeInTheDocument();
      expect(screen.getByText("boom")).toBeInTheDocument();
    });

    expect(screen.queryByText("Queued: 1")).not.toBeInTheDocument();
    expect(screen.queryByText("stats unavailable")).not.toBeInTheDocument();
  });

  it("applies state and type filters", async () => {
    mockListJobs.mockResolvedValue(makeListResponse([makeJob()]));

    renderWithProviders(<Jobs />);

    await waitFor(() => {
      expect(mockListJobs).toHaveBeenCalledWith({});
    });

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText("State"), "failed");
    await user.type(screen.getByLabelText("Type"), "webhook");
    await user.click(screen.getByRole("button", { name: "Apply Filters" }));

    await waitFor(() => {
      expect(mockListJobs).toHaveBeenLastCalledWith({
        state: "failed",
        type: "webhook",
      });
    });
  });

  it("shows first-visit empty state when queue has no jobs", async () => {
    mockListJobs.mockResolvedValueOnce(makeListResponse([]));
    renderWithProviders(<Jobs />);

    await waitFor(() => {
      expect(screen.getByText("No jobs in queue yet")).toBeInTheDocument();
      expect(
        screen.getByText(
          "Run a background task, webhook delivery, or scheduled job, then refresh to see it here.",
        ),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Refresh jobs" }),
      ).toBeInTheDocument();
    });
  });

  it("shows filtered empty state and clear filters action", async () => {
    mockListJobs.mockResolvedValue(makeListResponse([]));
    renderWithProviders(<Jobs />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(mockListJobs).toHaveBeenCalledWith({});
    });

    await user.selectOptions(screen.getByLabelText("State"), "failed");
    await user.type(screen.getByLabelText("Type"), "webhook");
    await user.click(screen.getByRole("button", { name: "Apply Filters" }));

    await waitFor(() => {
      expect(screen.getByText("No jobs match these filters")).toBeInTheDocument();
      expect(
        screen.getByText("Clear filters to see all jobs, or adjust state and type."),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Clear filters" }),
      ).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Clear filters" }));

    await waitFor(() => {
      expect(mockListJobs).toHaveBeenLastCalledWith({});
    });
  });

  it("retries a failed job", async () => {
    mockListJobs.mockResolvedValue(
      makeListResponse([makeJob({ id: "j-fail", state: "failed" })]),
    );

    renderWithProviders(<Jobs />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByLabelText("Retry job j-fail")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Retry job j-fail"));

    await waitFor(() => {
      expect(mockRetryJob).toHaveBeenCalledWith("j-fail");
    });
  });

  it("cancels a queued job", async () => {
    mockListJobs.mockResolvedValue(
      makeListResponse([makeJob({ id: "j-queued", state: "queued" })]),
    );

    renderWithProviders(<Jobs />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByLabelText("Cancel job j-queued")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Cancel job j-queued"));

    await waitFor(() => {
      expect(mockCancelJob).toHaveBeenCalledWith("j-queued");
    });
  });

  it("shows page error when jobs request fails even if stats succeed", async () => {
    mockListJobs.mockRejectedValueOnce(new Error("jobs unavailable"));

    renderWithProviders(<Jobs />);

    await waitFor(() => {
      expect(screen.getByText("jobs unavailable")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });
  });
});
