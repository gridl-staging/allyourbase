import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { Backups } from "../Backups";
import { normalizeLocalDateTimeInput } from "../backups/format";

vi.mock("../../api_backups", () => {
  const listBackups = vi.fn();
  const listAllBackups = vi.fn(async (params?: { status?: string }) => {
    const backups: Array<Record<string, unknown>> = [];
    let offset = 0;
    let total = 0;

    while (true) {
      const page = await listBackups({
        ...params,
        limit: 200,
        offset,
      });
      backups.push(...page.backups);
      total = page.total;
      if (page.backups.length === 0 || backups.length >= total) {
        break;
      }
      offset += page.backups.length;
    }

    return { backups, total };
  });

  return {
    listBackups,
    listAllBackups,
    triggerBackup: vi.fn(),
    validatePITR: vi.fn(),
    restorePITR: vi.fn(),
    listRestoreJobs: vi.fn(),
    abandonRestoreJob: vi.fn(),
  };
});

import * as api from "../../api_backups";

const mockBackups = {
  backups: [
    {
      id: "b-1",
      db_name: "mydb",
      object_key: "backups/b-1.tar",
      size_bytes: 1048576,
      checksum: "abc123",
      started_at: "2026-03-10T10:00:00Z",
      completed_at: "2026-03-10T10:05:00Z",
      status: "completed",
      triggered_by: "schedule",
      backup_type: "full",
      project_id: "project-1",
      database_id: "db-1",
    },
    {
      id: "b-2",
      db_name: "mydb",
      object_key: "backups/b-2.tar",
      size_bytes: 524288,
      checksum: "def456",
      started_at: "2026-03-11T10:00:00Z",
      status: "running",
      triggered_by: "manual",
      backup_type: "incremental",
      project_id: "project-1",
      database_id: "db-1",
    },
  ],
  total: 2,
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.listBackups as ReturnType<typeof vi.fn>).mockResolvedValue(mockBackups);
  (api.validatePITR as ReturnType<typeof vi.fn>).mockResolvedValue({
    base_backup: mockBackups.backups[0],
    earliest_recoverable: "2026-03-10T10:00:00Z",
    latest_recoverable: "2026-03-12T10:00:00Z",
    estimated_wal_bytes: 10485760,
    wal_segments_count: 42,
  });
  (api.restorePITR as ReturnType<typeof vi.fn>).mockResolvedValue({
    job_id: "restore-1",
    status: "running",
    phase: "validating",
  });
  (api.listRestoreJobs as ReturnType<typeof vi.fn>).mockResolvedValue({
    jobs: [],
    count: 0,
  });
  (api.abandonRestoreJob as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
});

