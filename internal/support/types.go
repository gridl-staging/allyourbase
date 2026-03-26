package support

import (
	"context"
	"time"
)

const (
	TicketStatusOpen              = "open"
	TicketStatusInProgress        = "in_progress"
	TicketStatusWaitingOnCustomer = "waiting_on_customer"
	TicketStatusResolved          = "resolved"
	TicketStatusClosed            = "closed"
)

const (
	TicketPriorityLow    = "low"
	TicketPriorityNormal = "normal"
	TicketPriorityHigh   = "high"
	TicketPriorityUrgent = "urgent"
)

const (
	SenderCustomer = "customer"
	SenderSupport  = "support"
	SenderSystem   = "system"
)

// Ticket is a support ticket record.
type Ticket struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Subject   string    `json:"subject"`
	Status    string    `json:"status"`
	Priority  string    `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message is a support ticket message.
type Message struct {
	ID         string    `json:"id"`
	TicketID   string    `json:"ticket_id"`
	SenderType string    `json:"sender_type"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// TicketFilters controls ticket listing filter criteria.
type TicketFilters struct {
	Status   string `json:"status"`
	Priority string `json:"priority"`
	TenantID string `json:"tenant_id"`
}

// TicketUpdate controls partial ticket updates.
type TicketUpdate struct {
	Status   *string `json:"status,omitempty"`
	Priority *string `json:"priority,omitempty"`
}

// SupportService defines support ticket operations.
type SupportService interface {
	CreateTicket(ctx context.Context, tenantID, userID, subject, body, priority string) (*Ticket, error)
	ListTickets(ctx context.Context, tenantID string, filters TicketFilters) ([]Ticket, error)
	GetTicket(ctx context.Context, ticketID string) (*Ticket, error)
	UpdateTicket(ctx context.Context, ticketID string, updates TicketUpdate) (*Ticket, error)
	AddMessage(ctx context.Context, ticketID, senderType, body string) (*Message, error)
	ListMessages(ctx context.Context, ticketID string) ([]Message, error)
}
