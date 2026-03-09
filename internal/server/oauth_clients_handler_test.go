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

// fakeOAuthClientManager is an in-memory fake implementing oauthClientManager.
type fakeOAuthClientManager struct {
	clients []auth.OAuthClient
	listErr error
	getErr       error
	registerErr  error
	revokeErr    error
	rotateErr    error
	updateErr    error
}

func (f *fakeOAuthClientManager) RegisterOAuthClient(_ context.Context, appID, name, clientType string, redirectURIs, scopes []string) (string, *auth.OAuthClient, error) {
	if f.registerErr != nil {
		return "", nil, f.registerErr
	}
	if err := auth.ValidateClientType(clientType); err != nil {
		return "", nil, err
	}
	if err := auth.ValidateRedirectURIs(redirectURIs); err != nil {
		return "", nil, err
	}
	if err := auth.ValidateOAuthScopes(scopes); err != nil {
		return "", nil, err
	}

	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	client := auth.OAuthClient{
		ID:           "00000000-0000-0000-0000-000000000099",
		AppID:        appID,
		ClientID:     "ayb_cid_aabbccdd11223344aabbccdd11223344aabbccdd11223344",
		Name:         name,
		RedirectURIs: redirectURIs,
		Scopes:       scopes,
		ClientType:   clientType,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	f.clients = append(f.clients, client)

	secret := ""
	if clientType == auth.OAuthClientTypeConfidential {
		secret = "ayb_cs_0011223344556677889900aabbccddeeff0011223344556677889900aabbccdd"
	}
	return secret, &client, nil
}

func (f *fakeOAuthClientManager) GetOAuthClient(_ context.Context, clientID string) (*auth.OAuthClient, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for _, c := range f.clients {
		if c.ClientID == clientID {
			return &c, nil
		}
	}
	return nil, auth.ErrOAuthClientNotFound
}

func (f *fakeOAuthClientManager) GetOAuthClientByUUID(_ context.Context, id string) (*auth.OAuthClient, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for _, c := range f.clients {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, auth.ErrOAuthClientNotFound
}

func (f *fakeOAuthClientManager) ListOAuthClients(_ context.Context, page, perPage int) (*auth.OAuthClientListResult, error) {
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

	total := len(f.clients)
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}

	items := f.clients[start:end]
	if items == nil {
		items = []auth.OAuthClient{}
	}

	totalPages := total / perPage
	if total%perPage != 0 {
		totalPages++
	}

	return &auth.OAuthClientListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
	}, nil
}

func (f *fakeOAuthClientManager) RevokeOAuthClient(_ context.Context, clientID string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	for i, c := range f.clients {
		if c.ClientID == clientID && c.RevokedAt == nil {
			now := time.Now()
			f.clients[i].RevokedAt = &now
			return nil
		}
	}
	return auth.ErrOAuthClientNotFound
}

func (f *fakeOAuthClientManager) RegenerateOAuthClientSecret(_ context.Context, clientID string) (string, error) {
	if f.rotateErr != nil {
		return "", f.rotateErr
	}
	for i, c := range f.clients {
		if c.ClientID == clientID {
			if c.RevokedAt != nil {
				return "", auth.ErrOAuthClientRevoked
			}
			if c.ClientType != auth.OAuthClientTypeConfidential {
				return "", auth.ErrOAuthClientPublicSecretRotator
			}
			f.clients[i].UpdatedAt = time.Now()
			return "ayb_cs_newsecretnewsecretnewsecretnewsecretnewsecretnewsecretnewsecret", nil
		}
	}
	return "", auth.ErrOAuthClientNotFound
}

func (f *fakeOAuthClientManager) UpdateOAuthClient(_ context.Context, clientID, name string, redirectURIs, scopes []string) (*auth.OAuthClient, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	for i, c := range f.clients {
		if c.ClientID == clientID {
			if c.RevokedAt != nil {
				return nil, auth.ErrOAuthClientRevoked
			}
			if name != "" {
				f.clients[i].Name = name
			}
			if redirectURIs != nil {
				f.clients[i].RedirectURIs = redirectURIs
			}
			if scopes != nil {
				f.clients[i].Scopes = scopes
			}
			f.clients[i].UpdatedAt = time.Now()
			return &f.clients[i], nil
		}
	}
	return nil, auth.ErrOAuthClientNotFound
}

