/**
 * @module Storage client implementation for cloud storage bucket operations with file upload, download, deletion, listing, and signed URL generation.
 */
import type { StorageObject } from "./types";
import {
  encodePathSegment,
  encodePathWithSlashes,
  normalizeStorageListResponse,
  normalizeStorageObject,
} from "./helpers";

interface StorageClientRuntime {
  request<T>(path: string, init?: RequestInit & { skipAuth?: boolean }): Promise<T>;
  getBaseURL(): string;
}

/**
 * Client for managing files in cloud storage buckets. Supports uploading, downloading, deleting, listing files, and generating signed URLs for time-limited file access.
 */
export class StorageClient {
  constructor(private client: StorageClientRuntime) {}

  private bucketPath(bucket: string): string {
    return `/api/storage/${encodePathSegment(bucket)}`;
  }

  private objectPath(bucket: string, name: string): string {
    return `${this.bucketPath(bucket)}/${encodePathWithSlashes(name)}`;
  }

  /** Upload a file to a bucket. */
  async upload(
    bucket: string,
    file: Blob | File,
    name?: string,
  ): Promise<StorageObject> {
    const form = new FormData();
    const resolvedName = name ?? (file instanceof File ? file.name : "upload");
    // Set the name as a separate form field so the server uses it directly.
    // FormData filename sanitization strips path separators (e.g. "dir/file.txt"
    // becomes "file.txt"), so names with "/" must be sent as a field, not just
    // as the Content-Disposition filename.
    form.append("name", resolvedName);
    form.append("file", file, resolvedName);
    const response = await this.client.request<StorageObject>(this.bucketPath(bucket), {
      method: "POST",
      body: form,
    });
    return normalizeStorageObject(response);
  }

  /** Get a download URL for a file. */
  downloadURL(bucket: string, name: string): string {
    return `${this.client.getBaseURL()}${this.objectPath(bucket, name)}`;
  }

  /** Delete a file from a bucket. */
  async delete(bucket: string, name: string): Promise<void> {
    return this.client.request(this.objectPath(bucket, name), {
      method: "DELETE",
    });
  }

  /** List files in a bucket. */
  async list(
    bucket: string,
    params?: { prefix?: string; limit?: number; offset?: number },
  ): Promise<{ items: StorageObject[]; totalItems: number }> {
    const qs = new URLSearchParams();
    if (params?.prefix) qs.set("prefix", params.prefix);
    if (params?.limit != null) qs.set("limit", String(params.limit));
    if (params?.offset != null) qs.set("offset", String(params.offset));
    const suffix = qs.toString() ? `?${qs}` : "";
    const response = await this.client.request<{ items: StorageObject[]; totalItems: number }>(
      `${this.bucketPath(bucket)}${suffix}`,
    );
    return normalizeStorageListResponse(response);
  }

  /** Get a signed URL for time-limited access to a file. */
  async getSignedURL(
    bucket: string,
    name: string,
    expiresIn?: number,
  ): Promise<{ url: string }> {
    return this.client.request(`${this.objectPath(bucket, name)}/sign`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ expiresIn: expiresIn ?? 3600 }),
    });
  }
}
