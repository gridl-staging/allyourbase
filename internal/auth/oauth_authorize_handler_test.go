package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeOAuthAuthorizationProvider struct {
	clients map[string]*OAuthClient

	hasConsentResult bool
	hasConsentErr    error
	saveConsentErr   error
	createCode       string
	createCodeErr    error
	getClientErr     error

	lastUserID              string
	lastClientID            string
	lastScope               string
	lastAllowedTables       []string
	lastRedirectURI         string
	lastCodeChallenge       string
	lastCodeChallengeMethod string
	lastState               string
	saveConsentCalls        int
	createCodeCalls         int
}

func (f *fakeOAuthAuthorizationProvider) GetOAuthClient(_ context.Context, clientID string) (*OAuthClient, error) {
	if f.getClientErr != nil {
		return nil, f.getClientErr
	}
	client, ok := f.clients[clientID]
	if !ok {
		return nil, ErrOAuthClientNotFound
	}
	return client, nil
}

func (f *fakeOAuthAuthorizationProvider) HasConsent(_ context.Context, userID, clientID, scope string, allowedTables []string) (bool, error) {
	f.lastUserID = userID
	f.lastClientID = clientID
	f.lastScope = scope
	f.lastAllowedTables = append([]string(nil), allowedTables...)
	if f.hasConsentErr != nil {
		return false, f.hasConsentErr
	}
	return f.hasConsentResult, nil
}

func (f *fakeOAuthAuthorizationProvider) SaveConsent(_ context.Context, userID, clientID, scope string, allowedTables []string) error {
	f.lastUserID = userID
	f.lastClientID = clientID
	f.lastScope = scope
	f.lastAllowedTables = append([]string(nil), allowedTables...)
	f.saveConsentCalls++
	return f.saveConsentErr
}

func (f *fakeOAuthAuthorizationProvider) CreateAuthorizationCode(_ context.Context, clientID, userID, redirectURI, scope string, allowedTables []string, codeChallenge, codeChallengeMethod, state string) (string, error) {
	f.lastClientID = clientID
	f.lastUserID = userID
	f.lastRedirectURI = redirectURI
	f.lastScope = scope
	f.lastAllowedTables = append([]string(nil), allowedTables...)
	f.lastCodeChallenge = codeChallenge
	f.lastCodeChallengeMethod = codeChallengeMethod
	f.lastState = state
	f.createCodeCalls++
	if f.createCodeErr != nil {
		return "", f.createCodeErr
	}
	if f.createCode == "" {
		return "auth-code-123", nil
	}
	return f.createCode, nil
}

func newOAuthAuthorizeTestHandler(t *testing.T) (*Handler, string, *fakeOAuthAuthorizationProvider) {
	t.Helper()

	svc := newTestService()
	token := generateTestToken(t, svc, "user-123", "user@example.com")
	h := NewHandler(svc, testutil.DiscardLogger())
	provider := &fakeOAuthAuthorizationProvider{
		clients: map[string]*OAuthClient{
			"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": {
				ClientID:     "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Name:         "Test OAuth App",
				RedirectURIs: []string{"https://client.example.com/callback"},
				Scopes:       []string{"readonly", "readwrite"},
			},
		},
	}
	h.oauthAuthorize = provider
	return h, token, provider
}

func decodeOAuthErr(t *testing.T, body string) OAuthError {
	t.Helper()
	var got OAuthError
	testutil.NoError(t, json.Unmarshal([]byte(body), &got))
	return got
}

func TestOAuthAuthorizeRouteRequiresAuth(t *testing.T) {
	t.Parallel()
	h, _, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://client.example.com/callback&scope=readonly&state=s1&code_challenge=c&code_challenge_method=S256", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOAuthAuthorizeMissingState(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://client.example.com/callback&scope=readonly&code_challenge=abc&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
	testutil.Contains(t, oe.Description, "state")
}

func TestOAuthAuthorizeInvalidRedirectURIRejected(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://evil.example.com/callback&scope=readonly&state=s1&code_challenge=abc&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
	testutil.Contains(t, oe.Description, "redirect_uri")
}

func TestOAuthAuthorizeRejectsPlainPKCEMethod(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://client.example.com/callback&scope=readonly&state=s1&code_challenge=abc&code_challenge_method=plain", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
	testutil.Contains(t, oe.Description, "S256")
}

func TestOAuthAuthorizeInvalidScopeRejected(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://client.example.com/callback&scope=*&state=s1&code_challenge=abc&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidScope, oe.Code)
}

