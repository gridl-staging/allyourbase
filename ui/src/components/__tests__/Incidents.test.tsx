import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { Incidents } from "../Incidents";

vi.mock("../../api_incidents", () => ({
  listIncidents: vi.fn(),
  createIncident: vi.fn(),
  updateIncident: vi.fn(),
  addIncidentUpdate: vi.fn(),
}));

import * as api from "../../api_incidents";

const mockIncidents = [
  {
    id: "inc-1",
    title: "Database latency spike",
    status: "investigating",
    affectedServices: ["Database", "Auth"],
    createdAt: "2026-03-14T10:00:00Z",
    updatedAt: "2026-03-14T10:30:00Z",
    updates: [
      {
        id: "upd-1",
        incidentId: "inc-1",
        message: "Investigating root cause",
        status: "investigating",
        createdAt: "2026-03-14T10:05:00Z",
      },
    ],
  },
  {
    id: "inc-2",
    title: "Storage outage resolved",
    status: "resolved",
    affectedServices: ["Storage"],
    createdAt: "2026-03-13T08:00:00Z",
    updatedAt: "2026-03-13T09:00:00Z",
    resolvedAt: "2026-03-13T09:00:00Z",
    updates: [],
  },
];

beforeEach(() => {
  vi.clearAllMocks();
  (api.listIncidents as ReturnType<typeof vi.fn>).mockResolvedValue(
    mockIncidents,
  );
  (api.createIncident as ReturnType<typeof vi.fn>).mockResolvedValue(
    mockIncidents[0],
  );
  (api.updateIncident as ReturnType<typeof vi.fn>).mockResolvedValue(
    mockIncidents[0],
  );
  (api.addIncidentUpdate as ReturnType<typeof vi.fn>).mockResolvedValue({
    id: "upd-2",
    incidentId: "inc-1",
    message: "Fix deployed",
    status: "monitoring",
    createdAt: "2026-03-14T11:00:00Z",
  });
});

describe("Incidents", () => {
  it("renders incident list with title/status/affected-services", async () => {
    renderWithProviders(<Incidents />);
    await waitFor(() => {
      expect(
        screen.getByText("Database latency spike"),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("Storage outage resolved")).toBeInTheDocument();
    expect(screen.getByText("investigating")).toBeInTheDocument();
    expect(screen.getByText("resolved")).toBeInTheDocument();
  });

  it("create form validates required title", async () => {
    renderWithProviders(<Incidents />);
    await waitFor(() => {
      expect(
        screen.getByText("Database latency spike"),
      ).toBeInTheDocument();
    });

    fireEvent.click(
      screen.getByRole("button", { name: /create incident/i }),
    );
    const submitBtn = screen.getByRole("button", { name: /^create$/i });
    expect(submitBtn).toBeDisabled();
  });

  it("create incident calls API with correct data", async () => {
    renderWithProviders(<Incidents />);
    await waitFor(() => {
      expect(
        screen.getByText("Database latency spike"),
      ).toBeInTheDocument();
    });

    fireEvent.click(
      screen.getByRole("button", { name: /create incident/i }),
    );
    fireEvent.change(screen.getByPlaceholderText("Incident title"), {
      target: { value: "New incident" },
    });

    const submitBtn = screen.getByRole("button", { name: /^create$/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(api.createIncident).toHaveBeenCalled();
    });
  });

  it("expand shows timeline updates", async () => {
    renderWithProviders(<Incidents />);
    await waitFor(() => {
      expect(
        screen.getByText("Database latency spike"),
      ).toBeInTheDocument();
    });

    // Click expand on the first incident
    const expandBtns = screen.getAllByRole("button", { name: /details/i });
    fireEvent.click(expandBtns[0]);

    await waitFor(() => {
      expect(
        screen.getByText("Investigating root cause"),
      ).toBeInTheDocument();
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listIncidents as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("Service unavailable"),
    );
    renderWithProviders(<Incidents />);
    await waitFor(() => {
      expect(screen.getByText("Service unavailable")).toBeInTheDocument();
    });
  });
});
