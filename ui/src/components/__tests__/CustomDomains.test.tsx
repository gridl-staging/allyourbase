import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { CustomDomains } from "../CustomDomains";

vi.mock("../../api_domains", () => ({
  listDomains: vi.fn(),
  createDomain: vi.fn(),
  deleteDomain: vi.fn(),
  verifyDomain: vi.fn(),
}));

import * as api from "../../api_domains";

const mockDomains = {
  items: [
    {
      id: "d-1",
      hostname: "api.example.com",
      environment: "production",
      status: "active",
      verificationToken: "tok-1",
      verificationRecord: "_ayb-challenge.api.example.com TXT tok-1",
      healthStatus: "healthy",
      createdAt: "2026-03-10T10:00:00Z",
      updatedAt: "2026-03-10T10:00:00Z",
    },
    {
      id: "d-2",
      hostname: "staging.example.com",
      environment: "staging",
      status: "pending_verification",
      verificationToken: "tok-2",
      verificationRecord: "_ayb-challenge.staging.example.com TXT tok-2",
      healthStatus: "unknown",
      createdAt: "2026-03-11T10:00:00Z",
      updatedAt: "2026-03-11T10:00:00Z",
    },
  ],
  page: 1,
  perPage: 20,
  totalItems: 2,
  totalPages: 1,
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.listDomains as ReturnType<typeof vi.fn>).mockResolvedValue(mockDomains);
  (api.createDomain as ReturnType<typeof vi.fn>).mockResolvedValue(mockDomains.items[0]);
  (api.deleteDomain as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  (api.verifyDomain as ReturnType<typeof vi.fn>).mockResolvedValue({
    ...mockDomains.items[1],
    status: "verified",
  });
});

describe("CustomDomains", () => {
  it("renders domain list with hostname/status/environment", async () => {
    renderWithProviders(<CustomDomains />);
    await waitFor(() => {
      expect(screen.getByText("api.example.com")).toBeInTheDocument();
    });
    expect(screen.getByText("staging.example.com")).toBeInTheDocument();
    expect(screen.getByText("active")).toBeInTheDocument();
    expect(screen.getByText("pending_verification")).toBeInTheDocument();
    expect(
      screen.getByText("_ayb-challenge.staging.example.com TXT tok-2"),
    ).toBeInTheDocument();
  });

  it("validates hostname in create form", async () => {
    renderWithProviders(<CustomDomains />);
    await waitFor(() => {
      expect(screen.getByText("api.example.com")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /add domain/i }));
    const submitBtn = screen.getByRole("button", { name: /^add$/i });
    expect(submitBtn).toBeDisabled();
  });

  it("fires ConfirmDialog on delete", async () => {
    renderWithProviders(<CustomDomains />);
    await waitFor(() => {
      expect(screen.getByText("api.example.com")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByLabelText(/^Delete /);
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /delete domain/i })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    await waitFor(() => {
      expect(api.deleteDomain).toHaveBeenCalledWith("d-1");
    });
  });

  it("verify button calls API for pending domains", async () => {
    renderWithProviders(<CustomDomains />);
    await waitFor(() => {
      expect(screen.getByText("staging.example.com")).toBeInTheDocument();
    });
    expect(
      screen.getByText("_ayb-challenge.staging.example.com TXT tok-2"),
    ).toBeInTheDocument();

    const verifyButtons = screen.getAllByRole("button", { name: /verify/i });
    fireEvent.click(verifyButtons[0]);

    await waitFor(() => {
      expect(api.verifyDomain).toHaveBeenCalledWith("d-2");
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listDomains as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("Timeout"));
    renderWithProviders(<CustomDomains />);
    await waitFor(() => {
      expect(screen.getByText("Timeout")).toBeInTheDocument();
    });
  });
});
