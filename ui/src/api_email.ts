import type {
  EmailTemplateListResponse,
  EmailTemplateEffective,
  UpsertEmailTemplateRequest,
  UpsertEmailTemplateResponse,
  SetEmailTemplateEnabledResponse,
  PreviewEmailTemplateRequest,
  PreviewEmailTemplateResponse,
  SendTemplateEmailRequest,
  SendTemplateEmailResponse,
} from "./types";
import {
  request,
  requestNoBody,
} from "./api_client";

export async function listEmailTemplates(): Promise<EmailTemplateListResponse> {
  return request("/api/admin/email/templates");
}

export async function getEmailTemplate(
  key: string,
): Promise<EmailTemplateEffective> {
  return request(`/api/admin/email/templates/${encodeURIComponent(key)}`);
}

export async function upsertEmailTemplate(
  key: string,
  data: UpsertEmailTemplateRequest,
): Promise<UpsertEmailTemplateResponse> {
  return request(`/api/admin/email/templates/${encodeURIComponent(key)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteEmailTemplate(key: string): Promise<void> {
  return requestNoBody(`/api/admin/email/templates/${encodeURIComponent(key)}`, {
    method: "DELETE",
  });
}

export async function setEmailTemplateEnabled(
  key: string,
  enabled: boolean,
): Promise<SetEmailTemplateEnabledResponse> {
  return request(`/api/admin/email/templates/${encodeURIComponent(key)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  });
}

export async function previewEmailTemplate(
  key: string,
  data: PreviewEmailTemplateRequest,
): Promise<PreviewEmailTemplateResponse> {
  return request(`/api/admin/email/templates/${encodeURIComponent(key)}/preview`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function sendTemplateEmail(
  data: SendTemplateEmailRequest,
): Promise<SendTemplateEmailResponse> {
  return request("/api/admin/email/send", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}
