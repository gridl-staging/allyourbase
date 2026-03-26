export interface SMSWindowStats {
  sent: number;
  confirmed: number;
  failed: number;
  conversion_rate: number;
}

export interface SMSHealthResponse {
  today: SMSWindowStats;
  last_7d: SMSWindowStats;
  last_30d: SMSWindowStats;
  warning?: string;
}

export interface SMSMessage {
  id: string;
  to: string;
  body: string;
  provider: string;
  message_id: string;
  status: string;
  created_at: string;
  updated_at: string;
  error_message?: string;
  user_id?: string;
}

export interface SMSMessageListResponse {
  items: SMSMessage[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export interface SMSSendResponse {
  id?: string;
  message_id: string;
  status: string;
  to: string;
}
