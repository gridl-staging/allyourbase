export type TicketStatus =
  | "open"
  | "in_progress"
  | "waiting_on_customer"
  | "resolved"
  | "closed";

export type TicketPriority = "low" | "normal" | "high" | "urgent";

export type SenderType = "customer" | "support" | "system";

export interface Ticket {
  id: string;
  tenant_id: string;
  user_id: string;
  subject: string;
  status: string;
  priority: string;
  created_at: string;
  updated_at: string;
}

export interface Message {
  id: string;
  ticket_id: string;
  sender_type: string;
  body: string;
  created_at: string;
}

export interface TicketWithMessages {
  ticket: Ticket;
  messages: Message[];
}

export interface UpdateTicketRequest {
  status?: string;
  priority?: string;
}

export interface AddMessageRequest {
  body: string;
}
