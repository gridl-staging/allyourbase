export interface ForeignServer {
  name: string;
  fdw_type: string;
  options: Record<string, string>;
  created_at: string;
}

export interface ForeignColumn {
  name: string;
  type: string;
}

export interface ForeignTable {
  schema: string;
  name: string;
  server_name: string;
  columns: ForeignColumn[];
  options: Record<string, string>;
}

export interface UserMapping {
  user: string;
  password: string;
}

export interface CreateServerRequest {
  name: string;
  fdw_type: string;
  options: Record<string, string>;
  user_mapping?: UserMapping;
}

export interface ImportTablesRequest {
  remote_schema: string;
  local_schema: string;
  table_names?: string[];
}
