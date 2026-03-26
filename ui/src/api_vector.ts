import { request } from "./api_client";
import type {
  VectorIndexInfo,
  CreateVectorIndexRequest,
} from "./types/vector";

interface VectorIndexesListResponse {
  indexes: VectorIndexInfo[];
}

function isVectorIndexesListResponse(payload: unknown): payload is VectorIndexesListResponse {
  return typeof payload === "object" && payload !== null && Array.isArray((payload as { indexes?: unknown }).indexes);
}

export async function listVectorIndexes(): Promise<VectorIndexInfo[]> {
  const payload = await request<VectorIndexInfo[] | VectorIndexesListResponse>("/api/admin/vector/indexes");
  if (Array.isArray(payload)) {
    return payload;
  }
  if (isVectorIndexesListResponse(payload)) {
    return payload.indexes;
  }
  throw new Error("Expected vector indexes array or { indexes: [...] } response");
}

export function createVectorIndex(
  req: CreateVectorIndexRequest,
): Promise<VectorIndexInfo> {
  return request<VectorIndexInfo>("/api/admin/vector/indexes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}
