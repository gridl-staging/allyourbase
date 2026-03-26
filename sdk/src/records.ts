/**
 * @module SDK client for managing database records via CRUD and batch operations.
 */
import type {
  BatchOperation,
  BatchResult,
  GetParams,
  ListParams,
  ListResponse,
} from "./types";
import { encodePathSegment } from "./helpers";

interface RecordsClientRuntime {
  request<T>(path: string, init?: RequestInit & { skipAuth?: boolean }): Promise<T>;
}

/**
 * HTTP client for CRUD and batch operations on database records.
 */
export class RecordsClient {
  constructor(private client: RecordsClientRuntime) {}

  /** List records in a collection with optional filtering, sorting, and pagination. */
  /**
   * Retrieves paginated records from a collection with optional filtering, sorting, search, and field selection.
   * @param collection - Collection name to query
   * @param params - Optional filtering, pagination, and response options
   * @returns Promise resolving to paginated records and metadata
   */
  async list<T = Record<string, unknown>>(
    collection: string,
    params?: ListParams,
  ): Promise<ListResponse<T>> {
    const safeCollection = encodePathSegment(collection);
    const qs = new URLSearchParams();
    if (params?.page != null) qs.set("page", String(params.page));
    if (params?.perPage != null) qs.set("perPage", String(params.perPage));
    if (params?.sort) qs.set("sort", params.sort);
    if (params?.filter) qs.set("filter", params.filter);
    if (params?.search) qs.set("search", params.search);
    if (params?.fields) qs.set("fields", params.fields);
    if (params?.expand) qs.set("expand", params.expand);
    if (params?.skipTotal) qs.set("skipTotal", "true");
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.client.request(`/api/collections/${safeCollection}${suffix}`);
  }

  /** Get a single record by primary key. */
  async get<T = Record<string, unknown>>(
    collection: string,
    id: string,
    params?: GetParams,
  ): Promise<T> {
    const safeCollection = encodePathSegment(collection);
    const safeID = encodePathSegment(id);
    const qs = new URLSearchParams();
    if (params?.fields) qs.set("fields", params.fields);
    if (params?.expand) qs.set("expand", params.expand);
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.client.request(`/api/collections/${safeCollection}/${safeID}${suffix}`);
  }

  /** Create a new record. */
  async create<T = Record<string, unknown>>(
    collection: string,
    data: Record<string, unknown>,
  ): Promise<T> {
    return this.client.request(`/api/collections/${encodePathSegment(collection)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
  }

  /** Update an existing record (partial update). */
  async update<T = Record<string, unknown>>(
    collection: string,
    id: string,
    data: Record<string, unknown>,
  ): Promise<T> {
    return this.client.request(
      `/api/collections/${encodePathSegment(collection)}/${encodePathSegment(id)}`,
      {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      },
    );
  }

  /** Delete a record by primary key. */
  async delete(collection: string, id: string): Promise<void> {
    return this.client.request(
      `/api/collections/${encodePathSegment(collection)}/${encodePathSegment(id)}`,
      {
        method: "DELETE",
      },
    );
  }

  /** Execute multiple operations in a single atomic transaction. Max 1000 operations. */
  async batch<T = Record<string, unknown>>(
    collection: string,
    operations: BatchOperation[],
  ): Promise<BatchResult<T>[]> {
    return this.client.request(`/api/collections/${encodePathSegment(collection)}/batch`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ operations }),
    });
  }
}
