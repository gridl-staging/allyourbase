package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/vault"
)

type fakeVaultSecretStore struct {
	values map[string][]byte
	meta   map[string]vault.SecretMetadata
}

func newFakeVaultSecretStore() *fakeVaultSecretStore {
	return &fakeVaultSecretStore{
		values: map[string][]byte{},
		meta:   map[string]vault.SecretMetadata{},
	}
}

func (f *fakeVaultSecretStore) ListSecrets(_ context.Context) ([]vault.SecretMetadata, error) {
	out := make([]vault.SecretMetadata, 0, len(f.meta))
	for _, s := range f.meta {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeVaultSecretStore) CreateSecret(_ context.Context, name string, value []byte) error {
	if _, ok := f.values[name]; ok {
		return vault.ErrSecretAlreadyExists
	}
	now := time.Now().UTC()
	f.values[name] = append([]byte(nil), value...)
	f.meta[name] = vault.SecretMetadata{
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return nil
}

func (f *fakeVaultSecretStore) UpdateSecret(_ context.Context, name string, value []byte) error {
	m, ok := f.meta[name]
	if !ok {
		return vault.ErrSecretNotFound
	}
	f.values[name] = append([]byte(nil), value...)
	m.UpdatedAt = time.Now().UTC()
	f.meta[name] = m
	return nil
}

func (f *fakeVaultSecretStore) GetSecret(_ context.Context, name string) ([]byte, error) {
	v, ok := f.values[name]
	if !ok {
		return nil, vault.ErrSecretNotFound
	}
	return append([]byte(nil), v...), nil
}

func (f *fakeVaultSecretStore) DeleteSecret(_ context.Context, name string) error {
	if _, ok := f.values[name]; !ok {
		return vault.ErrSecretNotFound
	}
	delete(f.values, name)
	delete(f.meta, name)
	return nil
}

func secretsAdminToken(t *testing.T, srv *Server, password string) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.True(t, body["token"] != "", "expected non-empty admin token")
	return body["token"]
}

func newSecretsServer(t *testing.T) (*Server, *fakeVaultSecretStore, string) {
	t.Helper()
	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := New(cfg, logger, ch, nil, nil, nil)
	store := newFakeVaultSecretStore()
	srv.SetVaultStore(store)
	token := secretsAdminToken(t, srv, "testpass")
	return srv, store, token
}

func TestSecretsCRUDLifecycle(t *testing.T) {
	t.Parallel()
	srv, store, token := newSecretsServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/secrets", strings.NewReader(`{"name":"API_KEY","value":"super-secret-value"}`))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	srv.Router().ServeHTTP(createW, createReq)
	testutil.Equal(t, http.StatusCreated, createW.Code)

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/secrets", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listW := httptest.NewRecorder()
	srv.Router().ServeHTTP(listW, listReq)
	testutil.Equal(t, http.StatusOK, listW.Code)
	testutil.False(t, strings.Contains(listW.Body.String(), "super-secret-value"), "list response must never contain secret values")

	var listed []vault.SecretMetadata
	testutil.NoError(t, json.Unmarshal(listW.Body.Bytes(), &listed))
	testutil.Equal(t, 1, len(listed))
	testutil.Equal(t, "API_KEY", listed[0].Name)
	testutil.True(t, !listed[0].CreatedAt.IsZero(), "created_at should be populated")
	testutil.True(t, !listed[0].UpdatedAt.IsZero(), "updated_at should be populated")

	updateReq := httptest.NewRequest(http.MethodPut, "/api/admin/secrets/API_KEY", strings.NewReader(`{"value":"updated-secret"}`))
	updateReq.Header.Set("Authorization", "Bearer "+token)
	updateReq.Header.Set("Content-Type", "application/json")
	updateW := httptest.NewRecorder()
	srv.Router().ServeHTTP(updateW, updateReq)
	testutil.Equal(t, http.StatusOK, updateW.Code)
	testutil.Equal(t, "updated-secret", string(store.values["API_KEY"]))

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/admin/secrets/API_KEY", nil)
	deleteReq.Header.Set("Authorization", "Bearer "+token)
	deleteW := httptest.NewRecorder()
	srv.Router().ServeHTTP(deleteW, deleteReq)
	testutil.Equal(t, http.StatusNoContent, deleteW.Code)

	listReq2 := httptest.NewRequest(http.MethodGet, "/api/admin/secrets", nil)
	listReq2.Header.Set("Authorization", "Bearer "+token)
	listW2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(listW2, listReq2)
	testutil.Equal(t, http.StatusOK, listW2.Code)
	testutil.NoError(t, json.Unmarshal(listW2.Body.Bytes(), &listed))
	testutil.Equal(t, 0, len(listed))
}

func TestSecretsHandlersRequireAdminToken(t *testing.T) {
	t.Parallel()
	srv, _, _ := newSecretsServer(t)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/admin/secrets"},
		{method: http.MethodGet, path: "/api/admin/secrets/N"},
		{method: http.MethodPost, path: "/api/admin/secrets", body: `{"name":"N","value":"V"}`},
		{method: http.MethodPut, path: "/api/admin/secrets/N", body: `{"value":"V2"}`},
		{method: http.MethodDelete, path: "/api/admin/secrets/N"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusUnauthorized, w.Code)
		testutil.Contains(t, w.Body.String(), "admin authentication required")
	}
}

