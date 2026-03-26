export interface VectorIndexInfo {
  name: string;
  schema: string;
  table: string;
  method: string;
  definition: string;
}

export interface CreateVectorIndexRequest {
  schema: string;
  table: string;
  column: string;
  method: string;
  metric: string;
  index_name?: string;
  lists?: number;
}
