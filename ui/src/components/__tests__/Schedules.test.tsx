import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Schedules } from "../Schedules";
import {
  listSchedules,
  createSchedule,
  updateSchedule,
  deleteSchedule,
  enableSchedule,
  disableSchedule,
} from "../../api";
import type { ScheduleListResponse, ScheduleResponse } from "../../types";

vi.mock("../../api", () => ({
  listSchedules: vi.fn(),
  createSchedule: vi.fn(),
  updateSchedule: vi.fn(),
  deleteSchedule: vi.fn(),
  enableSchedule: vi.fn(),
  disableSchedule: vi.fn(),
}));

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
}));

const mockListSchedules = vi.mocked(listSchedules);
const mockCreateSchedule = vi.mocked(createSchedule);
const mockUpdateSchedule = vi.mocked(updateSchedule);
const mockDeleteSchedule = vi.mocked(deleteSchedule);
const mockEnableSchedule = vi.mocked(enableSchedule);
const mockDisableSchedule = vi.mocked(disableSchedule);

function makeSchedule(overrides: Partial<ScheduleResponse> = {}): ScheduleResponse {
  return {
    id: "s1",
    name: "session_cleanup_hourly",
    jobType: "stale_session_cleanup",
    payload: {},
    cronExpr: "0 * * * *",
    timezone: "UTC",
    enabled: true,
    maxAttempts: 3,
    nextRunAt: "2026-02-22T11:00:00Z",
    lastRunAt: "2026-02-22T10:00:00Z",
    createdAt: "2026-02-22T09:00:00Z",
    updatedAt: "2026-02-22T09:00:00Z",
    ...overrides,
  };
}

function makeListResponse(items: ScheduleResponse[]): ScheduleListResponse {
  return { items, count: items.length };
}

describe("Schedules", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListSchedules.mockResolvedValue(makeListResponse([makeSchedule()]));
    mockCreateSchedule.mockResolvedValue(makeSchedule());
    mockUpdateSchedule.mockResolvedValue(makeSchedule());
    mockEnableSchedule.mockResolvedValue(makeSchedule({ enabled: true }));
    mockDisableSchedule.mockResolvedValue(makeSchedule({ enabled: false }));
    mockDeleteSchedule.mockResolvedValue();
  });

  it("renders schedules table", async () => {
    render(<Schedules />);

    await waitFor(() => {
      expect(screen.getByText("Job Schedules")).toBeInTheDocument();
      expect(screen.getByText("session_cleanup_hourly")).toBeInTheDocument();
      expect(screen.getByText("0 * * * *")).toBeInTheDocument();
    });
  });

  it("toggles enabled state", async () => {
    mockListSchedules.mockResolvedValueOnce(
      makeListResponse([makeSchedule({ id: "s-disabled", enabled: false })]),
    );

    render(<Schedules />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByLabelText("Enable schedule s-disabled")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Enable schedule s-disabled"));

    await waitFor(() => {
      expect(mockEnableSchedule).toHaveBeenCalledWith("s-disabled");
    });
  });

  it("shows cron validation error in create modal", async () => {
    render(<Schedules />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByText("Create Schedule")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Create Schedule"));
    await user.type(screen.getByLabelText("Name"), "nightly_job");
    await user.type(screen.getByLabelText("Job Type"), "expired_auth_cleanup");
    await user.clear(screen.getByLabelText("Cron Expression"));
    await user.type(screen.getByLabelText("Cron Expression"), "invalid");

    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(screen.getByText("Cron expression must have 5 fields.")).toBeInTheDocument();
    expect(mockCreateSchedule).not.toHaveBeenCalled();
  });

  it("creates a schedule from modal", async () => {
    render(<Schedules />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByText("Create Schedule")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Create Schedule"));
    await user.type(screen.getByLabelText("Name"), "nightly_job");
    await user.type(screen.getByLabelText("Job Type"), "expired_auth_cleanup");
    await user.clear(screen.getByLabelText("Cron Expression"));
    await user.type(screen.getByLabelText("Cron Expression"), "0 5 * * *");

    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockCreateSchedule).toHaveBeenCalledWith({
        name: "nightly_job",
        jobType: "expired_auth_cleanup",
        cronExpr: "0 5 * * *",
        timezone: "UTC",
        payload: {},
        enabled: true,
      });
    });
  });

  it("updates and deletes a schedule", async () => {
    render(<Schedules />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByLabelText("Edit schedule s1")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Edit schedule s1"));
    await user.clear(screen.getByLabelText("Cron Expression"));
    await user.type(screen.getByLabelText("Cron Expression"), "15 * * * *");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateSchedule).toHaveBeenCalledWith("s1", {
        cronExpr: "15 * * * *",
        timezone: "UTC",
        payload: {},
        enabled: true,
      });
    });

    await user.click(screen.getByLabelText("Delete schedule s1"));
    await user.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(mockDeleteSchedule).toHaveBeenCalledWith("s1");
    });
  });
});
