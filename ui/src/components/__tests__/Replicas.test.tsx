import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { Replicas } from "../Replicas";

vi.mock("../../api_replicas", () => ({
  listReplicas: vi.fn(),
  checkReplicas: vi.fn(),
  addReplica: vi.fn(),
  removeReplica: vi.fn(),
  promoteReplica: vi.fn(),
  failover: vi.fn(),
}));

import * as api from "../../api_replicas";

const mockReplicas = {
  replicas: [
    {
      name: "replica-1",
      url: "postgres://replica1:5432/mydb",
      state: "healthy",
      lag_bytes: 1024,
      weight: 100,
      connections: { total: 10, idle: 5, in_use: 5 },
      last_checked_at: "2026-03-12T15:00:00Z",
      last_error: null,
    },
    {
      name: "replica-2",
      url: "postgres://replica2:5432/mydb",
      state: "lagging",
      lag_bytes: 5242880,
      weight: 50,
      connections: { total: 8, idle: 3, in_use: 5 },
      last_checked_at: "2026-03-12T15:00:00Z",
      last_error: "WAL replay delayed",
    },
  ],
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.listReplicas as ReturnType<typeof vi.fn>).mockResolvedValue(mockReplicas);
});

describe("Replicas", () => {
  it("renders replica list with url, lag, connections, and state", async () => {
    renderWithProviders(<Replicas />);
    await waitFor(() => {
      expect(screen.getByText("postgres://replica1:5432/mydb")).toBeInTheDocument();
    });
    expect(screen.getByText("postgres://replica2:5432/mydb")).toBeInTheDocument();
    expect(screen.getByText("healthy")).toBeInTheDocument();
    expect(screen.getByText("lagging")).toBeInTheDocument();
  });

  it("validates required fields in add-replica form", async () => {
    renderWithProviders(<Replicas />);
    await waitFor(() => {
      expect(screen.getByText("postgres://replica1:5432/mydb")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /add replica/i }));
    const submitBtn = screen.getByRole("button", { name: /^add$/i });
    expect(submitBtn).toBeDisabled();
  });

  it("uses verify-full as the default ssl mode when adding a replica", async () => {
    (api.addReplica as ReturnType<typeof vi.fn>).mockResolvedValue({
      status: "added",
      record: {
        name: "replica-new",
        host: "replica-new.local",
        port: 5432,
        database: "appdb",
        ssl_mode: "verify-full",
        weight: 100,
        max_lag_bytes: 0,
        role: "replica",
        state: "active",
      },
      replicas: mockReplicas.replicas,
    });

    renderWithProviders(<Replicas />);
    await waitFor(() => {
      expect(screen.getByText("postgres://replica1:5432/mydb")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /add replica/i }));
    expect(screen.getByLabelText("SSL Mode")).toHaveValue("verify-full");

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "replica-new" },
    });
    fireEvent.change(screen.getByLabelText("Host"), {
      target: { value: "replica-new.local" },
    });
    fireEvent.change(screen.getByLabelText("Database"), {
      target: { value: "appdb" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^add$/i }));

    await waitFor(() => {
      expect(api.addReplica).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "replica-new",
          host: "replica-new.local",
          database: "appdb",
          ssl_mode: "verify-full",
        }),
      );
    });
  });

  it("shows confirm dialog for remove action", async () => {
    renderWithProviders(<Replicas />);
    await waitFor(() => {
      expect(screen.getByText("postgres://replica1:5432/mydb")).toBeInTheDocument();
    });
    const removeButtons = screen.getAllByLabelText(/^Remove /);
    fireEvent.click(removeButtons[0]);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /remove replica/i })).toBeInTheDocument();
    });
  });

  it("promotes replicas by canonical status name instead of URL metadata", async () => {
    (api.listReplicas as ReturnType<typeof vi.fn>).mockResolvedValue({
      replicas: [
        {
          ...mockReplicas.replicas[0],
          name: "replica-canonical",
          url: "postgres://replica1:5432/mydb?application_name=replica-url-hint",
        },
      ],
    });
    (api.promoteReplica as ReturnType<typeof vi.fn>).mockResolvedValue({
      status: "promoted",
      primary: {
        name: "replica-canonical",
        host: "replica1",
        port: 5432,
        database: "mydb",
        ssl_mode: "prefer",
        weight: 100,
        max_lag_bytes: 0,
        role: "primary",
        state: "healthy",
      },
      replicas: mockReplicas.replicas,
    });

    renderWithProviders(<Replicas />);
    await waitFor(() => {
      expect(screen.getByText(/application_name=replica-url-hint/)).toBeInTheDocument();
    });

    expect(screen.getByLabelText("Promote replica-canonical")).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("Promote replica-canonical"));
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /promote replica/i })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^Promote$/ }));
    await waitFor(() => {
      expect(api.promoteReplica).toHaveBeenCalledWith("replica-canonical");
    });
  });

  it("requires typing failover before allowing failover confirm", async () => {
    (api.failover as ReturnType<typeof vi.fn>).mockResolvedValue({ status: "failover_complete" });

    renderWithProviders(<Replicas />);
    await waitFor(() => {
      expect(screen.getByText("postgres://replica1:5432/mydb")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^Failover$/ }));
    const failoverConfirm = await screen.findByRole("button", { name: /execute failover/i });
    expect(failoverConfirm).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Type failover to confirm"), {
      target: { value: "failover" },
    });
    fireEvent.click(failoverConfirm);

    await waitFor(() => {
      expect(api.failover).toHaveBeenCalledWith({ target: "", force: false });
    });
  });

  it("removes replicas by canonical status name instead of URL metadata", async () => {
    (api.listReplicas as ReturnType<typeof vi.fn>).mockResolvedValue({
      replicas: [
        {
          ...mockReplicas.replicas[0],
          name: "replica-canonical",
          url: "postgres://replica1:5432/mydb?application_name=replica-url-hint",
        },
      ],
    });
    (api.removeReplica as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);

    renderWithProviders(<Replicas />);
    await waitFor(() => {
      expect(screen.getByText(/application_name=replica-url-hint/)).toBeInTheDocument();
    });

    expect(screen.getByLabelText("Remove replica-canonical")).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("Remove replica-canonical"));
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /remove replica/i })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^Remove$/ }));
    await waitFor(() => {
      expect(api.removeReplica).toHaveBeenCalledWith("replica-canonical");
    });
  });

  it("shows canonical replica names in destructive action copy", async () => {
    (api.listReplicas as ReturnType<typeof vi.fn>).mockResolvedValue({
      replicas: [{ ...mockReplicas.replicas[0] }],
    });

    renderWithProviders(<Replicas />);
    await waitFor(() => {
      expect(screen.getByText("postgres://replica1:5432/mydb")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText(/^Remove /));
    await waitFor(() => {
      expect(screen.getByText(/remove replica replica-1\?/i)).toBeInTheDocument();
    });
  });

  it("allows manual replica-name entry when a row has no lifecycle name", async () => {
    (api.listReplicas as ReturnType<typeof vi.fn>).mockResolvedValue({
      replicas: [
        {
          ...mockReplicas.replicas[0],
          name: "",
          last_checked_at: "",
        },
      ],
    });
    (api.removeReplica as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);

    renderWithProviders(<Replicas />);
    await waitFor(() => {
      expect(screen.getByText("postgres://replica1:5432/mydb")).toBeInTheDocument();
    });

    expect(screen.getByText("-")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText(/^Remove /));
    const confirmButton = await screen.findByRole("button", { name: /^Remove$/ });
    expect(confirmButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Replica name"), {
      target: { value: "replica-manual" },
    });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(api.removeReplica).toHaveBeenCalledWith("replica-manual");
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listReplicas as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("Connection refused"),
    );
    renderWithProviders(<Replicas />);
    await waitFor(() => {
      expect(screen.getByText("Connection refused")).toBeInTheDocument();
    });
  });
});