func TestCreateSecretDuplicateReturnsConflict(t *testing.T) {
	t.Parallel()
	srv, _, token := newSecretsServer(t)

	createReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/secrets", strings.NewReader(`{"name":"DUPLICATE","value":"v"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	w1 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w1, createReq())
	testutil.Equal(t, http.StatusCreated, w1.Code)

	w2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w2, createReq())
	testutil.Equal(t, http.StatusConflict, w2.Code)
}

func TestUpdateDeleteSecretMissingReturnsNotFound(t *testing.T) {
	t.Parallel()
	srv, _, token := newSecretsServer(t)

	updateReq := httptest.NewRequest(http.MethodPut, "/api/admin/secrets/MISSING", strings.NewReader(`{"value":"new"}`))
	updateReq.Header.Set("Authorization", "Bearer "+token)
	updateReq.Header.Set("Content-Type", "application/json")
	updateW := httptest.NewRecorder()
	srv.Router().ServeHTTP(updateW, updateReq)
	testutil.Equal(t, http.StatusNotFound, updateW.Code)

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/admin/secrets/MISSING", nil)
	deleteReq.Header.Set("Authorization", "Bearer "+token)
	deleteW := httptest.NewRecorder()
	srv.Router().ServeHTTP(deleteW, deleteReq)
	testutil.Equal(t, http.StatusNotFound, deleteW.Code)
}

func TestGetSecretReturnsValue(t *testing.T) {
	t.Parallel()
	srv, _, token := newSecretsServer(t)

	// Create a secret first.
	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/secrets", strings.NewReader(`{"name":"DB_PASS","value":"hunter2"}`))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	srv.Router().ServeHTTP(createW, createReq)
	testutil.Equal(t, http.StatusCreated, createW.Code)

	// Get the secret.
	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/secrets/DB_PASS", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getW := httptest.NewRecorder()
	srv.Router().ServeHTTP(getW, getReq)
	testutil.Equal(t, http.StatusOK, getW.Code)

	var body map[string]string
	testutil.NoError(t, json.Unmarshal(getW.Body.Bytes(), &body))
	testutil.Equal(t, "DB_PASS", body["name"])
	testutil.Equal(t, "hunter2", body["value"])
}

func TestGetSecretNotFoundReturns404(t *testing.T) {
	t.Parallel()
	srv, _, token := newSecretsServer(t)

	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/secrets/MISSING", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getW := httptest.NewRecorder()
	srv.Router().ServeHTTP(getW, getReq)
	testutil.Equal(t, http.StatusNotFound, getW.Code)
}

func TestListSecretsNeverContainsValues(t *testing.T) {
	t.Parallel()
	srv, _, token := newSecretsServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/secrets", strings.NewReader(`{"name":"NO_LEAK","value":"sensitive-never-list"}`))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	srv.Router().ServeHTTP(createW, createReq)
	testutil.Equal(t, http.StatusCreated, createW.Code)

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/secrets", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listW := httptest.NewRecorder()
	srv.Router().ServeHTTP(listW, listReq)
	testutil.Equal(t, http.StatusOK, listW.Code)
	testutil.False(t, strings.Contains(listW.Body.String(), "sensitive-never-list"), "list response must never include secret values")
}

func TestSecretsHandlersReturnUnavailableWithoutVault(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := New(cfg, logger, ch, nil, nil, nil)
	// Deliberately do NOT call srv.SetVaultStore — vaultStore remains nil.
	token := secretsAdminToken(t, srv, "testpass")

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/admin/secrets", ""},
		{http.MethodGet, "/api/admin/secrets/FOO", ""},
		{http.MethodPost, "/api/admin/secrets", `{"name":"FOO","value":"v"}`},
		{http.MethodPut, "/api/admin/secrets/FOO", `{"value":"v2"}`},
		{http.MethodDelete, "/api/admin/secrets/FOO", ""},
	}
	for _, tc := range endpoints {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+token)
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	}
}

func TestSecretsHandlersRejectInvalidName(t *testing.T) {
	t.Parallel()
	srv, store, token := newSecretsServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/secrets", strings.NewReader(`{"name":"a/b","value":"v"}`))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	srv.Router().ServeHTTP(createW, createReq)
	testutil.Equal(t, http.StatusBadRequest, createW.Code)
	testutil.Equal(t, 0, len(store.values))

	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/secrets/..", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getW := httptest.NewRecorder()
	srv.Router().ServeHTTP(getW, getReq)
	testutil.Equal(t, http.StatusBadRequest, getW.Code)

	updateReq := httptest.NewRequest(http.MethodPut, "/api/admin/secrets/..", strings.NewReader(`{"value":"v2"}`))
	updateReq.Header.Set("Authorization", "Bearer "+token)
	updateReq.Header.Set("Content-Type", "application/json")
	updateW := httptest.NewRecorder()
	srv.Router().ServeHTTP(updateW, updateReq)
	testutil.Equal(t, http.StatusBadRequest, updateW.Code)

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/admin/secrets/..", nil)
	deleteReq.Header.Set("Authorization", "Bearer "+token)
	deleteW := httptest.NewRecorder()
	srv.Router().ServeHTTP(deleteW, deleteReq)
	testutil.Equal(t, http.StatusBadRequest, deleteW.Code)
}
