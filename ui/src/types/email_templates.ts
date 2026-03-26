export type EmailTemplateSource = "builtin" | "custom";

export interface EmailTemplateListItem {
  templateKey: string;
  source: EmailTemplateSource;
  subjectTemplate: string;
  enabled: boolean;
  updatedAt?: string;
}

export interface EmailTemplateListResponse {
  items: EmailTemplateListItem[];
  count: number;
}

export interface EmailTemplateEffective {
  source: EmailTemplateSource;
  templateKey: string;
  subjectTemplate: string;
  htmlTemplate: string;
  enabled: boolean;
  variables?: string[];
}

export interface UpsertEmailTemplateRequest {
  subjectTemplate: string;
  htmlTemplate: string;
}

export interface UpsertEmailTemplateResponse {
  templateKey: string;
  subjectTemplate: string;
  htmlTemplate: string;
  enabled: boolean;
}

export interface SetEmailTemplateEnabledResponse {
  templateKey: string;
  enabled: boolean;
}

export interface PreviewEmailTemplateRequest {
  subjectTemplate: string;
  htmlTemplate: string;
  variables: Record<string, string>;
}

export interface PreviewEmailTemplateResponse {
  subject: string;
  html: string;
  text: string;
}

export interface SendTemplateEmailRequest {
  templateKey: string;
  to: string;
  variables: Record<string, string>;
}

export interface SendTemplateEmailResponse {
  status: string;
}
