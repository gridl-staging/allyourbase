import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { Secrets } from "../Secrets";

vi.mock("../../api_secrets", () => ({
  listSecrets: vi.fn(),
  getSecret: vi.fn(),
  createSecret: vi.fn(),
  updateSecret: vi.fn(),
  deleteSecret: vi.fn(),
  rotateSecrets: vi.fn(),
}));

import * as api from "../../api_secrets";

const mockSecrets = [
  { name: "DATABASE_URL", created_at: "2026-03-10T10:00:00Z", updated_at: "2026-03-10T10:00:00Z" },
  { name: "API_KEY", created_at: "2026-03-11T10:00:00Z", updated_at: "2026-03-12T10:00:00Z" },
];

beforeEach(() => {
  vi.clearAllMocks();
  (api.listSecrets as ReturnType<typeof vi.fn>).mockResolvedValue(mockSecrets);
  (api.getSecret as ReturnType<typeof vi.fn>).mockResolvedValue({
    name: "DATABASE_URL",
    value: "postgres://localhost:5432/mydb",
  });
  (api.createSecret as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  (api.updateSecret as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  (api.deleteSecret as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  (api.rotateSecrets as ReturnType<typeof vi.fn>).mockResolvedValue({
    status: "rotated",
  });
});

describe("Secrets", () => {
  it("renders secret list with names and dates", async () => {
    renderWithProviders(<Secrets />);
    await waitFor(() => {
      expect(screen.getByText("DATABASE_URL")).toBeInTheDocument();
    });
    expect(screen.getByText("API_KEY")).toBeInTheDocument();
  });

  it("shows empty-state guidance and create entry point when there are no secrets", async () => {
    (api.listSecrets as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);
    renderWithProviders(<Secrets />);

    await waitFor(() => {
      expect(screen.getByText("No secrets configured yet")).toBeInTheDocument();
      expect(
        screen.getByText(
          "Store API tokens, signing keys, and other sensitive values for your backend.",
        ),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Create your first secret" }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Create your first secret" }));
    expect(screen.getByText("New Secret")).toBeInTheDocument();
  });

  it("validates name and value are required for create", async () => {
    renderWithProviders(<Secrets />);
    await waitFor(() => {
      expect(screen.getByText("DATABASE_URL")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /create secret/i }));
    const submitBtn = screen.getByRole("button", { name: /^create$/i });
    expect(submitBtn).toBeDisabled();
  });

  it("creates a secret when form is filled and submitted", async () => {
    renderWithProviders(<Secrets />);
    await waitFor(() => {
      expect(screen.getByText("DATABASE_URL")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /create secret/i }));
    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "NEW_SECRET" },
    });
    fireEvent.change(screen.getByLabelText("Value"), {
      target: { value: "secret-value" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(api.createSecret).toHaveBeenCalledWith({
        name: "NEW_SECRET",
        value: "secret-value",
      });
    });
  });

  it("reveals secret value on reveal click", async () => {
    renderWithProviders(<Secrets />);
    await waitFor(() => {
      expect(screen.getByText("DATABASE_URL")).toBeInTheDocument();
    });

    const revealButtons = screen.getAllByLabelText(/^Reveal /);
    fireEvent.click(revealButtons[0]);

    await waitFor(() => {
      expect(api.getSecret).toHaveBeenCalledWith("DATABASE_URL");
    });
    await waitFor(() => {
      expect(
        screen.getByText("postgres://localhost:5432/mydb"),
      ).toBeInTheDocument();
    });
  });

  it("fires ConfirmDialog on delete", async () => {
    renderWithProviders(<Secrets />);
    await waitFor(() => {
      expect(screen.getByText("DATABASE_URL")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByLabelText(/^Delete /);
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: /delete secret/i }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    await waitFor(() => {
      expect(api.deleteSecret).toHaveBeenCalledWith("DATABASE_URL");
    });
  });

  it("updates a secret value when form is submitted in edit mode", async () => {
    renderWithProviders(<Secrets />);
    await waitFor(() => {
      expect(screen.getByText("DATABASE_URL")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("Update DATABASE_URL"));
    expect(screen.getByLabelText("Name")).toHaveValue("DATABASE_URL");
    expect(screen.getByLabelText("Name")).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Value"), {
      target: { value: "rotated-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^update$/i }));

    await waitFor(() => {
      expect(api.updateSecret).toHaveBeenCalledWith("DATABASE_URL", {
        value: "rotated-secret",
      });
    });
  });

  it("fires destructive ConfirmDialog for rotate JWT secret", async () => {
    renderWithProviders(<Secrets />);
    await waitFor(() => {
      expect(screen.getByText("DATABASE_URL")).toBeInTheDocument();
    });

    fireEvent.click(
      screen.getByRole("button", { name: /rotate jwt secret/i }),
    );

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: /rotate jwt secret/i }),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByText(/this will invalidate all existing jwt tokens/i),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /^rotate$/i }));
    await waitFor(() => {
      expect(api.rotateSecrets).toHaveBeenCalled();
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listSecrets as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("Forbidden"),
    );
    renderWithProviders(<Secrets />);
    await waitFor(() => {
      expect(screen.getByText("Forbidden")).toBeInTheDocument();
    });
  });
});
