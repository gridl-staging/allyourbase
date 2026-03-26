package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/notifications"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type fakeNotificationAdmin struct {
	items map[string]*notifications.Notification

	listItems   []*notifications.Notification
	listTotal   int
	listErr     error
	createErr   error
	markReadErr error
	markAllN    int64
	markAllErr  error

	lastListUser    string
	lastListUnread  bool
	lastListPage    int
	lastListPerPage int
	lastCreate      *notifications.Notification
	lastCreateMeta  map[string]any
	lastMarkReadID  string
	lastMarkReadUID string
}

func newFakeNotificationAdmin() *fakeNotificationAdmin {
	return &fakeNotificationAdmin{items: make(map[string]*notifications.Notification)}
}

func (f *fakeNotificationAdmin) Create(ctx context.Context, userID, title, body string, metadata map[string]any, channel string) (*notifications.Notification, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	if channel == "" {
		channel = "general"
	}
	n := &notifications.Notification{
		ID:        "notif-created",
		UserID:    userID,
		Title:     title,
		Body:      body,
		Metadata:  metadata,
		Channel:   channel,
		CreatedAt: time.Now().UTC(),
	}
	f.lastCreate = n
	f.lastCreateMeta = metadata
	f.items[n.ID] = n
	return n, nil
}

func (f *fakeNotificationAdmin) ListByUser(ctx context.Context, userID string, unreadOnly bool, page, perPage int) ([]*notifications.Notification, int, error) {
	f.lastListUser = userID
	f.lastListUnread = unreadOnly
	f.lastListPage = page
	f.lastListPerPage = perPage
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	if f.listItems != nil {
		return f.listItems, f.listTotal, nil
	}
	return nil, 0, nil
}

func (f *fakeNotificationAdmin) GetByID(ctx context.Context, id, userID string) (*notifications.Notification, error) {
	n, ok := f.items[id]
	if !ok || n.UserID != userID {
		return nil, notifications.ErrNotFound
	}
	return n, nil
}

func (f *fakeNotificationAdmin) MarkRead(ctx context.Context, id, userID string) error {
	f.lastMarkReadID = id
	f.lastMarkReadUID = userID
	if f.markReadErr != nil {
		return f.markReadErr
	}
	n, ok := f.items[id]
	if !ok || n.UserID != userID {
		return notifications.ErrNotFound
	}
	now := time.Now().UTC()
	n.ReadAt = &now
	return nil
}

func (f *fakeNotificationAdmin) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	if f.markAllErr != nil {
		return 0, f.markAllErr
	}
	return f.markAllN, nil
}

func notifClaims(userID string) *auth.Claims {
	return &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: userID},
		Email:            userID + "@example.com",
	}
}

func notifRequest(method, path, body string, claims *auth.Claims) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if claims != nil {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	}
	return req
}

func notifServerForHandlerTests() *Server {
	return &Server{
		cfg:    config.Default(),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		hub:    realtime.NewHub(testutil.DiscardLogger()),
	}
}

