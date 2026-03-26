import { request, requestNoBody } from "./api_client";
import type {
  SecretMetadata,
  SecretValue,
  CreateSecretRequest,
  UpdateSecretRequest,
} from "./types/secrets";

export function listSecrets(): Promise<SecretMetadata[]> {
  return request<SecretMetadata[]>("/api/admin/secrets");
}

export function getSecret(name: string): Promise<SecretValue> {
  return request<SecretValue>(
    `/api/admin/secrets/${encodeURIComponent(name)}`,
  );
}

export function createSecret(req: CreateSecretRequest): Promise<void> {
  return requestNoBody("/api/admin/secrets", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function updateSecret(
  name: string,
  req: UpdateSecretRequest,
): Promise<void> {
  return requestNoBody(`/api/admin/secrets/${encodeURIComponent(name)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function deleteSecret(name: string): Promise<void> {
  return requestNoBody(`/api/admin/secrets/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

export function rotateSecrets(): Promise<{ status: string }> {
  return request<{ status: string }>("/api/admin/secrets/rotate", {
    method: "POST",
  });
}
