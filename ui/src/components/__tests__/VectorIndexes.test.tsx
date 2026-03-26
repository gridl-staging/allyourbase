import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { VectorIndexes } from "../VectorIndexes";

vi.mock("../../api_vector", () => ({
  listVectorIndexes: vi.fn(),
  createVectorIndex: vi.fn(),
}));

import * as api from "../../api_vector";

const mockIndexes = [
  {
    name: "idx_embeddings_hnsw",
    schema: "public",
    table: "documents",
    method: "hnsw",
    definition: "CREATE INDEX idx_embeddings_hnsw ON documents USING hnsw (embedding vector_cosine_ops)",
  },
];

beforeEach(() => {
  vi.clearAllMocks();
  (api.listVectorIndexes as ReturnType<typeof vi.fn>).mockResolvedValue(mockIndexes);
  (api.createVectorIndex as ReturnType<typeof vi.fn>).mockResolvedValue(mockIndexes[0]);
});

describe("VectorIndexes", () => {
  it("renders vector index list with name/table/method", async () => {
    renderWithProviders(<VectorIndexes />);
    await waitFor(() => {
      expect(screen.getByText("idx_embeddings_hnsw")).toBeInTheDocument();
    });
    expect(screen.getByText("documents")).toBeInTheDocument();
    expect(screen.getByText("hnsw")).toBeInTheDocument();
  });

  it("validates required fields in create form", async () => {
    renderWithProviders(<VectorIndexes />);
    await waitFor(() => {
      expect(screen.getByText("idx_embeddings_hnsw")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /create index/i }));
    const submitBtn = screen.getByRole("button", { name: /^create$/i });
    expect(submitBtn).toBeDisabled();
  });

  it("creates index with required fields", async () => {
    renderWithProviders(<VectorIndexes />);
    await waitFor(() => {
      expect(screen.getByText("idx_embeddings_hnsw")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /create index/i }));
    fireEvent.change(screen.getByLabelText("Table"), { target: { value: "items" } });
    fireEvent.change(screen.getByLabelText("Column"), { target: { value: "embedding" } });
    fireEvent.change(screen.getByLabelText("Method"), { target: { value: "hnsw" } });
    fireEvent.change(screen.getByLabelText("Metric"), { target: { value: "cosine" } });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(api.createVectorIndex).toHaveBeenCalledWith(
        expect.objectContaining({
          table: "items",
          column: "embedding",
          method: "hnsw",
          metric: "cosine",
        }),
      );
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listVectorIndexes as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("Not found"));
    renderWithProviders(<VectorIndexes />);
    await waitFor(() => {
      expect(screen.getByText("Not found")).toBeInTheDocument();
    });
  });
});
