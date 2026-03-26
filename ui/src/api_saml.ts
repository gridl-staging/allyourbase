import { request, requestNoBody } from "./api_client";
import type { SAMLProvider, SAMLUpsertRequest } from "./types/saml";

type SAMLProviderListResponse =
  | SAMLProvider[]
  | {
      providers?: SAMLProvider[];
    };

export async function listSAMLProviders(): Promise<SAMLProvider[]> {
  const response = await request<SAMLProviderListResponse>("/api/admin/auth/saml");
  if (Array.isArray(response)) {
    return response;
  }
  if (Array.isArray(response?.providers)) {
    return response.providers;
  }
  return [];
}

export function createSAMLProvider(
  req: SAMLUpsertRequest,
): Promise<SAMLProvider> {
  return request<SAMLProvider>("/api/admin/auth/saml", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function updateSAMLProvider(
  name: string,
  req: SAMLUpsertRequest,
): Promise<SAMLProvider> {
  return request<SAMLProvider>(
    `/api/admin/auth/saml/${encodeURIComponent(name)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

export function deleteSAMLProvider(name: string): Promise<void> {
  return requestNoBody(
    `/api/admin/auth/saml/${encodeURIComponent(name)}`,
    { method: "DELETE" },
  );
}
