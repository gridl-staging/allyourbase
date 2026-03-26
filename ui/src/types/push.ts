/**
 * @module Type definitions for push notification devices, deliveries, and API responses.
 */
export type PushProvider = "fcm" | "apns";
export type PushPlatform = "android" | "ios";
export type PushDeliveryStatus = "pending" | "sent" | "failed" | "invalid_token";

export interface PushDeviceToken {
  id: string;
  app_id: string;
  user_id: string;
  provider: PushProvider;
  platform: PushPlatform;
  token: string;
  device_name?: string;
  is_active: boolean;
  last_used?: string;
  last_refreshed_at: string;
  created_at: string;
  updated_at: string;
}

/**
 * Represents a single push notification delivery attempt. Tracks the notification content (title, body, data), delivery status, provider metadata, and associated device/user/job references.
 */
export interface PushDelivery {
  id: string;
  device_token_id: string;
  job_id?: string;
  app_id: string;
  user_id: string;
  provider: PushProvider;
  title: string;
  body: string;
  data_payload?: Record<string, string>;
  status: PushDeliveryStatus;
  error_code?: string | null;
  error_message?: string | null;
  provider_message_id?: string | null;
  sent_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface PushDeviceListResponse {
  items: PushDeviceToken[];
}

export interface PushDeliveryListResponse {
  items: PushDelivery[];
}

export interface PushSendResponse {
  deliveries: PushDelivery[];
}
