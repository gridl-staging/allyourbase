// Package server support_handler.go provides HTTP handlers for the support ticket system, serving both customer and administrator endpoints.
package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/support"
	"github.com/go-chi/chi/v5"
)

const supportWebhookMaxBodySize = 2 << 20

type createSupportTicketRequest struct {
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Priority string `json:"priority"`
}

type addSupportMessageRequest struct {
	Body string `json:"body"`
}

type updateSupportTicketRequest struct {
	Status   *string `json:"status"`
	Priority *string `json:"priority"`
}

type supportTicketWithMessagesResponse struct {
	Ticket   *support.Ticket   `json:"ticket"`
	Messages []support.Message `json:"messages"`
}

func (s *Server) supportNotConfigured(w http.ResponseWriter) bool {
	if s.supportSvc != nil {
		return false
	}
	httputil.WriteError(w, http.StatusNotImplemented, "support service is not configured")
	return true
}

// mapSupportError maps support service errors to HTTP error responses, returning true if the error was handled.
func mapSupportError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, support.ErrTicketNotFound):
		httputil.WriteError(w, http.StatusNotFound, "support ticket not found")
		return true
	case errors.Is(err, support.ErrEmptySubject),
		errors.Is(err, support.ErrEmptyBody),
		errors.Is(err, support.ErrInvalidPriority),
		errors.Is(err, support.ErrInvalidSender),
		errors.Is(err, support.ErrNoTicketUpdates),
		errors.Is(err, support.ErrInvalidStatus):
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return true
	default:
		return false
	}
}

// loadCustomerTicket retrieves a support ticket and verifies it belongs to the specified tenant.
func (s *Server) loadCustomerTicket(w http.ResponseWriter, r *http.Request, ticketID, tenantID string) (*support.Ticket, bool) {
	ticket, err := s.supportSvc.GetTicket(r.Context(), ticketID)
	if err != nil {
		if mapSupportError(w, err) {
			return nil, false
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get support ticket")
		return nil, false
	}
	if ticket == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get support ticket")
		return nil, false
	}
	if ticket.TenantID != "" && ticket.TenantID != tenantID {
		httputil.WriteError(w, http.StatusNotFound, "support ticket not found")
		return nil, false
	}
	return ticket, true
}

// handleCreateSupportTicket handles requests to create a new support ticket for the authenticated user.
func (s *Server) handleCreateSupportTicket(w http.ResponseWriter, r *http.Request) {
	if s.supportNotConfigured(w) {
		return
	}
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req createSupportTicketRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	ticket, err := s.supportSvc.CreateTicket(r.Context(), claims.TenantID, claims.Subject, req.Subject, req.Body, req.Priority)
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create support ticket")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, ticket)
}

// handleListSupportTickets handles requests to list support tickets for the authenticated user's tenant, optionally filtered by status and priority.
func (s *Server) handleListSupportTickets(w http.ResponseWriter, r *http.Request) {
	if s.supportNotConfigured(w) {
		return
	}
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	filters := support.TicketFilters{
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Priority: strings.TrimSpace(r.URL.Query().Get("priority")),
	}

	tickets, err := s.supportSvc.ListTickets(r.Context(), claims.TenantID, filters)
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list support tickets")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tickets)
}

// handleGetSupportTicket handles requests to retrieve a support ticket and its messages for the authenticated user's tenant.
func (s *Server) handleGetSupportTicket(w http.ResponseWriter, r *http.Request) {
	if s.supportNotConfigured(w) {
		return
	}
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	ticketID := strings.TrimSpace(chi.URLParam(r, "id"))
	if !httputil.IsValidUUID(ticketID) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid support ticket id")
		return
	}

	ticket, ok := s.loadCustomerTicket(w, r, ticketID, claims.TenantID)
	if !ok {
		return
	}

	messages, err := s.supportSvc.ListMessages(r.Context(), ticketID)
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list support messages")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, supportTicketWithMessagesResponse{Ticket: ticket, Messages: messages})
}

// handleAddSupportMessage handles requests to add a customer message to a support ticket.
func (s *Server) handleAddSupportMessage(w http.ResponseWriter, r *http.Request) {
	if s.supportNotConfigured(w) {
		return
	}
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	ticketID := strings.TrimSpace(chi.URLParam(r, "id"))
	if !httputil.IsValidUUID(ticketID) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid support ticket id")
		return
	}

	var req addSupportMessageRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if _, ok := s.loadCustomerTicket(w, r, ticketID, claims.TenantID); !ok {
		return
	}

	msg, err := s.supportSvc.AddMessage(r.Context(), ticketID, support.SenderCustomer, req.Body)
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to add support message")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, msg)
}

// handleAdminListSupportTickets handles admin requests to list all support tickets across tenants, optionally filtered by status, priority, and tenant ID.
func (s *Server) handleAdminListSupportTickets(w http.ResponseWriter, r *http.Request) {
	if s.supportNotConfigured(w) {
		return
	}

	filters := support.TicketFilters{
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Priority: strings.TrimSpace(r.URL.Query().Get("priority")),
		TenantID: strings.TrimSpace(r.URL.Query().Get("tenant_id")),
	}

	tickets, err := s.supportSvc.ListTickets(r.Context(), "", filters)
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list support tickets")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tickets)
}