func TestNotificationsList_NoAuth(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()
	srv.SetNotificationService(newFakeNotificationAdmin())

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodGet, "/api/notifications", "", nil)
	srv.handleNotificationsList(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestNotificationsList_Success(t *testing.T) {
	t.Parallel()
	svc := newFakeNotificationAdmin()
	svc.listItems = []*notifications.Notification{{ID: "n1", UserID: "user-1", Title: "T", Channel: "general", CreatedAt: time.Now().UTC()}}
	svc.listTotal = 1

	srv := notifServerForHandlerTests()
	srv.SetNotificationService(svc)

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodGet, "/api/notifications?page=2&perPage=5", "", notifClaims("user-1"))
	srv.handleNotificationsList(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "user-1", svc.lastListUser)
	testutil.Equal(t, 2, svc.lastListPage)
	testutil.Equal(t, 5, svc.lastListPerPage)

	var body struct {
		Page       int                           `json:"page"`
		PerPage    int                           `json:"perPage"`
		TotalItems int                           `json:"totalItems"`
		TotalPages int                           `json:"totalPages"`
		Items      []*notifications.Notification `json:"items"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.Equal(t, 1, body.TotalItems)
	testutil.Equal(t, 1, len(body.Items))
}

func TestNotificationsList_UnreadFilter(t *testing.T) {
	t.Parallel()
	svc := newFakeNotificationAdmin()
	srv := notifServerForHandlerTests()
	srv.SetNotificationService(svc)

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodGet, "/api/notifications?unread=true", "", notifClaims("user-1"))
	srv.handleNotificationsList(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.True(t, svc.lastListUnread, "unread filter should be true")
}

func TestNotificationsCreate_NoAdminAuth(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cache := schema.NewCacheHolder(nil, logger)
	srv := New(cfg, logger, cache, nil, nil, nil)
	srv.SetNotificationService(newFakeNotificationAdmin())

	req := httptest.NewRequest(http.MethodPost, "/api/admin/notifications", strings.NewReader(`{"user_id":"u1","title":"t"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	testutil.True(t, rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden,
		"expected unauthorized/forbidden, got %d", rec.Code)
}

func TestNotificationsCreate_MissingUserID(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()
	srv.SetNotificationService(newFakeNotificationAdmin())

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodPost, "/api/admin/notifications", `{"title":"hello"}`, nil)
	srv.handleNotificationsCreate(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestNotificationsCreate_MissingTitle(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()
	srv.SetNotificationService(newFakeNotificationAdmin())

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodPost, "/api/admin/notifications", `{"user_id":"u1"}`, nil)
	srv.handleNotificationsCreate(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestNotificationsCreate_Success(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()
	svc := newFakeNotificationAdmin()
	srv.SetNotificationService(svc)

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodPost, "/api/admin/notifications", `{"user_id":"u1","title":"T","body":"B","metadata":{"k":"v"},"channel":"system"}`, nil)
	srv.handleNotificationsCreate(w, req)
	testutil.Equal(t, http.StatusCreated, w.Code)

	var got notifications.Notification
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	testutil.Equal(t, "u1", got.UserID)
	testutil.Equal(t, "T", got.Title)
}

func TestNotificationMarkRead_NotFound(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()
	svc := newFakeNotificationAdmin()
	svc.markReadErr = notifications.ErrNotFound
	srv.SetNotificationService(svc)

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodPost, "/api/notifications/missing/read", "", notifClaims("user-1"))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "missing")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	srv.handleNotificationMarkRead(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestNotificationMarkRead_Success(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()
	svc := newFakeNotificationAdmin()
	svc.items["n1"] = &notifications.Notification{ID: "n1", UserID: "user-1", Title: "x", CreatedAt: time.Now().UTC()}
	srv.SetNotificationService(svc)

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodPost, "/api/notifications/n1/read", "", notifClaims("user-1"))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "n1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	srv.handleNotificationMarkRead(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestNotificationMarkRead_WrongUser(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()
	svc := newFakeNotificationAdmin()
	svc.items["n1"] = &notifications.Notification{ID: "n1", UserID: "user-2", Title: "x", CreatedAt: time.Now().UTC()}
	srv.SetNotificationService(svc)

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodPost, "/api/notifications/n1/read", "", notifClaims("user-1"))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "n1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	srv.handleNotificationMarkRead(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestNotificationMarkAllRead_Success(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()
	svc := newFakeNotificationAdmin()
	svc.markAllN = 7
	srv.SetNotificationService(svc)

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodPost, "/api/notifications/read-all", "", notifClaims("user-1"))
	srv.handleNotificationMarkAllRead(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "\"updated\":7")
}

func TestNotificationsCreate_PublishesRealtimeEvent(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()
	svc := newFakeNotificationAdmin()
	srv.SetNotificationService(svc)
	client := srv.hub.Subscribe(map[string]bool{"_ayb_notifications": true})
	defer srv.hub.Unsubscribe(client.ID)

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodPost, "/api/admin/notifications", `{"user_id":"u1","title":"T","metadata":{"k":"v"}}`, nil)
	srv.handleNotificationsCreate(w, req)
	testutil.Equal(t, http.StatusCreated, w.Code)

	select {
	case ev := <-client.Events():
		testutil.Equal(t, "create", ev.Action)
		testutil.Equal(t, "_ayb_notifications", ev.Table)
		testutil.Equal(t, "u1", fmt.Sprint(ev.Record["user_id"]))
	case <-time.After(1 * time.Second):
		t.Fatal("expected realtime notification event")
	}
}

func TestNotifications_ServiceNotConfigured(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()

	cases := []struct {
		name   string
		path   string
		h      func(http.ResponseWriter, *http.Request)
		claims *auth.Claims
		id     string
		body   string
	}{
		{name: "list", path: "/api/notifications", h: srv.handleNotificationsList, claims: notifClaims("user-1")},
		{name: "create", path: "/api/admin/notifications", h: srv.handleNotificationsCreate, body: `{"user_id":"u1","title":"t"}`},
		{name: "mark-read", path: "/api/notifications/n1/read", h: srv.handleNotificationMarkRead, claims: notifClaims("user-1"), id: "n1"},
		{name: "mark-all", path: "/api/notifications/read-all", h: srv.handleNotificationMarkAllRead, claims: notifClaims("user-1")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := notifRequest(http.MethodPost, tc.path, tc.body, tc.claims)
			if tc.name == "list" {
				req = notifRequest(http.MethodGet, tc.path, tc.body, tc.claims)
			}
			if tc.id != "" {
				rctx := chi.NewRouteContext()
				rctx.URLParams.Add("id", tc.id)
				req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			}
			tc.h(w, req)
			testutil.Equal(t, http.StatusNotImplemented, w.Code)
		})
	}
}

func TestNotificationsList_InternalError(t *testing.T) {
	t.Parallel()
	srv := notifServerForHandlerTests()
	svc := newFakeNotificationAdmin()
	svc.listErr = errors.New("boom")
	srv.SetNotificationService(svc)

	w := httptest.NewRecorder()
	req := notifRequest(http.MethodGet, "/api/notifications", "", notifClaims("user-1"))
	srv.handleNotificationsList(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}
