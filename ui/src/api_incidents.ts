import { request } from "./api_client";
import type {
  Incident,
  IncidentUpdateEntry,
  CreateIncidentRequest,
  UpdateIncidentRequest,
  AddUpdateRequest,
} from "./types/incidents";

export function listIncidents(active?: boolean): Promise<Incident[]> {
  const params = active != null ? `?active=${active}` : "";
  return request<Incident[]>(`/api/admin/incidents${params}`);
}

export function createIncident(
  req: CreateIncidentRequest,
): Promise<Incident> {
  return request<Incident>("/api/admin/incidents", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function updateIncident(
  id: string,
  req: UpdateIncidentRequest,
): Promise<Incident> {
  return request<Incident>(
    `/api/admin/incidents/${encodeURIComponent(id)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

export function addIncidentUpdate(
  id: string,
  req: AddUpdateRequest,
): Promise<IncidentUpdateEntry> {
  return request<IncidentUpdateEntry>(
    `/api/admin/incidents/${encodeURIComponent(id)}/updates`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}