// handleAdminGetSupportTicket handles admin requests to retrieve a support ticket and its messages.
func (s *Server) handleAdminGetSupportTicket(w http.ResponseWriter, r *http.Request) {
	if s.supportNotConfigured(w) {
		return
	}
	ticketID := strings.TrimSpace(chi.URLParam(r, "id"))
	if !httputil.IsValidUUID(ticketID) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid support ticket id")
		return
	}

	ticket, err := s.supportSvc.GetTicket(r.Context(), ticketID)
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get support ticket")
		return
	}
	if ticket == nil {
		httputil.WriteError(w, http.StatusNotFound, "support ticket not found")
		return
	}

	messages, err := s.supportSvc.ListMessages(r.Context(), ticketID)
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list support messages")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, supportTicketWithMessagesResponse{Ticket: ticket, Messages: messages})
}

// handleAdminUpdateSupportTicket handles admin requests to update a support ticket's status and/or priority.
func (s *Server) handleAdminUpdateSupportTicket(w http.ResponseWriter, r *http.Request) {
	if s.supportNotConfigured(w) {
		return
	}
	ticketID := strings.TrimSpace(chi.URLParam(r, "id"))
	if !httputil.IsValidUUID(ticketID) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid support ticket id")
		return
	}

	var req updateSupportTicketRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	ticket, err := s.supportSvc.UpdateTicket(r.Context(), ticketID, support.TicketUpdate{Status: req.Status, Priority: req.Priority})
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to update support ticket")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, ticket)
}

// handleAdminAddSupportMessage handles admin requests to add a support agent message to a support ticket.
func (s *Server) handleAdminAddSupportMessage(w http.ResponseWriter, r *http.Request) {
	if s.supportNotConfigured(w) {
		return
	}
	ticketID := strings.TrimSpace(chi.URLParam(r, "id"))
	if !httputil.IsValidUUID(ticketID) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid support ticket id")
		return
	}

	var req addSupportMessageRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	msg, err := s.supportSvc.AddMessage(r.Context(), ticketID, support.SenderSupport, req.Body)
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to add support message")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, msg)
}

func (s *Server) supportWebhookAuthorized(r *http.Request) bool {
	secret := strings.TrimSpace(s.cfg.Support.WebhookSecret)
	got := strings.TrimSpace(r.Header.Get("X-Webhook-Secret"))
	if secret == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}

func (s *Server) supportTicketUserEmail(ctx context.Context, userID string) (string, error) {
	if s.supportUserEmailLookup != nil {
		return s.supportUserEmailLookup(ctx, userID)
	}
	if s.authSvc == nil {
		return "", fmt.Errorf("auth service is not configured")
	}
	user, err := s.authSvc.UserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(user.Email), nil
}

// handleSupportEmailWebhook handles inbound support email webhooks, extracting the ticket ID from the email subject and adding the message to the corresponding ticket.
func (s *Server) handleSupportEmailWebhook(w http.ResponseWriter, r *http.Request) {
	if s.supportNotConfigured(w) {
		return
	}
	if !s.supportWebhookAuthorized(r) {
		httputil.WriteError(w, http.StatusUnauthorized, "invalid support webhook auth")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, supportWebhookMaxBodySize)
	email, err := support.ParseInboundEmail(r)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httputil.WriteError(w, http.StatusRequestEntityTooLarge, "support webhook payload too large")
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, "invalid inbound email payload")
		return
	}

	domain := strings.ToLower(strings.TrimSpace(s.cfg.Support.InboundEmailDomain))
	if domain == "" {
		httputil.WriteError(w, http.StatusBadRequest, "support inbound email domain not configured")
		return
	}
	to, err := support.NormalizeEmailAddress(email.To)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid inbound email payload")
		return
	}
	if !strings.HasSuffix(to, "@"+domain) {
		httputil.WriteError(w, http.StatusBadRequest, "inbound email domain mismatch")
		return
	}

	ticketID, ok := support.ExtractTicketIDFromSubject(email.Subject)
	if !ok {
		s.currentLogger().Info("support inbound email ignored: no ticket id in subject", "subject", email.Subject, "from", email.From, "to", email.To)
		httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	ticket, err := s.supportSvc.GetTicket(r.Context(), ticketID)
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load support ticket")
		return
	}
	if ticket == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load support ticket")
		return
	}

	from, err := support.NormalizeEmailAddress(email.From)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid inbound email payload")
		return
	}
	userEmail, err := s.supportTicketUserEmail(r.Context(), ticket.UserID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to authorize inbound support email")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(userEmail), from) {
		s.currentLogger().Warn("support inbound email rejected: sender mismatch", "ticket_id", ticketID, "from", from, "user_id", ticket.UserID)
		httputil.WriteError(w, http.StatusForbidden, "support sender mismatch")
		return
	}

	msg, err := s.supportSvc.AddMessage(r.Context(), ticketID, support.SenderCustomer, email.Text)
	if err != nil {
		if mapSupportError(w, err) {
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to process inbound support email")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, msg)
}
