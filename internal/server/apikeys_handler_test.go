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

// fakeAPIKeyManager is an in-memory fake for testing API key admin handlers.
type fakeAPIKeyManager struct {
	keys      []auth.APIKey
	listErr   error
	revErr    error
	createErr error
	created   []string // track created key names
}

func (f *fakeAPIKeyManager) ListAllAPIKeys(_ context.Context, page, perPage int) (*auth.APIKeyListResult, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	total := len(f.keys)
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}

	items := f.keys[start:end]
	if items == nil {
		items = []auth.APIKey{}
	}

	totalPages := total / perPage
	if total%perPage != 0 {
		totalPages++
	}

	return &auth.APIKeyListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
	}, nil
}

func (f *fakeAPIKeyManager) AdminRevokeAPIKey(_ context.Context, keyID string) error {
	if f.revErr != nil {
		return f.revErr
	}
	for i, k := range f.keys {
		if k.ID == keyID && k.RevokedAt == nil {
			now := time.Now()
			f.keys[i].RevokedAt = &now
			return nil
		}
	}
	return auth.ErrAPIKeyNotFound
}

func (f *fakeAPIKeyManager) CreateAPIKey(_ context.Context, userID, name string, opts ...auth.CreateAPIKeyOptions) (string, *auth.APIKey, error) {
	if f.createErr != nil {
		return "", nil, f.createErr
	}
	f.created = append(f.created, name)

	scope := "*"
	var allowedTables []string
	var appID *string
	if len(opts) > 0 {
		if opts[0].Scope != "" {
			scope = opts[0].Scope
		}
		if !auth.ValidScopes[scope] {
			return "", nil, auth.ErrInvalidScope
		}
		allowedTables = opts[0].AllowedTables
		appID = opts[0].AppID
	}
	if allowedTables == nil {
		allowedTables = []string{}
	}

	key := auth.APIKey{
		ID:            "key-new",
		UserID:        userID,
		Name:          name,
		KeyPrefix:     "ayb_abcd1234",
		Scope:         scope,
		AllowedTables: allowedTables,
		AppID:         appID,
		CreatedAt:     time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC),
	}
	return "ayb_aabbccdd11223344aabbccdd11223344aabbccdd11223344", &key, nil
}

func sampleAPIKeys() []auth.APIKey {
	now := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	return []auth.APIKey{
		{ID: "00000000-0000-0000-0000-000000000001", UserID: "00000000-0000-0000-0000-000000000011", Name: "CI/CD", KeyPrefix: "ayb_abc12345", Scope: "*", AllowedTables: []string{}, CreatedAt: now},
		{ID: "00000000-0000-0000-0000-000000000002", UserID: "00000000-0000-0000-0000-000000000012", Name: "Backend", KeyPrefix: "ayb_def67890", Scope: "readwrite", AllowedTables: []string{"posts", "comments"}, CreatedAt: now},
		{ID: "00000000-0000-0000-0000-000000000003", UserID: "00000000-0000-0000-0000-000000000013", Name: "Cron", KeyPrefix: "ayb_ghi11111", Scope: "readonly", AllowedTables: []string{}, CreatedAt: now},
	}
}

// --- List API keys tests ---

func TestAdminListAPIKeysSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminListAPIKeys(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.APIKeyListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, result.TotalItems)
	testutil.Equal(t, 3, len(result.Items))
	testutil.Equal(t, "CI/CD", result.Items[0].Name)
}

func TestAdminListAPIKeysWithPagination(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminListAPIKeys(mgr)

	// Page 1: should return first 2 items
	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys?page=1&perPage=2", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.APIKeyListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, result.TotalItems)
	testutil.Equal(t, 2, len(result.Items))
	testutil.Equal(t, 2, result.TotalPages)
	testutil.Equal(t, "CI/CD", result.Items[0].Name)
	testutil.Equal(t, "Backend", result.Items[1].Name)

	// Page 2: should return the remaining 1 item
	req2 := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys?page=2&perPage=2", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	var result2 auth.APIKeyListResult
	err2 := json.NewDecoder(w2.Body).Decode(&result2)
	testutil.NoError(t, err2)
	testutil.Equal(t, 1, len(result2.Items))
	testutil.Equal(t, "Cron", result2.Items[0].Name)
	testutil.Equal(t, 2, result2.Page)
}

func TestAdminListAPIKeysEmpty(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: nil}
	handler := handleAdminListAPIKeys(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.APIKeyListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, result.TotalItems)
	testutil.Equal(t, 0, len(result.Items))
}

func TestAdminListAPIKeysServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{listErr: fmt.Errorf("db down")}
	handler := handleAdminListAPIKeys(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to list api keys")
}

// --- Revoke API key tests ---

func TestAdminRevokeAPIKeySuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminRevokeAPIKey(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/api-keys/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/api-keys/00000000-0000-0000-0000-000000000002", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	// Verify the targeted key was revoked
	testutil.NotNil(t, mgr.keys[1].RevokedAt)
	// Verify other keys were NOT affected
	testutil.Nil(t, mgr.keys[0].RevokedAt)
	testutil.Nil(t, mgr.keys[2].RevokedAt)
}

func TestAdminRevokeAPIKeyNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminRevokeAPIKey(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/api-keys/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/api-keys/00000000-0000-0000-0000-000000000099", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "api key not found")
}

func TestAdminRevokeAPIKeyInvalidUUID(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminRevokeAPIKey(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/api-keys/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/api-keys/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid api key id format")
}

func TestAdminRevokeAPIKeyNoID(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminRevokeAPIKey(mgr)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/api-keys/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "api key id is required")
}

func TestAdminRevokeAPIKeyServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{
		keys:   sampleAPIKeys(),
		revErr: fmt.Errorf("constraint violation"),
	}
	handler := handleAdminRevokeAPIKey(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/api-keys/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/api-keys/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to revoke api key")
}

// --- Create API key tests ---

func TestAdminCreateAPIKeySuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"Deploy Key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp adminCreateAPIKeyResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	testutil.NoError(t, err)
	testutil.True(t, strings.HasPrefix(resp.Key, "ayb_"), "key should start with ayb_ prefix")
	// Key should be exactly 52 chars: "ayb_" (4) + 48 hex chars
	testutil.Equal(t, 52, len(resp.Key))
	testutil.Equal(t, "Deploy Key", resp.APIKey.Name)
	testutil.Equal(t, "00000000-0000-0000-0000-000000000011", resp.APIKey.UserID)
	testutil.Equal(t, "key-new", resp.APIKey.ID)
	testutil.True(t, strings.HasPrefix(resp.APIKey.KeyPrefix, "ayb_"), "prefix should start with ayb_")
	testutil.Equal(t, 1, len(mgr.created))
	testutil.Equal(t, "Deploy Key", mgr.created[0])
}

func TestAdminCreateAPIKeyMissingUserID(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"name":"Deploy Key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "userId is required")
}

func TestAdminCreateAPIKeyInvalidUserIDFormat(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"not-a-uuid","name":"Bad User Key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid userId format")
	testutil.Equal(t, 0, len(mgr.created))
}

func TestAdminCreateAPIKeyMissingName(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"00000000-0000-0000-0000-000000000011"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "name is required")
}

func TestAdminCreateAPIKeyServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{createErr: fmt.Errorf("db connection failed")}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"Test Key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to create api key")
}

func TestAdminCreateAPIKeyInvalidJSON(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{}
	handler := handleAdminCreateAPIKey(mgr)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	msg, ok := resp["message"].(string)
	testutil.True(t, ok, "response should contain a 'message' string field")
	testutil.Contains(t, msg, "invalid JSON")
}

func TestAdminCreateAPIKeyEmptyBody(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{}
	handler := handleAdminCreateAPIKey(mgr)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "userId is required")
}

func TestAdminRevokeAlreadyRevokedKey(t *testing.T) {
	t.Parallel()
	keys := sampleAPIKeys()
	now := time.Now()
	keys[0].RevokedAt = &now // mark k1 as already revoked
	mgr := &fakeAPIKeyManager{keys: keys}
	handler := handleAdminRevokeAPIKey(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/api-keys/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/api-keys/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Already revoked key should return 404 (not found with revoked_at IS NULL)
	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "api key not found")
}

func TestAdminListAPIKeysPaginationDefaults(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminListAPIKeys(mgr)

	// page=0 and perPage=0 should default to page=1, perPage=20
	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys?page=0&perPage=0", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.APIKeyListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, result.Page)
	testutil.Equal(t, 20, result.PerPage)
}

func TestAdminListAPIKeysPaginationClamp(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminListAPIKeys(mgr)

	// perPage > 100 should clamp to 100
	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys?page=1&perPage=500", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.APIKeyListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 100, result.PerPage)
}

func TestAdminListAPIKeysBeyondLastPage(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminListAPIKeys(mgr)

	// Request page well beyond total pages
	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys?page=999&perPage=20", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.APIKeyListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(result.Items))
	testutil.Equal(t, 3, result.TotalItems)
}

