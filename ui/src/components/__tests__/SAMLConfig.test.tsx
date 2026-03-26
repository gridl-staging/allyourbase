import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { SAMLConfig } from "../SAMLConfig";

vi.mock("../../api_saml", () => ({
  listSAMLProviders: vi.fn(),
  createSAMLProvider: vi.fn(),
  updateSAMLProvider: vi.fn(),
  deleteSAMLProvider: vi.fn(),
}));

import * as api from "../../api_saml";

const mockProviders = [
  {
    name: "okta-prod",
    entity_id: "https://okta.example.com/saml",
    idp_metadata_xml: "<xml/>",
    attribute_mapping: { email: "user.email" },
    created_at: "2026-03-10T10:00:00Z",
    updated_at: "2026-03-12T10:00:00Z",
  },
  {
    name: "azure-ad",
    entity_id: "https://login.microsoftonline.com/tenant/saml",
    created_at: "2026-03-11T10:00:00Z",
    updated_at: "2026-03-11T10:00:00Z",
  },
];

beforeEach(() => {
  vi.clearAllMocks();
  (api.listSAMLProviders as ReturnType<typeof vi.fn>).mockResolvedValue(
    mockProviders,
  );
  (api.createSAMLProvider as ReturnType<typeof vi.fn>).mockResolvedValue(
    mockProviders[0],
  );
  (api.deleteSAMLProvider as ReturnType<typeof vi.fn>).mockResolvedValue(
    undefined,
  );
});

describe("SAMLConfig", () => {
  it("renders provider list with name/entity_id/updated_at", async () => {
    renderWithProviders(<SAMLConfig />);
    await waitFor(() => {
      expect(screen.getByText("okta-prod")).toBeInTheDocument();
    });
    expect(screen.getByText("azure-ad")).toBeInTheDocument();
    expect(
      screen.getByText("https://okta.example.com/saml"),
    ).toBeInTheDocument();
  });

  it("validates required name and entity_id in create form", async () => {
    renderWithProviders(<SAMLConfig />);
    await waitFor(() => {
      expect(screen.getByText("okta-prod")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /add provider/i }));
    const submitBtn = screen.getByRole("button", { name: /^create$/i });
    expect(submitBtn).toBeDisabled();
  });

  it("creates provider with metadata URL", async () => {
    renderWithProviders(<SAMLConfig />);
    await waitFor(() => {
      expect(screen.getByText("okta-prod")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /add provider/i }));
    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "new-idp" },
    });
    fireEvent.change(screen.getByLabelText("Entity ID"), {
      target: { value: "https://new.example.com" },
    });
    fireEvent.change(screen.getByLabelText("Metadata URL"), {
      target: { value: "https://new.example.com/metadata" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(api.createSAMLProvider).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "new-idp",
          entity_id: "https://new.example.com",
          idp_metadata_url: "https://new.example.com/metadata",
        }),
      );
    });
  });

  it("supports metadata via raw XML textarea", async () => {
    renderWithProviders(<SAMLConfig />);
    await waitFor(() => {
      expect(screen.getByText("okta-prod")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /add provider/i }));
    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "xml-idp" },
    });
    fireEvent.change(screen.getByLabelText("Entity ID"), {
      target: { value: "https://xml.example.com" },
    });
    fireEvent.change(screen.getByLabelText("Metadata XML"), {
      target: { value: "<EntityDescriptor/>" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(api.createSAMLProvider).toHaveBeenCalledWith(
        expect.objectContaining({
          idp_metadata_xml: "<EntityDescriptor/>",
        }),
      );
    });
  });

  it("fires ConfirmDialog on delete", async () => {
    renderWithProviders(<SAMLConfig />);
    await waitFor(() => {
      expect(screen.getByText("okta-prod")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByLabelText(/^Delete /);
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: /delete provider/i }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    await waitFor(() => {
      expect(api.deleteSAMLProvider).toHaveBeenCalledWith("okta-prod");
    });
  });

  it("pre-fills form on edit", async () => {
    renderWithProviders(<SAMLConfig />);
    await waitFor(() => {
      expect(screen.getByText("okta-prod")).toBeInTheDocument();
    });

    const editButtons = screen.getAllByLabelText(/^Edit /);
    fireEvent.click(editButtons[0]);

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue("okta-prod");
    });
    expect(screen.getByLabelText("Entity ID")).toHaveValue(
      "https://okta.example.com/saml",
    );
  });

  it("shows error state on fetch failure", async () => {
    (api.listSAMLProviders as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("Network error"),
    );
    renderWithProviders(<SAMLConfig />);
    await waitFor(() => {
      expect(screen.getByText("Network error")).toBeInTheDocument();
    });
  });
});
