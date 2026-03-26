import type {
  StorageListResponse,
  StorageObject,
  StorageCDNPurgeRequest,
  StorageCDNPurgeResponse,
} from "./types";
import {
  request,
  requestNoBody,
} from "./api_client";

function encodeStoragePathSegment(segment: string): string {
  if (segment === ".") {
    return "%2E";
  }
  if (segment === "..") {
    return "%2E%2E";
  }
  return encodeURIComponent(segment);
}

function encodeStorageObjectName(name: string): string {
  return name.split("/").map((segment) => encodeStoragePathSegment(segment)).join("/");
}

function storagePath(bucket: string, name?: string): string {
  const bucketPath = encodeStoragePathSegment(bucket);
  if (name === undefined) {
    return `/api/storage/${bucketPath}`;
  }
  return `/api/storage/${bucketPath}/${encodeStorageObjectName(name)}`;
}

export async function listStorageFiles(
  bucket: string,
  params: { prefix?: string; limit?: number; offset?: number } = {},
): Promise<StorageListResponse> {
  const qs = new URLSearchParams();
  if (params.prefix) qs.set("prefix", params.prefix);
  if (params.limit) qs.set("limit", String(params.limit));
  if (params.offset) qs.set("offset", String(params.offset));
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`${storagePath(bucket)}${suffix}`);
}

export async function uploadStorageFile(
  bucket: string,
  file: File,
): Promise<StorageObject> {
  const formData = new FormData();
  formData.append("file", file);
  return request<StorageObject>(storagePath(bucket), {
    method: "POST",
    body: formData,
  });
}

export async function deleteStorageFile(
  bucket: string,
  name: string,
): Promise<void> {
  return requestNoBody(storagePath(bucket, name), {
    method: "DELETE",
  });
}

export async function getSignedURL(
  bucket: string,
  name: string,
  expiresIn?: number,
): Promise<{ url: string }> {
  return request(`${storagePath(bucket, name)}/sign`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(expiresIn ? { expiresIn } : {}),
  });
}

export function storageDownloadURL(bucket: string, name: string): string {
  return storagePath(bucket, name);
}

export async function purgeStorageCDN(
  req: StorageCDNPurgeRequest,
): Promise<StorageCDNPurgeResponse> {
  const body = "purgeAll" in req
    ? { purge_all: true }
    : { urls: req.urls };
  return request<StorageCDNPurgeResponse>("/api/admin/storage/cdn/purge", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}
