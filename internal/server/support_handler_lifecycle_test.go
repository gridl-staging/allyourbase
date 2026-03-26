package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/support"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

func TestSupportHandlerLifecycleIntegration(t *testing.T) {
	t.Parallel()

	svc := newMemorySupportService()
	srv := supportTestServer(svc)
	r := chi.NewRouter()
	r.Post("/api/support/tickets", srv.handleCreateSupportTicket)
	r.Get("/api/support/tickets/{id}", srv.handleGetSupportTicket)
	r.Post("/api/support/tickets/{id}/messages", srv.handleAddSupportMessage)
	r.Post("/api/admin/support/tickets/{id}/messages", srv.handleAdminAddSupportMessage)
	r.Put("/api/admin/support/tickets/{id}", srv.handleAdminUpdateSupportTicket)

	createReq := httptest.NewRequest(http.MethodPost, "/api/support/tickets", strings.NewReader(`{"subject":"Need help","body":"initial","priority":"normal"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = createReq.WithContext(auth.ContextWithClaims(createReq.Context(), supportClaims("tenant-1", "user-1")))
	createW := httptest.NewRecorder()
	r.ServeHTTP(createW, createReq)
	testutil.Equal(t, http.StatusCreated, createW.Code)

	var created support.Ticket
	testutil.NoError(t, json.Unmarshal(createW.Body.Bytes(), &created))

	adminReplyReq := httptest.NewRequest(http.MethodPost, "/api/admin/support/tickets/"+created.ID+"/messages", strings.NewReader(`{"body":"admin reply"}`))
	adminReplyReq.Header.Set("Content-Type", "application/json")
	adminReplyW := httptest.NewRecorder()
	r.ServeHTTP(adminReplyW, adminReplyReq)
	testutil.Equal(t, http.StatusCreated, adminReplyW.Code)

	userReplyReq := httptest.NewRequest(http.MethodPost, "/api/support/tickets/"+created.ID+"/messages", strings.NewReader(`{"body":"customer reply"}`))
	userReplyReq.Header.Set("Content-Type", "application/json")
	userReplyReq = userReplyReq.WithContext(auth.ContextWithClaims(userReplyReq.Context(), supportClaims("tenant-1", "user-1")))
	userReplyW := httptest.NewRecorder()
	r.ServeHTTP(userReplyW, userReplyReq)
	testutil.Equal(t, http.StatusCreated, userReplyW.Code)

	resolveReq := httptest.NewRequest(http.MethodPut, "/api/admin/support/tickets/"+created.ID, strings.NewReader(`{"status":"resolved"}`))
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveW := httptest.NewRecorder()
	r.ServeHTTP(resolveW, resolveReq)
	testutil.Equal(t, http.StatusOK, resolveW.Code)

	getReq := httptest.NewRequest(http.MethodGet, "/api/support/tickets/"+created.ID, nil)
	getReq = getReq.WithContext(auth.ContextWithClaims(getReq.Context(), supportClaims("tenant-1", "user-1")))
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)
	testutil.Equal(t, http.StatusOK, getW.Code)

	var got struct {
		Ticket   support.Ticket    `json:"ticket"`
		Messages []support.Message `json:"messages"`
	}
	testutil.NoError(t, json.Unmarshal(getW.Body.Bytes(), &got))
	testutil.Equal(t, support.TicketStatusResolved, got.Ticket.Status)
	testutil.SliceLen(t, got.Messages, 3)
	testutil.Equal(t, "initial", got.Messages[0].Body)
	testutil.Equal(t, "admin reply", got.Messages[1].Body)
	testutil.Equal(t, "customer reply", got.Messages[2].Body)
}

type memorySupportService struct {
	mu       sync.Mutex
	tickets  map[string]*support.Ticket
	messages map[string][]support.Message
	nextID   int
	nextMsg  int
}

func newMemorySupportService() *memorySupportService {
	return &memorySupportService{
		tickets:  make(map[string]*support.Ticket),
		messages: make(map[string][]support.Message),
		nextID:   1,
		nextMsg:  1,
	}
}

func (m *memorySupportService) CreateTicket(ctx context.Context, tenantID, userID, subject, body, priority string) (*support.Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := "00000000-0000-0000-0000-" + fmt12(m.nextID)
	m.nextID++
	now := time.Now().UTC()
	if priority == "" {
		priority = support.TicketPriorityNormal
	}
	t := &support.Ticket{ID: id, TenantID: tenantID, UserID: userID, Subject: subject, Status: support.TicketStatusOpen, Priority: priority, CreatedAt: now, UpdatedAt: now}
	m.tickets[id] = t
	m.messages[id] = append(m.messages[id], support.Message{ID: "00000000-0000-0000-0000-" + fmt12(m.nextMsg), TicketID: id, SenderType: support.SenderCustomer, Body: body, CreatedAt: now})
	m.nextMsg++
	cp := *t
	return &cp, nil
}

func (m *memorySupportService) ListTickets(ctx context.Context, tenantID string, filters support.TicketFilters) ([]support.Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]support.Ticket, 0)
	for _, t := range m.tickets {
		if tenantID != "" && t.TenantID != tenantID {
			continue
		}
		if filters.TenantID != "" && t.TenantID != filters.TenantID {
			continue
		}
		if filters.Status != "" && t.Status != filters.Status {
			continue
		}
		if filters.Priority != "" && t.Priority != filters.Priority {
			continue
		}
		out = append(out, *t)
	}
	return out, nil
}

func (m *memorySupportService) GetTicket(ctx context.Context, ticketID string) (*support.Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tickets[ticketID]
	if !ok {
		return nil, support.ErrTicketNotFound
	}
	cp := *t
	return &cp, nil
}

func (m *memorySupportService) UpdateTicket(ctx context.Context, ticketID string, updates support.TicketUpdate) (*support.Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tickets[ticketID]
	if !ok {
		return nil, support.ErrTicketNotFound
	}
	if updates.Status == nil && updates.Priority == nil {
		return nil, support.ErrNoTicketUpdates
	}
	if updates.Status != nil {
		t.Status = *updates.Status
	}
	if updates.Priority != nil {
		t.Priority = *updates.Priority
	}
	t.UpdatedAt = time.Now().UTC()
	cp := *t
	return &cp, nil
}

func (m *memorySupportService) AddMessage(ctx context.Context, ticketID, senderType, body string) (*support.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tickets[ticketID]
	if !ok {
		return nil, support.ErrTicketNotFound
	}
	msg := support.Message{ID: "00000000-0000-0000-0000-" + fmt12(m.nextMsg), TicketID: ticketID, SenderType: senderType, Body: body, CreatedAt: time.Now().UTC()}
	m.nextMsg++
	m.messages[ticketID] = append(m.messages[ticketID], msg)
	t.UpdatedAt = msg.CreatedAt
	return &msg, nil
}

func (m *memorySupportService) ListMessages(ctx context.Context, ticketID string) ([]support.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs := m.messages[ticketID]
	out := make([]support.Message, len(msgs))
	copy(out, msgs)
	return out, nil
}

func fmt12(v int) string {
	s := strconv.Itoa(v)
	if len(s) >= 12 {
		return s
	}
	return strings.Repeat("0", 12-len(s)) + s
}

func TestSupportNotConfigured(t *testing.T) {
	t.Parallel()
	srv := supportTestServer(nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/support/tickets", nil)
	srv.handleListSupportTickets(w, req)
	testutil.Equal(t, http.StatusNotImplemented, w.Code)
}
