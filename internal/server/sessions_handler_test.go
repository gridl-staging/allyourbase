package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

type fakeUserSessionManager struct {
	sessions map[string][]auth.SessionInfo
	revoked  map[string]map[string]bool
	err      error
}

func newFakeUserSessionManager() *fakeUserSessionManager {
	now := time.Date(2026, 2, 25, 18, 0, 0, 0, time.UTC)
	return &fakeUserSessionManager{
		sessions: map[string][]auth.SessionInfo{
			"00000000-0000-0000-0000-000000000021": {
				{
					ID:           "00000000-0000-0000-0000-0000000000a1",
					UserAgent:    "Browser/1.0",
					IPAddress:    "203.0.113.10",
					CreatedAt:    now,
					LastActiveAt: now,
				},
			},
		},
		revoked: make(map[string]map[string]bool),
	}
}

func (f *fakeUserSessionManager) ListSessions(_ context.Context, userID, _ string) ([]auth.SessionInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	s := f.sessions[userID]
	out := make([]auth.SessionInfo, 0, len(s))
	for _, session := range s {
		if f.revoked[userID][session.ID] {
			continue
		}
		out = append(out, session)
	}
	return out, nil
}

func (f *fakeUserSessionManager) RevokeSession(_ context.Context, userID, sessionID string) error {
	if f.err != nil {
		return f.err
	}
	for otherUserID, sessions := range f.sessions {
		for _, session := range sessions {
			if session.ID == sessionID {
				if otherUserID != userID {
					return auth.ErrSessionForbidden
				}
				if f.revoked[userID] == nil {
					f.revoked[userID] = make(map[string]bool)
				}
				f.revoked[userID][sessionID] = true
				return nil
			}
		}
	}
	return auth.ErrSessionNotFound
}

func (f *fakeUserSessionManager) RevokeAllSessions(_ context.Context, userID string) error {
	if f.err != nil {
		return f.err
	}
	if f.revoked[userID] == nil {
		f.revoked[userID] = make(map[string]bool)
	}
	for _, session := range f.sessions[userID] {
		f.revoked[userID][session.ID] = true
	}
	return nil
}

func TestAdminListUserSessionsSuccess(t *testing.T) {
	t.Parallel()
	mgr := newFakeUserSessionManager()
	handler := handleAdminListUserSessions(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/users/{id}/sessions", handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/00000000-0000-0000-0000-000000000021/sessions", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var sessions []auth.SessionInfo
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &sessions))
	testutil.Equal(t, 1, len(sessions))
	testutil.Equal(t, "00000000-0000-0000-0000-0000000000a1", sessions[0].ID)
}

func TestAdminListUserSessionsInvalidUserID(t *testing.T) {
	t.Parallel()
	mgr := newFakeUserSessionManager()
	handler := handleAdminListUserSessions(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/users/{id}/sessions", handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/not-a-uuid/sessions", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid user id format")
}

func TestAdminDeleteUserSessionSuccess(t *testing.T) {
	t.Parallel()
	mgr := newFakeUserSessionManager()
	handler := handleAdminDeleteUserSession(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/users/{id}/sessions/{session_id}", handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/00000000-0000-0000-0000-000000000021/sessions/00000000-0000-0000-0000-0000000000a1", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
}

func TestAdminDeleteUserSessionNotFound(t *testing.T) {
	t.Parallel()
	mgr := newFakeUserSessionManager()
	handler := handleAdminDeleteUserSession(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/users/{id}/sessions/{session_id}", handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/00000000-0000-0000-0000-000000000021/sessions/00000000-0000-0000-0000-0000000000ff", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "session not found")
}

func TestAdminDeleteUserSessionMissingSessionID(t *testing.T) {
	t.Parallel()
	mgr := newFakeUserSessionManager()
	handler := handleAdminDeleteUserSession(mgr)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/00000000-0000-0000-0000-000000000021/sessions/", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", "00000000-0000-0000-0000-000000000021")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid session id format")
}

func TestAdminDeleteAllUserSessionsSuccess(t *testing.T) {
	t.Parallel()
	mgr := newFakeUserSessionManager()
	handler := handleAdminDeleteAllUserSessions(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/users/{id}/sessions", handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/00000000-0000-0000-0000-000000000021/sessions", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)

	sessions, err := mgr.ListSessions(context.Background(), "00000000-0000-0000-0000-000000000021", "")
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(sessions))
}

func TestAdminSessionsHandlersServiceError(t *testing.T) {
	t.Parallel()
	mgr := newFakeUserSessionManager()
	mgr.err = fmt.Errorf("db unavailable")

	listHandler := handleAdminListUserSessions(mgr)
	revokeOneHandler := handleAdminDeleteUserSession(mgr)
	revokeAllHandler := handleAdminDeleteAllUserSessions(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/users/{id}/sessions", listHandler)
	r.Delete("/api/admin/users/{id}/sessions/{session_id}", revokeOneHandler)
	r.Delete("/api/admin/users/{id}/sessions", revokeAllHandler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/00000000-0000-0000-0000-000000000021/sessions", nil)
	r.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/users/00000000-0000-0000-0000-000000000021/sessions/00000000-0000-0000-0000-0000000000a1", nil)
	r.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/users/00000000-0000-0000-0000-000000000021/sessions", nil)
	r.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}
