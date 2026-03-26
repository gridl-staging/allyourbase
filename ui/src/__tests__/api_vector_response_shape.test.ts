import { beforeEach, describe, expect, it, vi } from "vitest";
import { createVectorIndex, listVectorIndexes } from "../api_vector";

describe("api_vector listVectorIndexes", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it("unwraps wrapped backend payloads", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          indexes: [
            {
              name: "idx_embeddings_hnsw",
              schema: "public",
              table: "documents",
              method: "hnsw",
              definition: "CREATE INDEX ...",
            },
          ],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(listVectorIndexes()).resolves.toEqual([
      {
        name: "idx_embeddings_hnsw",
        schema: "public",
        table: "documents",
        method: "hnsw",
        definition: "CREATE INDEX ...",
      },
    ]);
    expect(fetchMock).toHaveBeenCalledWith("/api/admin/vector/indexes", {
      headers: { Authorization: "Bearer admin-token" },
    });
  });

  it("keeps supporting legacy array payloads", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify([
          {
            name: "idx_embeddings_ivfflat",
            schema: "public",
            table: "documents",
            method: "ivfflat",
            definition: "CREATE INDEX ...",
          },
        ]),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(listVectorIndexes()).resolves.toEqual([
      {
        name: "idx_embeddings_ivfflat",
        schema: "public",
        table: "documents",
        method: "ivfflat",
        definition: "CREATE INDEX ...",
      },
    ]);
  });

  it("throws a clear error for malformed wrapped payloads", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ items: [] }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(listVectorIndexes()).rejects.toThrow(
      "Expected vector indexes array or { indexes: [...] } response",
    );
  });

  it("throws when wrapped payload has non-array indexes", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ indexes: { name: "not-an-array" } }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(listVectorIndexes()).rejects.toThrow(
      "Expected vector indexes array or { indexes: [...] } response",
    );
  });

  it("sends create requests with JSON body and auth header", async () => {
    const req = {
      name: "idx_embeddings_hnsw",
      table: "documents",
      column: "embedding",
      method: "hnsw" as const,
      dimensions: 1536,
      metric: "cosine" as const,
    };

    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          name: "idx_embeddings_hnsw",
          schema: "public",
          table: "documents",
          method: "hnsw",
          definition: "CREATE INDEX ...",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(createVectorIndex(req)).resolves.toEqual({
      name: "idx_embeddings_hnsw",
      schema: "public",
      table: "documents",
      method: "hnsw",
      definition: "CREATE INDEX ...",
    });

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/vector/indexes", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: JSON.stringify(req),
    });
  });

  it("surfaces backend validation errors from create requests", async () => {
    const req = {
      name: "idx_embeddings_bad",
      table: "documents",
      column: "embedding",
      method: "ivfflat" as const,
      dimensions: 1536,
      metric: "cosine" as const,
    };

    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ code: 400, message: "invalid vector index method" }),
        { status: 400, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(createVectorIndex(req)).rejects.toThrow("invalid vector index method");
    expect(fetchMock).toHaveBeenCalledWith("/api/admin/vector/indexes", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: JSON.stringify(req),
    });
  });
});
