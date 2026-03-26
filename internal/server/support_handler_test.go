package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/support"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type fakeSupportService struct {
	createTicketFn func(ctx context.Context, tenantID, userID, subject, body, priority string) (*support.Ticket, error)
	listTicketsFn  func(ctx context.Context, tenantID string, filters support.TicketFilters) ([]support.Ticket, error)
	getTicketFn    func(ctx context.Context, ticketID string) (*support.Ticket, error)
	updateTicketFn func(ctx context.Context, ticketID string, updates support.TicketUpdate) (*support.Ticket, error)
	addMessageFn   func(ctx context.Context, ticketID, senderType, body string) (*support.Message, error)
	listMsgsFn     func(ctx context.Context, ticketID string) ([]support.Message, error)

	lastCreateTenantID string
	lastCreateUserID   string
	lastCreateSubject  string
	lastCreateBody     string
	lastCreatePriority string

	lastListTenantID string
	lastListFilters  support.TicketFilters

	lastGetTicketID string

	lastUpdateTicketID string
	lastUpdate         support.TicketUpdate

	lastAddTicketID string
	lastAddSender   string
	lastAddBody     string

	lastListMsgTicketID string
}

func (f *fakeSupportService) CreateTicket(ctx context.Context, tenantID, userID, subject, body, priority string) (*support.Ticket, error) {
	f.lastCreateTenantID = tenantID
	f.lastCreateUserID = userID
	f.lastCreateSubject = subject
	f.lastCreateBody = body
	f.lastCreatePriority = priority
	if f.createTicketFn != nil {
		return f.createTicketFn(ctx, tenantID, userID, subject, body, priority)
	}
	return &support.Ticket{ID: "00000000-0000-0000-0000-000000000101", TenantID: tenantID, UserID: userID, Subject: subject, Status: support.TicketStatusOpen, Priority: priority}, nil
}

func (f *fakeSupportService) ListTickets(ctx context.Context, tenantID string, filters support.TicketFilters) ([]support.Ticket, error) {
	f.lastListTenantID = tenantID
	f.lastListFilters = filters
	if f.listTicketsFn != nil {
		return f.listTicketsFn(ctx, tenantID, filters)
	}
	return []support.Ticket{}, nil
}

func (f *fakeSupportService) GetTicket(ctx context.Context, ticketID string) (*support.Ticket, error) {
	f.lastGetTicketID = ticketID
	if f.getTicketFn != nil {
		return f.getTicketFn(ctx, ticketID)
	}
	return &support.Ticket{ID: ticketID, TenantID: "tenant-1", UserID: "user-1", Subject: "subject", Status: support.TicketStatusOpen, Priority: support.TicketPriorityNormal}, nil
}

func (f *fakeSupportService) UpdateTicket(ctx context.Context, ticketID string, updates support.TicketUpdate) (*support.Ticket, error) {
	f.lastUpdateTicketID = ticketID
	f.lastUpdate = updates
	if f.updateTicketFn != nil {
		return f.updateTicketFn(ctx, ticketID, updates)
	}
	status := support.TicketStatusOpen
	priority := support.TicketPriorityNormal
	if updates.Status != nil {
		status = *updates.Status
	}
	if updates.Priority != nil {
		priority = *updates.Priority
	}
	return &support.Ticket{ID: ticketID, TenantID: "tenant-1", UserID: "user-1", Subject: "subject", Status: status, Priority: priority}, nil
}

func (f *fakeSupportService) AddMessage(ctx context.Context, ticketID, senderType, body string) (*support.Message, error) {
	f.lastAddTicketID = ticketID
	f.lastAddSender = senderType
	f.lastAddBody = body
	if f.addMessageFn != nil {
		return f.addMessageFn(ctx, ticketID, senderType, body)
	}
	return &support.Message{ID: "00000000-0000-0000-0000-000000000201", TicketID: ticketID, SenderType: senderType, Body: body, CreatedAt: time.Now().UTC()}, nil
}