func sampleOAuthClients() []auth.OAuthClient {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	return []auth.OAuthClient{
		{
			ID:           "00000000-0000-0000-0000-000000000001",
			AppID:        "00000000-0000-0000-0000-0000000000a1",
			ClientID:     "ayb_cid_aaaa11112222333344445555aaaa11112222333344445555",
			Name:         "Mobile App",
			RedirectURIs: []string{"https://mobile.example.com/callback"},
			Scopes:       []string{"readwrite"},
			ClientType:   "confidential",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "00000000-0000-0000-0000-000000000002",
			AppID:        "00000000-0000-0000-0000-0000000000a2",
			ClientID:     "ayb_cid_bbbb11112222333344445555bbbb11112222333344445555",
			Name:         "SPA Frontend",
			RedirectURIs: []string{"http://localhost:3000/callback", "https://app.example.com/callback"},
			Scopes:       []string{"readonly"},
			ClientType:   "public",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "00000000-0000-0000-0000-000000000003",
			AppID:        "00000000-0000-0000-0000-0000000000a1",
			ClientID:     "ayb_cid_cccc11112222333344445555cccc11112222333344445555",
			Name:         "Backend Service",
			RedirectURIs: []string{"https://api.example.com/callback"},
			Scopes:       []string{"*"},
			ClientType:   "confidential",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
}

// --- List OAuth clients ---

func TestAdminListOAuthClientsSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminListOAuthClients(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/oauth/clients", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.OAuthClientListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 3, result.TotalItems)
	testutil.Equal(t, 3, len(result.Items))
	testutil.Equal(t, "Mobile App", result.Items[0].Name)
}

func TestAdminListOAuthClientsIncludesTokenStatsFields(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminListOAuthClients(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/oauth/clients", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var payload map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&payload))

	rawItems, ok := payload["items"].([]any)
	testutil.True(t, ok, "expected items array in response")
	testutil.True(t, len(rawItems) > 0, "expected at least one client")

	first, ok := rawItems[0].(map[string]any)
	testutil.True(t, ok, "expected first item to be object")

	accessCount, ok := first["activeAccessTokenCount"].(float64)
	testutil.True(t, ok, "expected activeAccessTokenCount field")
	testutil.Equal(t, float64(0), accessCount)

	refreshCount, ok := first["activeRefreshTokenCount"].(float64)
	testutil.True(t, ok, "expected activeRefreshTokenCount field")
	testutil.Equal(t, float64(0), refreshCount)

	totalGrants, ok := first["totalGrants"].(float64)
	testutil.True(t, ok, "expected totalGrants field")
	testutil.Equal(t, float64(0), totalGrants)

	_, ok = first["lastTokenIssuedAt"]
	testutil.True(t, ok, "expected lastTokenIssuedAt field")
}

func TestAdminListOAuthClientsEmpty(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: nil}
	handler := handleAdminListOAuthClients(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/oauth/clients", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.OAuthClientListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 0, result.TotalItems)
}

func TestAdminListOAuthClientsServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{listErr: fmt.Errorf("db down")}
	handler := handleAdminListOAuthClients(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/oauth/clients", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to list oauth clients")
}

func TestAdminListOAuthClientsWithPagination(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminListOAuthClients(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/oauth/clients?page=1&perPage=2", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.OAuthClientListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 3, result.TotalItems)
	testutil.Equal(t, 2, len(result.Items))
	testutil.Equal(t, 2, result.TotalPages)

	// Page 2.
	req2 := httptest.NewRequest(http.MethodGet, "/api/admin/oauth/clients?page=2&perPage=2", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	var result2 auth.OAuthClientListResult
	testutil.NoError(t, json.NewDecoder(w2.Body).Decode(&result2))
	testutil.Equal(t, 1, len(result2.Items))
	testutil.Equal(t, 2, result2.Page)
}

func TestAdminListOAuthClientsPaginationDefaults(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminListOAuthClients(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/oauth/clients?page=0&perPage=0", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var result auth.OAuthClientListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 1, result.Page)
	testutil.Equal(t, 20, result.PerPage)
}

// --- Get OAuth client ---

func TestAdminGetOAuthClientSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminGetOAuthClient(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/oauth/clients/{clientId}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var client auth.OAuthClient
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&client))
	testutil.Equal(t, "Mobile App", client.Name)
	testutil.Equal(t, "confidential", client.ClientType)
	testutil.Equal(t, "readwrite", client.Scopes[0])
}

func TestAdminGetOAuthClientNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminGetOAuthClient(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/oauth/clients/{clientId}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/oauth/clients/ayb_cid_nonexistent000000000000000000000000000000000000", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "oauth client not found")
}

func TestAdminGetOAuthClientServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients(), getErr: fmt.Errorf("db timeout")}
	handler := handleAdminGetOAuthClient(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/oauth/clients/{clientId}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to get oauth client")
}

// --- Create OAuth client ---

func TestAdminCreateOAuthClientConfidentialSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: []auth.OAuthClient{}}
	handler := handleAdminCreateOAuthClient(mgr)

	body := `{"appId":"00000000-0000-0000-0000-0000000000a1","name":"New Client","clientType":"confidential","redirectUris":["https://example.com/callback"],"scopes":["readwrite"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp createOAuthClientResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.True(t, resp.ClientSecret != "", "confidential client should return secret")
	testutil.True(t, strings.HasPrefix(resp.ClientSecret, auth.OAuthClientSecretPrefix), "secret should have ayb_cs_ prefix")
	testutil.Equal(t, "New Client", resp.Client.Name)
	testutil.Equal(t, "confidential", resp.Client.ClientType)
	testutil.True(t, strings.HasPrefix(resp.Client.ClientID, auth.OAuthClientIDPrefix), "client_id should have ayb_cid_ prefix")
}

func TestAdminCreateOAuthClientPublicSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: []auth.OAuthClient{}}
	handler := handleAdminCreateOAuthClient(mgr)

	body := `{"appId":"00000000-0000-0000-0000-0000000000a1","name":"Public SPA","clientType":"public","redirectUris":["http://localhost:3000/callback"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp createOAuthClientResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, "", resp.ClientSecret) // public client: no secret
	testutil.Equal(t, "Public SPA", resp.Client.Name)
	testutil.Equal(t, "public", resp.Client.ClientType)
}

func TestAdminCreateOAuthClientDefaultsToConfidential(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: []auth.OAuthClient{}}
	handler := handleAdminCreateOAuthClient(mgr)

	// No clientType specified — should default to confidential.
	body := `{"appId":"00000000-0000-0000-0000-0000000000a1","name":"Default Type","redirectUris":["https://example.com/cb"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp createOAuthClientResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, "confidential", resp.Client.ClientType)
	testutil.True(t, resp.ClientSecret != "", "default confidential should return secret")
}

func TestAdminCreateOAuthClientMissingName(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{}
	handler := handleAdminCreateOAuthClient(mgr)

	body := `{"appId":"00000000-0000-0000-0000-0000000000a1","redirectUris":["https://example.com/cb"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "name is required")
}

func TestAdminCreateOAuthClientMissingAppID(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{}
	handler := handleAdminCreateOAuthClient(mgr)

	body := `{"name":"No App","redirectUris":["https://example.com/cb"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "appId is required")
}

func TestAdminCreateOAuthClientInvalidAppIDFormat(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{}
	handler := handleAdminCreateOAuthClient(mgr)

	body := `{"appId":"not-a-uuid","name":"Bad App","redirectUris":["https://example.com/cb"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid appId format")
}

func TestAdminCreateOAuthClientAppNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{registerErr: auth.ErrAppNotFound}
	handler := handleAdminCreateOAuthClient(mgr)

	body := `{"appId":"00000000-0000-0000-0000-000000000099","name":"Missing App","redirectUris":["https://example.com/cb"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "app not found")
}

func TestAdminCreateOAuthClientInvalidRedirectURI(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{}
	handler := handleAdminCreateOAuthClient(mgr)

	// HTTP for non-localhost is rejected.
	body := `{"appId":"00000000-0000-0000-0000-0000000000a1","name":"Bad URI","redirectUris":["http://evil.com/callback"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "HTTPS required")
}

func TestAdminCreateOAuthClientInvalidScope(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{}
	handler := handleAdminCreateOAuthClient(mgr)

	body := `{"appId":"00000000-0000-0000-0000-0000000000a1","name":"Bad Scope","redirectUris":["https://example.com/cb"],"scopes":["admin"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid scope")
}

func TestAdminCreateOAuthClientInvalidClientType(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{}
	handler := handleAdminCreateOAuthClient(mgr)

	body := `{"appId":"00000000-0000-0000-0000-0000000000a1","name":"Bad Type","clientType":"hybrid","redirectUris":["https://example.com/cb"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid client type")
}

func TestAdminCreateOAuthClientInvalidJSON(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{}
	handler := handleAdminCreateOAuthClient(mgr)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(`{broken`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminCreateOAuthClientServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{registerErr: fmt.Errorf("db connection lost")}
	handler := handleAdminCreateOAuthClient(mgr)

	body := `{"appId":"00000000-0000-0000-0000-0000000000a1","name":"Will Fail","redirectUris":["https://example.com/cb"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to create oauth client")
}

// --- Revoke OAuth client ---

func TestAdminRevokeOAuthClientSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminRevokeOAuthClient(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/oauth/clients/{clientId}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.NotNil(t, mgr.clients[0].RevokedAt)
	// Other clients unaffected.
	testutil.Nil(t, mgr.clients[1].RevokedAt)
	testutil.Nil(t, mgr.clients[2].RevokedAt)
}

func TestAdminRevokeOAuthClientNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminRevokeOAuthClient(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/oauth/clients/{clientId}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/oauth/clients/ayb_cid_nonexistent000000000000000000000000000000000000", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "oauth client not found")
}

func TestAdminRevokeOAuthClientAlreadyRevoked(t *testing.T) {
	t.Parallel()
	clients := sampleOAuthClients()
	now := time.Now()
	clients[0].RevokedAt = &now
	mgr := &fakeOAuthClientManager{clients: clients}
	handler := handleAdminRevokeOAuthClient(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/oauth/clients/{clientId}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminRevokeOAuthClientServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients(), revokeErr: fmt.Errorf("constraint error")}
	handler := handleAdminRevokeOAuthClient(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/oauth/clients/{clientId}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to revoke oauth client")
}

// --- Rotate secret ---

func TestAdminRotateOAuthClientSecretSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminRotateOAuthClientSecret(mgr)

	r := chi.NewRouter()
	r.Post("/api/admin/oauth/clients/{clientId}/rotate-secret", handler)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555/rotate-secret", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp rotateSecretResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.True(t, resp.ClientSecret != "", "should return new secret")
	testutil.True(t, strings.HasPrefix(resp.ClientSecret, auth.OAuthClientSecretPrefix), "new secret should have ayb_cs_ prefix")
}

func TestAdminRotateOAuthClientSecretNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminRotateOAuthClientSecret(mgr)

	r := chi.NewRouter()
	r.Post("/api/admin/oauth/clients/{clientId}/rotate-secret", handler)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients/ayb_cid_nonexistent000000000000000000000000000000000000/rotate-secret", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "oauth client not found")
}

func TestAdminRotateOAuthClientSecretRevoked(t *testing.T) {
	t.Parallel()
	clients := sampleOAuthClients()
	now := time.Now()
	clients[0].RevokedAt = &now
	mgr := &fakeOAuthClientManager{clients: clients}
	handler := handleAdminRotateOAuthClientSecret(mgr)

	r := chi.NewRouter()
	r.Post("/api/admin/oauth/clients/{clientId}/rotate-secret", handler)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555/rotate-secret", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "revoked")
}

func TestAdminRotateOAuthClientSecretPublicClient(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminRotateOAuthClientSecret(mgr)

	r := chi.NewRouter()
	r.Post("/api/admin/oauth/clients/{clientId}/rotate-secret", handler)

	// SPA Frontend is a public client — cannot rotate secret.
	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients/ayb_cid_bbbb11112222333344445555bbbb11112222333344445555/rotate-secret", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "public client")
}

func TestAdminRotateOAuthClientSecretServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients(), rotateErr: fmt.Errorf("crypto failure")}
	handler := handleAdminRotateOAuthClientSecret(mgr)

	r := chi.NewRouter()
	r.Post("/api/admin/oauth/clients/{clientId}/rotate-secret", handler)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555/rotate-secret", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to rotate oauth client secret")
}

// --- Update OAuth client ---

func TestAdminUpdateOAuthClientSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminUpdateOAuthClient(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/oauth/clients/{clientId}", handler)

	body := `{"name":"Updated Mobile App","redirectUris":["https://new.example.com/callback"],"scopes":["*"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var client auth.OAuthClient
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&client))
	testutil.Equal(t, "Updated Mobile App", client.Name)
	testutil.Equal(t, 1, len(client.RedirectURIs))
	testutil.Equal(t, "https://new.example.com/callback", client.RedirectURIs[0])
	testutil.Equal(t, "*", client.Scopes[0])
}

func TestAdminUpdateOAuthClientNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminUpdateOAuthClient(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/oauth/clients/{clientId}", handler)

	body := `{"name":"Gone","redirectUris":["https://example.com/cb"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/oauth/clients/ayb_cid_nonexistent000000000000000000000000000000000000", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "oauth client not found")
}

func TestAdminUpdateOAuthClientRevokedRejected(t *testing.T) {
	t.Parallel()
	clients := sampleOAuthClients()
	now := time.Now()
	clients[0].RevokedAt = &now
	mgr := &fakeOAuthClientManager{clients: clients}
	handler := handleAdminUpdateOAuthClient(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/oauth/clients/{clientId}", handler)

	body := `{"name":"Should Fail","redirectUris":["https://example.com/cb"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "revoked")
}

func TestAdminUpdateOAuthClientMissingName(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminUpdateOAuthClient(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/oauth/clients/{clientId}", handler)

	body := `{"redirectUris":["https://example.com/cb"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "name is required")
}

func TestAdminUpdateOAuthClientInvalidRedirectURI(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminUpdateOAuthClient(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/oauth/clients/{clientId}", handler)

	body := `{"name":"Bad URIs","redirectUris":["http://evil.com/callback"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "HTTPS required")
}

func TestAdminUpdateOAuthClientInvalidScope(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminUpdateOAuthClient(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/oauth/clients/{clientId}", handler)

	body := `{"name":"Bad Scopes","redirectUris":["https://example.com/cb"],"scopes":["superadmin"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid scope")
}

func TestAdminUpdateOAuthClientInvalidJSON(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients()}
	handler := handleAdminUpdateOAuthClient(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/oauth/clients/{clientId}", handler)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", strings.NewReader(`{broken`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminUpdateOAuthClientServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeOAuthClientManager{clients: sampleOAuthClients(), updateErr: fmt.Errorf("db crash")}
	handler := handleAdminUpdateOAuthClient(mgr)

	r := chi.NewRouter()
	r.Put("/api/admin/oauth/clients/{clientId}", handler)

	body := `{"name":"Error","redirectUris":["https://example.com/cb"],"scopes":["readonly"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/oauth/clients/ayb_cid_aaaa11112222333344445555aaaa11112222333344445555", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to update oauth client")
}
