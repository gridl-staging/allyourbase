import { request, requestNoBody } from "./api_client";
import type {
  DomainBindingListResult,
  DomainBinding,
  CreateDomainRequest,
} from "./types/domains";

export function listDomains(): Promise<DomainBindingListResult> {
  return request<DomainBindingListResult>("/api/admin/domains");
}

export function createDomain(
  req: CreateDomainRequest,
): Promise<DomainBinding> {
  return request<DomainBinding>("/api/admin/domains", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function deleteDomain(id: string): Promise<void> {
  return requestNoBody(`/api/admin/domains/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export function verifyDomain(
  id: string,
): Promise<DomainBinding> {
  return request<DomainBinding>(
    `/api/admin/domains/${encodeURIComponent(id)}/verify`,
    { method: "POST" },
  );
}