func TestOAuthAuthorizeReturnsConsentPromptWhenConsentMissing(t *testing.T) {
	t.Parallel()
	h, token, provider := newOAuthAuthorizeTestHandler(t)
	provider.hasConsentResult = false
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://client.example.com/callback&scope=readonly&state=s1&code_challenge=abc&code_challenge_method=S256&allowed_tables=users,profiles", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		RequiresConsent     bool     `json:"requires_consent"`
		ClientID            string   `json:"client_id"`
		ClientName          string   `json:"client_name"`
		RedirectURI         string   `json:"redirect_uri"`
		Scope               string   `json:"scope"`
		State               string   `json:"state"`
		CodeChallenge       string   `json:"code_challenge"`
		CodeChallengeMethod string   `json:"code_challenge_method"`
		AllowedTables       []string `json:"allowed_tables"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.True(t, resp.RequiresConsent, "expected consent prompt")
	testutil.Equal(t, "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", resp.ClientID)
	testutil.Equal(t, "Test OAuth App", resp.ClientName)
	testutil.Equal(t, "readonly", resp.Scope)
	testutil.Equal(t, "s1", resp.State)
	testutil.Equal(t, "abc", resp.CodeChallenge)
	testutil.Equal(t, "S256", resp.CodeChallengeMethod)
	testutil.Equal(t, 2, len(resp.AllowedTables))
	testutil.Equal(t, "users", resp.AllowedTables[0])
	testutil.Equal(t, "profiles", resp.AllowedTables[1])
	testutil.Equal(t, 2, len(provider.lastAllowedTables))
	testutil.Equal(t, "users", provider.lastAllowedTables[0])
	testutil.Equal(t, "profiles", provider.lastAllowedTables[1])
	testutil.Equal(t, 0, provider.createCodeCalls)
}

func TestOAuthAuthorizeSkipsConsentAndRedirectsWithCodeAndState(t *testing.T) {
	t.Parallel()
	h, token, provider := newOAuthAuthorizeTestHandler(t)
	provider.hasConsentResult = true
	provider.createCode = "issued-code-001"
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://client.example.com/callback&scope=readonly&state=opaque-state-42&code_challenge=abc&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusFound, w.Code)
	loc := w.Header().Get("Location")
	u, err := url.Parse(loc)
	testutil.NoError(t, err)
	testutil.Equal(t, "https://client.example.com/callback", u.Scheme+"://"+u.Host+u.Path)
	testutil.Equal(t, "issued-code-001", u.Query().Get("code"))
	testutil.Equal(t, "opaque-state-42", u.Query().Get("state"))
	testutil.Equal(t, 1, provider.createCodeCalls)
}

func TestOAuthConsentDenyRedirectsAccessDenied(t *testing.T) {
	t.Parallel()
	h, token, provider := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	body := `{"decision":"deny","response_type":"code","client_id":"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","redirect_uri":"https://client.example.com/callback","scope":"readonly","state":"deny-state","code_challenge":"abc","code_challenge_method":"S256"}`
	req := httptest.NewRequest(http.MethodPost, "/authorize/consent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusFound, w.Code)
	loc := w.Header().Get("Location")
	u, err := url.Parse(loc)
	testutil.NoError(t, err)
	testutil.Equal(t, "access_denied", u.Query().Get("error"))
	testutil.Equal(t, "deny-state", u.Query().Get("state"))
	testutil.Equal(t, 0, provider.saveConsentCalls)
	testutil.Equal(t, 0, provider.createCodeCalls)
}

func TestOAuthConsentApproveSavesConsentAndRedirectsWithCode(t *testing.T) {
	t.Parallel()
	h, token, provider := newOAuthAuthorizeTestHandler(t)
	provider.createCode = "issued-after-consent"
	router := h.Routes()

	body := `{"decision":"approve","response_type":"code","client_id":"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","redirect_uri":"https://client.example.com/callback","scope":"readonly","state":"approved-state","code_challenge":"abc","code_challenge_method":"S256","allowed_tables":["users"]}`
	req := httptest.NewRequest(http.MethodPost, "/authorize/consent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusFound, w.Code)
	loc := w.Header().Get("Location")
	u, err := url.Parse(loc)
	testutil.NoError(t, err)
	testutil.Equal(t, "issued-after-consent", u.Query().Get("code"))
	testutil.Equal(t, "approved-state", u.Query().Get("state"))
	testutil.Equal(t, 1, provider.saveConsentCalls)
	testutil.Equal(t, 1, len(provider.lastAllowedTables))
	testutil.Equal(t, "users", provider.lastAllowedTables[0])
	testutil.Equal(t, 1, provider.createCodeCalls)
}

func TestOAuthAuthorizeRevokedClientRejected(t *testing.T) {
	t.Parallel()
	h, token, provider := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	// Mark the client as revoked.
	now := time.Now()
	revokedClient := *provider.clients["ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]
	revokedClient.RevokedAt = &now
	provider.clients["ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"] = &revokedClient

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://client.example.com/callback&scope=readonly&state=s1&code_challenge=abc&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidClient, oe.Code)
	testutil.Contains(t, oe.Description, "revoked")
	testutil.Equal(t, 0, provider.createCodeCalls)
}

func TestOAuthConsentRevokedClientRejected(t *testing.T) {
	t.Parallel()
	h, token, provider := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	// Mark the client as revoked.
	now := time.Now()
	revokedClient := *provider.clients["ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]
	revokedClient.RevokedAt = &now
	provider.clients["ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"] = &revokedClient

	body := `{"decision":"approve","response_type":"code","client_id":"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","redirect_uri":"https://client.example.com/callback","scope":"readonly","state":"s1","code_challenge":"abc","code_challenge_method":"S256"}`
	req := httptest.NewRequest(http.MethodPost, "/authorize/consent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidClient, oe.Code)
	testutil.Contains(t, oe.Description, "revoked")
	testutil.Equal(t, 0, provider.saveConsentCalls)
	testutil.Equal(t, 0, provider.createCodeCalls)
}

func TestOAuthConsentInvalidDecision(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	body := `{"decision":"maybe","response_type":"code","client_id":"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","redirect_uri":"https://client.example.com/callback","scope":"readonly","state":"s","code_challenge":"abc","code_challenge_method":"S256"}`
	req := httptest.NewRequest(http.MethodPost, "/authorize/consent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
	testutil.Contains(t, oe.Description, "decision")
}

func TestOAuthAuthorizeUnknownClientID(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&redirect_uri=https://client.example.com/callback&scope=readonly&state=s1&code_challenge=abc&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidClient, oe.Code)
}

func TestOAuthAuthorizeInvalidResponseType(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=token&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://client.example.com/callback&scope=readonly&state=s1&code_challenge=abc&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
	testutil.Contains(t, oe.Description, "response_type")
}

func TestOAuthAuthorizeMissingCodeChallenge(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://client.example.com/callback&scope=readonly&state=s1&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
	testutil.Contains(t, oe.Description, "code_challenge")
}

func TestOAuthAuthorizeMissingClientID(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&redirect_uri=https://client.example.com/callback&scope=readonly&state=s1&code_challenge=abc&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
	testutil.Contains(t, oe.Description, "client_id")
}

func TestOAuthAuthorizeMissingRedirectURI(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&scope=readonly&state=s1&code_challenge=abc&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthErr(t, w.Body.String())
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
	testutil.Contains(t, oe.Description, "redirect_uri")
}

// --- JSON response mode tests (Accept: application/json for SPA consent page) ---

func TestOAuthAuthorizeJSONResponseWhenConsentExists(t *testing.T) {
	t.Parallel()
	h, token, provider := newOAuthAuthorizeTestHandler(t)
	provider.hasConsentResult = true
	provider.createCode = "json-code-001"
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&client_id=ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&redirect_uri=https://client.example.com/callback&scope=readonly&state=json-state-42&code_challenge=abc&code_challenge_method=S256", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		RequiresConsent bool   `json:"requires_consent"`
		RedirectTo      string `json:"redirect_to"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, false, resp.RequiresConsent)
	testutil.Contains(t, resp.RedirectTo, "code=json-code-001")
	testutil.Contains(t, resp.RedirectTo, "state=json-state-42")
}

