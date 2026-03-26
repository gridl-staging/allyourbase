package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

type quotaManagerMock struct {
	getUsageFn     func(ctx context.Context, userID string) (*storage.QuotaInfo, error)
	setUserQuotaFn func(ctx context.Context, userID string, quotaMB *int) error
}

func (m quotaManagerMock) GetUsage(ctx context.Context, userID string) (*storage.QuotaInfo, error) {
	if m.getUsageFn == nil {
		return nil, nil
	}
	return m.getUsageFn(ctx, userID)
}

func (m quotaManagerMock) SetUserQuota(ctx context.Context, userID string, quotaMB *int) error {
	if m.setUserQuotaFn == nil {
		return nil
	}
	return m.setUserQuotaFn(ctx, userID, quotaMB)
}

func withUserID(req *http.Request, userID string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", userID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestHandleAdminGetUserQuota_NotFound(t *testing.T) {
	t.Parallel()
	h := handleAdminGetUserQuota(quotaManagerMock{
		getUsageFn: func(context.Context, string) (*storage.QuotaInfo, error) {
			return nil, storage.ErrQuotaUserNotFound
		},
	})

	w := httptest.NewRecorder()
	req := withUserID(httptest.NewRequest(http.MethodGet, "/api/admin/users/u1/storage-quota", nil), "u1")
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminGetUserQuota_InternalError(t *testing.T) {
	t.Parallel()
	h := handleAdminGetUserQuota(quotaManagerMock{
		getUsageFn: func(context.Context, string) (*storage.QuotaInfo, error) {
			return nil, errors.New("db offline")
		},
	})

	w := httptest.NewRecorder()
	req := withUserID(httptest.NewRequest(http.MethodGet, "/api/admin/users/u1/storage-quota", nil), "u1")
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleAdminSetUserQuota_RejectsNegativeQuota(t *testing.T) {
	t.Parallel()
	called := false
	h := handleAdminSetUserQuota(quotaManagerMock{
		setUserQuotaFn: func(context.Context, string, *int) error {
			called = true
			return nil
		},
	})

	w := httptest.NewRecorder()
	req := withUserID(httptest.NewRequest(http.MethodPut, "/api/admin/users/u1/storage-quota", strings.NewReader(`{"quota_mb":-1}`)), "u1")
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.False(t, called, "SetUserQuota should not be called for invalid input")
}

func TestHandleAdminSetUserQuota_InternalError(t *testing.T) {
	t.Parallel()
	h := handleAdminSetUserQuota(quotaManagerMock{
		setUserQuotaFn: func(context.Context, string, *int) error {
			return errors.New("write failed")
		},
	})

	w := httptest.NewRecorder()
	req := withUserID(httptest.NewRequest(http.MethodPut, "/api/admin/users/u1/storage-quota", strings.NewReader(`{"quota_mb":5}`)), "u1")
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}
