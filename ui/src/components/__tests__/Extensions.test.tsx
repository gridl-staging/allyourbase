import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { Extensions } from "../Extensions";

vi.mock("../../api_extensions", () => ({
  listExtensions: vi.fn(),
  enableExtension: vi.fn(),
  disableExtension: vi.fn(),
}));

import * as api from "../../api_extensions";

const mockExtensions = {
  extensions: [
    {
      name: "pg_stat_statements",
      installed: true,
      available: true,
      installed_version: "1.10",
      default_version: "1.10",
      comment: "Track execution statistics of SQL statements",
    },
    {
      name: "pgvector",
      installed: false,
      available: true,
      default_version: "0.5.0",
      comment: "Vector data type and operators",
    },
  ],
  total: 2,
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.listExtensions as ReturnType<typeof vi.fn>).mockResolvedValue(mockExtensions);
  (api.enableExtension as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  (api.disableExtension as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
});

describe("Extensions", () => {
  it("renders extension list with name/installed status/version", async () => {
    renderWithProviders(<Extensions />);
    await waitFor(() => {
      expect(screen.getByText("pg_stat_statements")).toBeInTheDocument();
    });
    expect(screen.getByText("pgvector")).toBeInTheDocument();
    expect(screen.getByText("1.10")).toBeInTheDocument();
  });

  it("enable calls API for uninstalled extensions", async () => {
    renderWithProviders(<Extensions />);
    await waitFor(() => {
      expect(screen.getByText("pgvector")).toBeInTheDocument();
    });

    const enableButtons = screen.getAllByRole("button", { name: /enable/i });
    fireEvent.click(enableButtons[0]);

    await waitFor(() => {
      expect(api.enableExtension).toHaveBeenCalledWith("pgvector");
    });
  });

  it("disables extension actions while enable is pending", async () => {
    let resolveEnable: (() => void) | undefined;
    (api.enableExtension as ReturnType<typeof vi.fn>).mockReturnValue(
      new Promise<void>((resolve) => {
        resolveEnable = resolve;
      }),
    );

    renderWithProviders(<Extensions />);
    await waitFor(() => {
      expect(screen.getByText("pgvector")).toBeInTheDocument();
    });

    fireEvent.click(screen.getAllByRole("button", { name: /enable/i })[0]);

    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /enable/i })[0]).toBeDisabled();
      expect(screen.getAllByRole("button", { name: /disable/i })[0]).toBeDisabled();
    });

    resolveEnable?.();
    await waitFor(() => {
      expect(api.enableExtension).toHaveBeenCalledWith("pgvector");
    });
  });

  it("disable fires ConfirmDialog for installed extensions", async () => {
    renderWithProviders(<Extensions />);
    await waitFor(() => {
      expect(screen.getByText("pg_stat_statements")).toBeInTheDocument();
    });

    const disableButtons = screen.getAllByRole("button", { name: /disable/i });
    fireEvent.click(disableButtons[0]);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /disable extension/i })).toBeInTheDocument();
    });

    // The confirm dialog's button label matches the table button; use getAllByRole to pick the dialog one
    const disableConfirmButtons = screen.getAllByRole("button", { name: /^disable$/i });
    fireEvent.click(disableConfirmButtons[disableConfirmButtons.length - 1]);
    await waitFor(() => {
      expect(api.disableExtension).toHaveBeenCalledWith("pg_stat_statements");
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listExtensions as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("Connection lost"));
    renderWithProviders(<Extensions />);
    await waitFor(() => {
      expect(screen.getByText("Connection lost")).toBeInTheDocument();
    });
  });
});