describe("Backups", () => {
  it("renders backup list with status badges", async () => {
    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(screen.getByText("completed")).toBeInTheDocument();
      expect(screen.getByText("running")).toBeInTheDocument();
    });
    expect(screen.getByText("full")).toBeInTheDocument();
    expect(screen.getByText("incremental")).toBeInTheDocument();
  });

  it("calls triggerBackup when Trigger Backup is clicked", async () => {
    (api.triggerBackup as ReturnType<typeof vi.fn>).mockResolvedValue({
      backup_id: "b-3",
      status: "started",
    });
    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(screen.getByText("completed")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /trigger backup/i }));
    await waitFor(() => {
      expect(api.triggerBackup).toHaveBeenCalledOnce();
    });
  });

  it("loads restore jobs using project and database context", async () => {
    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(api.listRestoreJobs).toHaveBeenCalledWith("project-1", "db-1");
    });
  });

  it("requires explicit restore context selection when multiple databases are present", async () => {
    (api.listBackups as ReturnType<typeof vi.fn>).mockResolvedValue({
      backups: [
        { ...mockBackups.backups[0] },
        {
          ...mockBackups.backups[1],
          id: "b-3",
          db_name: "otherdb",
          project_id: "project-2",
          database_id: "db-2",
        },
      ],
      total: 2,
    });

    renderWithProviders(<Backups />);

    const contextSelect = await screen.findByLabelText("Restore Context");
    expect(api.listRestoreJobs).not.toHaveBeenCalled();

    fireEvent.change(contextSelect, { target: { value: "project-2:db-2" } });

    await waitFor(() => {
      expect(api.listRestoreJobs).toHaveBeenCalledWith("project-2", "db-2");
    });
  });

  it("clears previous restore jobs while a different restore context is loading", async () => {
    let resolveNextContextJobs: ((value: { jobs: never[]; count: number }) => void) | null = null;
    const nextContextJobs = new Promise<{ jobs: never[]; count: number }>((resolve) => {
      resolveNextContextJobs = resolve;
    });

    (api.listBackups as ReturnType<typeof vi.fn>).mockResolvedValue({
      backups: [
        { ...mockBackups.backups[0] },
        {
          ...mockBackups.backups[1],
          id: "b-3",
          db_name: "otherdb",
          project_id: "project-2",
          database_id: "db-2",
        },
      ],
      total: 2,
    });
    (api.listRestoreJobs as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        jobs: [
          {
            id: "job-1",
            project_id: "project-1",
            database_id: "db-1",
            environment: "dev",
            target_time: "2026-03-11T09:30:00Z",
            base_backup_id: "b-1",
            wal_segments_needed: 10,
            logs: "",
            requested_by: "admin",
            status: "running",
            phase: "restoring",
            started_at: "2026-03-11T09:31:00Z",
            error_message: "",
          },
        ],
        count: 1,
      })
      .mockImplementationOnce(() => nextContextJobs);

    renderWithProviders(<Backups />);

    const contextSelect = await screen.findByLabelText("Restore Context");
    fireEvent.change(contextSelect, { target: { value: "project-1:db-1" } });

    await waitFor(() => {
      expect(screen.getByText("Restore Jobs")).toBeInTheDocument();
    });

    fireEvent.change(contextSelect, { target: { value: "project-2:db-2" } });

    await waitFor(() => {
      expect(screen.queryByText("Restore Jobs")).not.toBeInTheDocument();
    });
    expect(screen.queryByText("restoring")).not.toBeInTheDocument();

    resolveNextContextJobs?.({ jobs: [], count: 0 });
    await waitFor(() => {
      expect(api.listRestoreJobs).toHaveBeenCalledWith("project-2", "db-2");
    });
  });

  it("clears restore jobs when the selected restore context becomes unset", async () => {
    (api.listBackups as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(mockBackups)
      .mockResolvedValueOnce({
        backups: [
          {
            ...mockBackups.backups[1],
            id: "b-3",
            db_name: "otherdb",
            project_id: "project-2",
            database_id: "db-2",
          },
          {
            ...mockBackups.backups[0],
            id: "b-4",
            db_name: "third-db",
            project_id: "project-3",
            database_id: "db-3",
          },
        ],
        total: 2,
      });
    (api.listRestoreJobs as ReturnType<typeof vi.fn>).mockResolvedValue({
      jobs: [
        {
          id: "job-1",
          project_id: "project-1",
          database_id: "db-1",
          environment: "dev",
          target_time: "2026-03-11T09:30:00Z",
          base_backup_id: "b-1",
          wal_segments_needed: 10,
          logs: "",
          requested_by: "admin",
          status: "running",
          phase: "restoring",
          started_at: "2026-03-11T09:31:00Z",
          error_message: "",
        },
      ],
      count: 1,
    });

    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(screen.getByText("Restore Jobs")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Status"), {
      target: { value: "running" },
    });
    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));

    await waitFor(() => {
      expect(screen.queryByText("Restore Jobs")).not.toBeInTheDocument();
    });
    expect(screen.getByText(/select a project\/database/i)).toBeInTheDocument();
  });

  it("only reapplies backup filters after Apply is clicked", async () => {
    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(api.listBackups).toHaveBeenCalledTimes(1);
    });

    fireEvent.change(screen.getByLabelText("Status"), {
      target: { value: "running" },
    });
    expect(api.listBackups).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));
    await waitFor(() => {
      expect(api.listBackups).toHaveBeenCalledTimes(2);
    });
    expect(api.listBackups).toHaveBeenLastCalledWith({
      status: "running",
      limit: 200,
      offset: 0,
    });
  });

  it("applies the backup type filter after Apply is clicked", async () => {
    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(api.listBackups).toHaveBeenCalledTimes(1);
    });

    fireEvent.change(screen.getByLabelText("Type"), {
      target: { value: "incremental" },
    });
    expect(api.listBackups).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));
    await waitFor(() => {
      expect(screen.getByText("incremental")).toBeInTheDocument();
      expect(screen.queryByText("full")).not.toBeInTheDocument();
    });
  });

  it("keeps restore contexts available when the backup type filter hides table rows", async () => {
    (api.listBackups as ReturnType<typeof vi.fn>).mockResolvedValue({
      backups: [
        {
          ...mockBackups.backups[0],
          project_id: "project-1",
          database_id: "db-1",
          backup_type: "full",
        },
        {
          ...mockBackups.backups[1],
          id: "b-3",
          project_id: "project-2",
          database_id: "db-2",
          db_name: "otherdb",
          backup_type: "incremental",
        },
      ],
      total: 2,
    });

    renderWithProviders(<Backups />);
    const contextSelect = await screen.findByLabelText("Restore Context");

    fireEvent.change(screen.getByLabelText("Type"), {
      target: { value: "incremental" },
    });
    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));

    await waitFor(() => {
      expect(screen.getByText("incremental")).toBeInTheDocument();
      expect(screen.queryByText("full")).not.toBeInTheDocument();
    });
    expect(screen.getByRole("option", { name: "project-1 / db-1" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "project-2 / db-2" })).toBeInTheDocument();

    fireEvent.change(contextSelect, { target: { value: "project-1:db-1" } });

    await waitFor(() => {
      expect(api.listRestoreJobs).toHaveBeenCalledWith("project-1", "db-1");
    });
  });

  it("loads backup rows from later backend pages", async () => {
    (api.listBackups as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        backups: [{ ...mockBackups.backups[0] }],
        total: 2,
      })
      .mockResolvedValueOnce({
        backups: [
          {
            ...mockBackups.backups[1],
            id: "b-3",
            db_name: "otherdb",
            project_id: "project-2",
            database_id: "db-2",
          },
        ],
        total: 2,
      });

    renderWithProviders(<Backups />);

    await waitFor(() => {
      expect(screen.getByText("otherdb")).toBeInTheDocument();
    });
    expect(api.listBackups).toHaveBeenNthCalledWith(1, {
      limit: 200,
      offset: 0,
    });
    expect(api.listBackups).toHaveBeenNthCalledWith(2, {
      limit: 200,
      offset: 1,
    });
    expect(api.listRestoreJobs).not.toHaveBeenCalled();
  });

  it("validates PITR target time and displays recovery window", async () => {
    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(screen.getByText("completed")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Target Time"), {
      target: { value: "2026-03-11T09:30" },
    });
    fireEvent.click(screen.getByRole("button", { name: /validate pitr/i }));

    await waitFor(() => {
      expect(api.validatePITR).toHaveBeenCalledWith(
        "project-1",
        "db-1",
        normalizeLocalDateTimeInput("2026-03-11T09:30"),
      );
    });
    expect(screen.getByText(/Earliest:/)).toBeInTheDocument();
    expect(screen.getByText(/Latest:/)).toBeInTheDocument();
  });

  it("starts PITR restore after validation and respects dry run toggle", async () => {
    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(screen.getByText("completed")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Target Time"), {
      target: { value: "2026-03-11T09:30" },
    });
    fireEvent.click(screen.getByRole("button", { name: /validate pitr/i }));
    await waitFor(() => {
      expect(api.validatePITR).toHaveBeenCalled();
    });

    fireEvent.click(screen.getByLabelText("Dry run"));
    fireEvent.click(screen.getByRole("button", { name: /start restore/i }));

    await waitFor(() => {
      expect(api.restorePITR).toHaveBeenCalledWith(
        "project-1",
        "db-1",
        normalizeLocalDateTimeInput("2026-03-11T09:30"),
        true,
      );
    });
  });

  it("fires abandon restore after confirm dialog approval", async () => {
    (api.listRestoreJobs as ReturnType<typeof vi.fn>).mockResolvedValue({
      jobs: [
        {
          id: "job-1",
          project_id: "project-1",
          database_id: "db-1",
          environment: "dev",
          target_time: "2026-03-11T09:30:00Z",
          base_backup_id: "b-1",
          wal_segments_needed: 10,
          logs: "",
          requested_by: "admin",
          status: "running",
          phase: "restoring",
          started_at: "2026-03-11T09:31:00Z",
          error_message: "",
        },
      ],
      count: 1,
    });

    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(screen.getByText("restoring")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /abandon/i }));
    fireEvent.click(screen.getByRole("button", { name: /^Abandon$/ }));

    await waitFor(() => {
      expect(api.abandonRestoreJob).toHaveBeenCalledWith("job-1");
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listBackups as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("Network error"),
    );
    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(screen.getByText("Network error")).toBeInTheDocument();
    });
  });

  it("renders empty state when no backups", async () => {
    (api.listBackups as ReturnType<typeof vi.fn>).mockResolvedValue({
      backups: [],
      total: 0,
    });
    renderWithProviders(<Backups />);
    await waitFor(() => {
      expect(screen.getByText(/no backups/i)).toBeInTheDocument();
    });
  });
});
