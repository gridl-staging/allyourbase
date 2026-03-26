package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

type fakeUserProviderTokenManager struct {
	listByUser   map[string][]auth.ProviderTokenInfo
	listErr      error
	deleteErr    error
	deleteCalls  int
	lastUserID   string
	lastProvider string
}

func (f *fakeUserProviderTokenManager) ListProviderTokens(_ context.Context, userID string) ([]auth.ProviderTokenInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	items := f.listByUser[userID]
	out := make([]auth.ProviderTokenInfo, len(items))
	copy(out, items)
	return out, nil
}

func (f *fakeUserProviderTokenManager) DeleteProviderToken(_ context.Context, userID, provider string) error {
	f.deleteCalls++
	f.lastUserID = userID
	f.lastProvider = provider
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return nil
}

func TestAdminListUserProviderTokensSuccess(t *testing.T) {
	t.Parallel()
	userID := "00000000-0000-0000-0000-000000000021"
	now := time.Date(2026, 2, 25, 19, 0, 0, 0, time.UTC)
	mgr := &fakeUserProviderTokenManager{
		listByUser: map[string][]auth.ProviderTokenInfo{
			userID: {
				{
					Provider:            "google",
					TokenType:           "Bearer",
					Scopes:              "openid profile email",
					RefreshFailureCount: 1,
					CreatedAt:           now,
					UpdatedAt:           now,
				},
			},
		},
	}

	handler := handleAdminListUserProviderTokens(mgr)
	r := chi.NewRouter()
	r.Get("/api/admin/users/{id}/provider-tokens", handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/"+userID+"/provider-tokens", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var tokens []auth.ProviderTokenInfo
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &tokens))
	testutil.Equal(t, 1, len(tokens))
	testutil.Equal(t, "google", tokens[0].Provider)

	body := w.Body.String()
	testutil.True(t, !strings.Contains(body, "access_token"), "response must not include access_token")
	testutil.True(t, !strings.Contains(body, "refresh_token"), "response must not include refresh_token")
}

func TestAdminListUserProviderTokensInvalidUserID(t *testing.T) {
	t.Parallel()
	mgr := &fakeUserProviderTokenManager{}
	handler := handleAdminListUserProviderTokens(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/users/{id}/provider-tokens", handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/not-a-uuid/provider-tokens", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid user id format")
}

func TestAdminDeleteUserProviderTokenSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeUserProviderTokenManager{}
	handler := handleAdminDeleteUserProviderToken(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/users/{id}/provider-tokens/{provider}", handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/00000000-0000-0000-0000-000000000021/provider-tokens/google", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, 1, mgr.deleteCalls)
	testutil.Equal(t, "00000000-0000-0000-0000-000000000021", mgr.lastUserID)
	testutil.Equal(t, "google", mgr.lastProvider)
}

func TestAdminDeleteUserProviderTokenNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeUserProviderTokenManager{deleteErr: auth.ErrProviderTokenNotFound}
	handler := handleAdminDeleteUserProviderToken(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/users/{id}/provider-tokens/{provider}", handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/00000000-0000-0000-0000-000000000021/provider-tokens/google", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "provider token not found")
}

func TestAdminProviderTokenHandlersServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeUserProviderTokenManager{
		listErr:   fmt.Errorf("db down"),
		deleteErr: fmt.Errorf("db down"),
	}

	listHandler := handleAdminListUserProviderTokens(mgr)
	deleteHandler := handleAdminDeleteUserProviderToken(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/users/{id}/provider-tokens", listHandler)
	r.Delete("/api/admin/users/{id}/provider-tokens/{provider}", deleteHandler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/00000000-0000-0000-0000-000000000021/provider-tokens", nil)
	r.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/users/00000000-0000-0000-0000-000000000021/provider-tokens/google", nil)
	r.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}
