export interface WebhookResponse {
  id: string;
  url: string;
  hasSecret: boolean;
  events: string[];
  tables: string[];
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface WebhookRequest {
  url: string;
  secret?: string;
  events?: string[];
  tables?: string[];
  enabled?: boolean;
}

export interface WebhookTestResult {
  success: boolean;
  statusCode?: number;
  durationMs: number;
  error?: string;
}

export interface WebhookDelivery {
  id: string;
  webhookId: string;
  eventAction: string;
  eventTable: string;
  success: boolean;
  statusCode: number;
  attempt: number;
  durationMs: number;
  error?: string;
  requestBody?: string;
  responseBody?: string;
  deliveredAt: string;
}

export interface DeliveryListResponse {
  items: WebhookDelivery[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}
