export interface AuditLogEntry {
  id: string;
  timestamp: string;
  user_id?: string;
  api_key_id?: string;
  table_name: string;
  record_id?: unknown;
  operation: string;
  old_values?: unknown;
  new_values?: unknown;
  ip_address?: string;
}

export interface AuditLogListResponse {
  items: AuditLogEntry[];
  count: number;
  limit: number;
  offset: number;
}
