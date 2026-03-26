import { request } from "./api_client";
import type {
  Ticket,
  TicketWithMessages,
  UpdateTicketRequest,
  AddMessageRequest,
  Message,
} from "./types/support";

export function adminListTickets(filters?: {
  status?: string;
  priority?: string;
  tenant_id?: string;
}): Promise<Ticket[]> {
  const params = new URLSearchParams();
  if (filters?.status) params.set("status", filters.status);
  if (filters?.priority) params.set("priority", filters.priority);
  if (filters?.tenant_id) params.set("tenant_id", filters.tenant_id);
  const qs = params.toString();
  return request<Ticket[]>(
    `/api/admin/support/tickets${qs ? `?${qs}` : ""}`,
  );
}

export function adminGetTicket(id: string): Promise<TicketWithMessages> {
  return request<TicketWithMessages>(
    `/api/admin/support/tickets/${encodeURIComponent(id)}`,
  );
}

export function adminUpdateTicket(
  id: string,
  req: UpdateTicketRequest,
): Promise<Ticket> {
  return request<Ticket>(
    `/api/admin/support/tickets/${encodeURIComponent(id)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

export function adminAddMessage(
  id: string,
  req: AddMessageRequest,
): Promise<Message> {
  return request<Message>(
    `/api/admin/support/tickets/${encodeURIComponent(id)}/messages`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}