func (f *fakeSupportService) ListMessages(ctx context.Context, ticketID string) ([]support.Message, error) {
	f.lastListMsgTicketID = ticketID
	if f.listMsgsFn != nil {
		return f.listMsgsFn(ctx, ticketID)
	}
	return []support.Message{}, nil
}

func supportClaims(tenantID, userID string) *auth.Claims {
	return &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: userID},
		TenantID:         tenantID,
		Email:            userID + "@example.com",
	}
}

func supportTestServer(svc support.SupportService) *Server {
	cfg := config.Default()
	cfg.Support.Enabled = true
	cfg.Support.InboundEmailDomain = "support.example.com"
	cfg.Support.WebhookSecret = "secret-1"
	return &Server{
		cfg:        cfg,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		supportSvc: svc,
		supportUserEmailLookup: func(ctx context.Context, userID string) (string, error) {
			return userID + "@example.com", nil
		},
	}
}

func withRouteID(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestHandleCreateSupportTicket(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		svc := &fakeSupportService{}
		srv := supportTestServer(svc)

		req := httptest.NewRequest(http.MethodPost, "/api/support/tickets", strings.NewReader(`{"subject":"Need help","body":"Please assist","priority":"high"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleCreateSupportTicket(w, req)
		testutil.Equal(t, http.StatusCreated, w.Code)
		testutil.Equal(t, "tenant-1", svc.lastCreateTenantID)
		testutil.Equal(t, "user-1", svc.lastCreateUserID)
		testutil.Equal(t, "Need help", svc.lastCreateSubject)
	})

	t.Run("missing auth", func(t *testing.T) {
		srv := supportTestServer(&fakeSupportService{})
		req := httptest.NewRequest(http.MethodPost, "/api/support/tickets", strings.NewReader(`{"subject":"s","body":"b"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.handleCreateSupportTicket(w, req)
		testutil.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("empty subject", func(t *testing.T) {
		svc := &fakeSupportService{createTicketFn: func(ctx context.Context, tenantID, userID, subject, body, priority string) (*support.Ticket, error) {
			return nil, support.ErrEmptySubject
		}}
		srv := supportTestServer(svc)
		req := httptest.NewRequest(http.MethodPost, "/api/support/tickets", strings.NewReader(`{"subject":"","body":"b"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleCreateSupportTicket(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty body", func(t *testing.T) {
		svc := &fakeSupportService{createTicketFn: func(ctx context.Context, tenantID, userID, subject, body, priority string) (*support.Ticket, error) {
			return nil, support.ErrEmptyBody
		}}
		srv := supportTestServer(svc)
		req := httptest.NewRequest(http.MethodPost, "/api/support/tickets", strings.NewReader(`{"subject":"x","body":""}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleCreateSupportTicket(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleListSupportTickets(t *testing.T) {
	t.Parallel()

	t.Run("success with filters", func(t *testing.T) {
		svc := &fakeSupportService{listTicketsFn: func(ctx context.Context, tenantID string, filters support.TicketFilters) ([]support.Ticket, error) {
			return []support.Ticket{{ID: "1", TenantID: tenantID, Subject: "s", Status: filters.Status, Priority: filters.Priority}}, nil
		}}
		srv := supportTestServer(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/support/tickets?status=open&priority=high", nil)
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleListSupportTickets(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.Equal(t, "tenant-1", svc.lastListTenantID)
		testutil.Equal(t, "open", svc.lastListFilters.Status)
		testutil.Equal(t, "high", svc.lastListFilters.Priority)
	})

	t.Run("no auth", func(t *testing.T) {
		srv := supportTestServer(&fakeSupportService{})
		req := httptest.NewRequest(http.MethodGet, "/api/support/tickets", nil)
		w := httptest.NewRecorder()

		srv.handleListSupportTickets(w, req)
		testutil.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestHandleGetSupportTicket(t *testing.T) {
	t.Parallel()

	t.Run("success returns ticket and messages", func(t *testing.T) {
		svc := &fakeSupportService{
			getTicketFn: func(ctx context.Context, ticketID string) (*support.Ticket, error) {
				return &support.Ticket{ID: ticketID, TenantID: "tenant-1", UserID: "user-1", Subject: "sub", Status: support.TicketStatusOpen, Priority: support.TicketPriorityNormal}, nil
			},
			listMsgsFn: func(ctx context.Context, ticketID string) ([]support.Message, error) {
				return []support.Message{{ID: "m1", TicketID: ticketID, SenderType: support.SenderCustomer, Body: "hello"}}, nil
			},
		}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodGet, "/api/support/tickets/00000000-0000-0000-0000-000000000111", nil), "00000000-0000-0000-0000-000000000111")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleGetSupportTicket(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.Equal(t, "00000000-0000-0000-0000-000000000111", svc.lastGetTicketID)
		testutil.Equal(t, "00000000-0000-0000-0000-000000000111", svc.lastListMsgTicketID)
	})

	t.Run("not found", func(t *testing.T) {
		svc := &fakeSupportService{getTicketFn: func(ctx context.Context, ticketID string) (*support.Ticket, error) {
			return nil, support.ErrTicketNotFound
		}}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodGet, "/api/support/tickets/00000000-0000-0000-0000-000000000111", nil), "00000000-0000-0000-0000-000000000111")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleGetSupportTicket(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("service returned nil ticket", func(t *testing.T) {
		svc := &fakeSupportService{getTicketFn: func(ctx context.Context, ticketID string) (*support.Ticket, error) {
			return nil, nil
		}}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodGet, "/api/support/tickets/00000000-0000-0000-0000-000000000111", nil), "00000000-0000-0000-0000-000000000111")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleGetSupportTicket(w, req)
		testutil.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("invalid uuid", func(t *testing.T) {
		srv := supportTestServer(&fakeSupportService{})
		req := withRouteID(httptest.NewRequest(http.MethodGet, "/api/support/tickets/not-a-uuid", nil), "not-a-uuid")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleGetSupportTicket(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleAddSupportMessage(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		svc := &fakeSupportService{}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodPost, "/api/support/tickets/00000000-0000-0000-0000-000000000111/messages", strings.NewReader(`{"body":"reply"}`)), "00000000-0000-0000-0000-000000000111")
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleAddSupportMessage(w, req)
		testutil.Equal(t, http.StatusCreated, w.Code)
		testutil.Equal(t, support.SenderCustomer, svc.lastAddSender)
	})

	t.Run("not found", func(t *testing.T) {
		svc := &fakeSupportService{addMessageFn: func(ctx context.Context, ticketID, senderType, body string) (*support.Message, error) {
			return nil, support.ErrTicketNotFound
		}}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodPost, "/api/support/tickets/00000000-0000-0000-0000-000000000111/messages", strings.NewReader(`{"body":"reply"}`)), "00000000-0000-0000-0000-000000000111")
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleAddSupportMessage(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("empty body", func(t *testing.T) {
		svc := &fakeSupportService{addMessageFn: func(ctx context.Context, ticketID, senderType, body string) (*support.Message, error) {
			return nil, support.ErrEmptyBody
		}}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodPost, "/api/support/tickets/00000000-0000-0000-0000-000000000111/messages", strings.NewReader(`{"body":""}`)), "00000000-0000-0000-0000-000000000111")
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleAddSupportMessage(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ticket tenant mismatch", func(t *testing.T) {
		svc := &fakeSupportService{
			getTicketFn: func(ctx context.Context, ticketID string) (*support.Ticket, error) {
				return &support.Ticket{ID: ticketID, TenantID: "tenant-2", UserID: "user-2", Subject: "sub", Status: support.TicketStatusOpen, Priority: support.TicketPriorityNormal}, nil
			},
		}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodPost, "/api/support/tickets/00000000-0000-0000-0000-000000000111/messages", strings.NewReader(`{"body":"reply"}`)), "00000000-0000-0000-0000-000000000111")
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(auth.ContextWithClaims(req.Context(), supportClaims("tenant-1", "user-1")))
		w := httptest.NewRecorder()

		srv.handleAddSupportMessage(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
		testutil.Equal(t, "", svc.lastAddTicketID)
	})

	t.Run("no auth", func(t *testing.T) {
		srv := supportTestServer(&fakeSupportService{})
		req := withRouteID(httptest.NewRequest(http.MethodPost, "/api/support/tickets/00000000-0000-0000-0000-000000000111/messages", strings.NewReader(`{"body":"x"}`)), "00000000-0000-0000-0000-000000000111")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.handleAddSupportMessage(w, req)
		testutil.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestHandleAdminListSupportTickets(t *testing.T) {
	t.Parallel()
	svc := &fakeSupportService{}
	srv := supportTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/support/tickets?status=open&priority=high&tenant_id=tenant-9", nil)
	w := httptest.NewRecorder()

	srv.handleAdminListSupportTickets(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "", svc.lastListTenantID)
	testutil.Equal(t, "tenant-9", svc.lastListFilters.TenantID)
	testutil.Equal(t, "open", svc.lastListFilters.Status)
	testutil.Equal(t, "high", svc.lastListFilters.Priority)
}

func TestHandleAdminUpdateSupportTicket(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		svc := &fakeSupportService{}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodPut, "/api/admin/support/tickets/00000000-0000-0000-0000-000000000111", strings.NewReader(`{"status":"resolved"}`)), "00000000-0000-0000-0000-000000000111")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.handleAdminUpdateSupportTicket(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.Equal(t, "00000000-0000-0000-0000-000000000111", svc.lastUpdateTicketID)
		testutil.True(t, svc.lastUpdate.Status != nil)
	})

	t.Run("not found", func(t *testing.T) {
		svc := &fakeSupportService{updateTicketFn: func(ctx context.Context, ticketID string, updates support.TicketUpdate) (*support.Ticket, error) {
			return nil, support.ErrTicketNotFound
		}}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodPut, "/api/admin/support/tickets/00000000-0000-0000-0000-000000000111", strings.NewReader(`{"status":"resolved"}`)), "00000000-0000-0000-0000-000000000111")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.handleAdminUpdateSupportTicket(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid update", func(t *testing.T) {
		svc := &fakeSupportService{updateTicketFn: func(ctx context.Context, ticketID string, updates support.TicketUpdate) (*support.Ticket, error) {
			return nil, support.ErrNoTicketUpdates
		}}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodPut, "/api/admin/support/tickets/00000000-0000-0000-0000-000000000111", strings.NewReader(`{}`)), "00000000-0000-0000-0000-000000000111")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.handleAdminUpdateSupportTicket(w, req)
		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleAdminAddSupportMessage(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		svc := &fakeSupportService{}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodPost, "/api/admin/support/tickets/00000000-0000-0000-0000-000000000111/messages", strings.NewReader(`{"body":"admin reply"}`)), "00000000-0000-0000-0000-000000000111")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.handleAdminAddSupportMessage(w, req)
		testutil.Equal(t, http.StatusCreated, w.Code)
		testutil.Equal(t, support.SenderSupport, svc.lastAddSender)
	})

	t.Run("not found", func(t *testing.T) {
		svc := &fakeSupportService{addMessageFn: func(ctx context.Context, ticketID, senderType, body string) (*support.Message, error) {
			return nil, support.ErrTicketNotFound
		}}
		srv := supportTestServer(svc)
		req := withRouteID(httptest.NewRequest(http.MethodPost, "/api/admin/support/tickets/00000000-0000-0000-0000-000000000111/messages", strings.NewReader(`{"body":"admin reply"}`)), "00000000-0000-0000-0000-000000000111")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.handleAdminAddSupportMessage(w, req)
		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}
