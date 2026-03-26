export interface RequestLogEntry {
  id: string;
  timestamp: string;
  method: string;
  path: string;
  status_code: number;
  duration_ms: number;
  user_id?: string;
  api_key_id?: string;
  request_size: number;
  response_size: number;
  ip_address?: string;
  request_id?: string;
}

export interface RequestLogListResponse {
  items: RequestLogEntry[];
  count: number;
  limit: number;
  offset: number;
}

export interface IndexSuggestion {
  statement: string;
  confidence: string;
}

export interface QueryStat {
  queryid: string;
  query: string;
  calls: number;
  total_exec_time: number;
  mean_exec_time: number;
  rows: number;
  shared_blks_hit: number;
  shared_blks_read: number;
  index_suggestions?: IndexSuggestion[];
}

export interface QueryAnalyticsResponse {
  items: QueryStat[];
  count: number;
  limit: number;
  sort: string;
}
