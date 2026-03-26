export type IncidentStatus =
  | "investigating"
  | "identified"
  | "monitoring"
  | "resolved";

export interface IncidentUpdateEntry {
  id: string;
  incidentId: string;
  message: string;
  status: IncidentStatus;
  createdAt: string;
}

export interface Incident {
  id: string;
  title: string;
  status: IncidentStatus;
  affectedServices: string[];
  createdAt: string;
  updatedAt: string;
  resolvedAt?: string;
  updates?: IncidentUpdateEntry[];
}

// Request types use snake_case (from handler json tags)
export interface CreateIncidentRequest {
  title: string;
  status: string;
  affected_services: string[];
}

export interface UpdateIncidentRequest {
  title?: string;
  status?: string;
}

export interface AddUpdateRequest {
  message: string;
  status: string;
}
