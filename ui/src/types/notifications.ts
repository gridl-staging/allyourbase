export interface Notification {
  id: string;
  user_id: string;
  title: string;
  body?: string;
  metadata?: Record<string, unknown>;
  channel: string;
  read_at?: string;
  created_at: string;
}

export interface CreateNotificationRequest {
  user_id: string;
  title: string;
  body?: string;
  metadata?: Record<string, unknown>;
  channel: string;
}