func TestAdminListAPIKeysNegativePage(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminListAPIKeys(mgr)

	// Negative page should default to page 1
	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys?page=-5&perPage=20", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.APIKeyListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, result.Page)
	testutil.Equal(t, 3, len(result.Items))
}

func TestAdminListAPIKeysNonNumericParams(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminListAPIKeys(mgr)

	// Non-numeric params should silently default (strconv.Atoi returns 0)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys?page=abc&perPage=xyz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.APIKeyListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, result.Page)
	testutil.Equal(t, 20, result.PerPage)
	testutil.Equal(t, 3, len(result.Items))
}

// --- Scoped API key tests ---

func TestAdminCreateAPIKeyWithReadonlyScope(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"Read Key","scope":"readonly"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp adminCreateAPIKeyResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	testutil.NoError(t, err)
	testutil.Equal(t, "readonly", resp.APIKey.Scope)
	testutil.Equal(t, 0, len(resp.APIKey.AllowedTables))
}

func TestAdminCreateAPIKeyWithReadwriteScope(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"RW Key","scope":"readwrite"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp adminCreateAPIKeyResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	testutil.NoError(t, err)
	testutil.Equal(t, "readwrite", resp.APIKey.Scope)
}

func TestAdminCreateAPIKeyWithAllowedTables(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"Table Key","scope":"readwrite","allowedTables":["posts","comments"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp adminCreateAPIKeyResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	testutil.NoError(t, err)
	testutil.Equal(t, "readwrite", resp.APIKey.Scope)
	testutil.Equal(t, 2, len(resp.APIKey.AllowedTables))
	testutil.Equal(t, "posts", resp.APIKey.AllowedTables[0])
	testutil.Equal(t, "comments", resp.APIKey.AllowedTables[1])
}

func TestAdminCreateAPIKeyInvalidScope(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"Bad Key","scope":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid scope")
}

func TestAdminCreateAPIKeyDefaultScope(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminCreateAPIKey(mgr)

	// No scope specified — should default to "*"
	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"Default Key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp adminCreateAPIKeyResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	testutil.NoError(t, err)
	testutil.Equal(t, "*", resp.APIKey.Scope)
	testutil.Equal(t, 0, len(resp.APIKey.AllowedTables))
}

func TestAdminCreateAPIKeyWithAppID(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"App Key","appId":"00000000-0000-0000-0000-000000000001"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp adminCreateAPIKeyResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	testutil.NoError(t, err)
	testutil.Equal(t, "App Key", resp.APIKey.Name)
	testutil.NotNil(t, resp.APIKey.AppID)
	testutil.Equal(t, "00000000-0000-0000-0000-000000000001", *resp.APIKey.AppID)
}

func TestAdminCreateAPIKeyWithoutAppID(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminCreateAPIKey(mgr)

	// No appId field — should create a legacy user-scoped key.
	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"Legacy Key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp adminCreateAPIKeyResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	testutil.NoError(t, err)
	testutil.Equal(t, "Legacy Key", resp.APIKey.Name)
	testutil.Nil(t, resp.APIKey.AppID)
}

func TestAdminCreateAPIKeyInvalidAppID(t *testing.T) {
	t.Parallel()
	// Use a valid UUID format that doesn't exist — service returns ErrInvalidAppID.
	mgr := &fakeAPIKeyManager{createErr: auth.ErrInvalidAppID}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"Bad App Key","appId":"00000000-0000-0000-0000-aaaaaaaaa099"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "app not found")
}

func TestAdminCreateAPIKeyNonUUIDAppID(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminCreateAPIKey(mgr)

	// Non-UUID appId should be rejected at the handler level with 400, not reach the DB.
	body := `{"userId":"00000000-0000-0000-0000-000000000011","name":"Bad Key","appId":"not-a-uuid"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid appId format")
	// Verify the service was never called (no created keys).
	testutil.Equal(t, 0, len(mgr.created))
}

func TestAdminCreateAPIKeyUserNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{createErr: auth.ErrUserNotFound}
	handler := handleAdminCreateAPIKey(mgr)

	body := `{"userId":"00000000-0000-0000-0000-000000000099","name":"Missing User Key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "user not found")
}

func TestAdminListAPIKeysShowsScope(t *testing.T) {
	t.Parallel()
	mgr := &fakeAPIKeyManager{keys: sampleAPIKeys()}
	handler := handleAdminListAPIKeys(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.APIKeyListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, "*", result.Items[0].Scope)
	testutil.Equal(t, "readwrite", result.Items[1].Scope)
	testutil.Equal(t, 2, len(result.Items[1].AllowedTables))
	testutil.Equal(t, "readonly", result.Items[2].Scope)
}
