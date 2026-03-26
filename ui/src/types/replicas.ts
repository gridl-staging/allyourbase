export interface ReplicaConnections {
  total: number;
  idle: number;
  in_use: number;
}

export interface ReplicaStatus {
  name: string;
  url: string;
  state: string;
  lag_bytes: number;
  weight: number;
  connections: ReplicaConnections;
  last_checked_at: string;
  last_error?: string;
}

export interface ReplicasResponse {
  replicas: ReplicaStatus[];
}

export interface TopologyNode {
  name: string;
  host: string;
  port: number;
  database: string;
  ssl_mode: string;
  weight: number;
  max_lag_bytes: number;
  role: string;
  state: string;
}

export interface AddReplicaRequest {
  name: string;
  host: string;
  port: number;
  database: string;
  ssl_mode: string;
  weight: number;
  max_lag_bytes: number;
}

export interface AddReplicaResponse {
  status: string;
  record: TopologyNode;
  replicas: ReplicaStatus[];
}

export interface PromoteReplicaResponse {
  status: string;
  primary: TopologyNode;
  replicas: ReplicaStatus[];
}

export interface FailoverRequest {
  target: string;
  force: boolean;
}
