import { request, requestNoBody } from "./api_client";
import type {
  ReplicasResponse,
  AddReplicaRequest,
  AddReplicaResponse,
  PromoteReplicaResponse,
  FailoverRequest,
} from "./types/replicas";

export function listReplicas(): Promise<ReplicasResponse> {
  return request<ReplicasResponse>("/api/admin/replicas");
}

export function checkReplicas(): Promise<ReplicasResponse> {
  return request<ReplicasResponse>("/api/admin/replicas/check", {
    method: "POST",
  });
}

export function addReplica(req: AddReplicaRequest): Promise<AddReplicaResponse> {
  return request<AddReplicaResponse>("/api/admin/replicas", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function removeReplica(name: string): Promise<void> {
  return requestNoBody(`/api/admin/replicas/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

export function promoteReplica(name: string): Promise<PromoteReplicaResponse> {
  return request<PromoteReplicaResponse>(
    `/api/admin/replicas/${encodeURIComponent(name)}/promote`,
    { method: "POST" },
  );
}

export function failover(req: FailoverRequest): Promise<{ status: string }> {
  return request<{ status: string }>("/api/admin/replicas/failover", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}