func TestOAuthConsentApproveJSONResponse(t *testing.T) {
	t.Parallel()
	h, token, provider := newOAuthAuthorizeTestHandler(t)
	provider.createCode = "consent-json-code"
	router := h.Routes()

	body := `{"decision":"approve","response_type":"code","client_id":"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","redirect_uri":"https://client.example.com/callback","scope":"readonly","state":"consent-json-state","code_challenge":"abc","code_challenge_method":"S256"}`
	req := httptest.NewRequest(http.MethodPost, "/authorize/consent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		RedirectTo string `json:"redirect_to"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Contains(t, resp.RedirectTo, "code=consent-json-code")
	testutil.Contains(t, resp.RedirectTo, "state=consent-json-state")
}

func TestOAuthConsentDenyJSONResponse(t *testing.T) {
	t.Parallel()
	h, token, _ := newOAuthAuthorizeTestHandler(t)
	router := h.Routes()

	body := `{"decision":"deny","response_type":"code","client_id":"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","redirect_uri":"https://client.example.com/callback","scope":"readonly","state":"deny-json-state","code_challenge":"abc","code_challenge_method":"S256"}`
	req := httptest.NewRequest(http.MethodPost, "/authorize/consent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		RedirectTo string `json:"redirect_to"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Contains(t, resp.RedirectTo, "error=access_denied")
	testutil.Contains(t, resp.RedirectTo, "state=deny-json-state")
}
