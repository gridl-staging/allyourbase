package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeOAuthTokenProvider struct {
	validateClient *OAuthClient
	validateErr    error

	exchangeResp *OAuthTokenResponse
	exchangeErr  error

	clientCredentialsResp *OAuthTokenResponse
	clientCredentialsErr  error

	refreshResp *OAuthTokenResponse
	refreshErr  error

	validateCalls               int
	exchangeCalls               int
	clientCredentialsCalls      int
	refreshCalls                int
	lastClientID                string
	lastClientSecret            string
	lastAuthCode                string
	lastRedirectURI             string
	lastCodeVerifier            string
	lastClientCredentialsScope  string
	lastClientCredentialsTables []string
	lastRefreshToken            string
}

func (f *fakeOAuthTokenProvider) ValidateOAuthClientCredentials(_ context.Context, clientID, clientSecret string) (*OAuthClient, error) {
	f.validateCalls++
	f.lastClientID = clientID
	f.lastClientSecret = clientSecret
	if f.validateErr != nil {
		return nil, f.validateErr
	}
	return f.validateClient, nil
}

func (f *fakeOAuthTokenProvider) ExchangeAuthorizationCode(_ context.Context, code, clientID, redirectURI, codeVerifier string) (*OAuthTokenResponse, error) {
	f.exchangeCalls++
	f.lastAuthCode = code
	f.lastClientID = clientID
	f.lastRedirectURI = redirectURI
	f.lastCodeVerifier = codeVerifier
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	return f.exchangeResp, nil
}

func (f *fakeOAuthTokenProvider) ClientCredentialsGrant(_ context.Context, clientID, scope string, allowedTables []string) (*OAuthTokenResponse, error) {
	f.clientCredentialsCalls++
	f.lastClientID = clientID
	f.lastClientCredentialsScope = scope
	f.lastClientCredentialsTables = append([]string(nil), allowedTables...)
	if f.clientCredentialsErr != nil {
		return nil, f.clientCredentialsErr
	}
	return f.clientCredentialsResp, nil
}

func (f *fakeOAuthTokenProvider) RefreshOAuthToken(_ context.Context, refreshToken, clientID string) (*OAuthTokenResponse, error) {
	f.refreshCalls++
	f.lastRefreshToken = refreshToken
	f.lastClientID = clientID
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	return f.refreshResp, nil
}

func newOAuthTokenTestHandler() (*Handler, *fakeOAuthTokenProvider) {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	fake := &fakeOAuthTokenProvider{}
	h.oauthToken = fake
	return h, fake
}

func decodeOAuthTokenError(t *testing.T, w *httptest.ResponseRecorder) OAuthError {
	t.Helper()
	var got OAuthError
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	return got
}

func decodeOAuthTokenResponse(t *testing.T, w *httptest.ResponseRecorder) OAuthTokenResponse {
	t.Helper()
	var got OAuthTokenResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	return got
}

func TestOAuthTokenAuthorizationCodeSuccess(t *testing.T) {
	t.Parallel()

	h, fake := newOAuthTokenTestHandler()
	fake.validateClient = &OAuthClient{
		ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ClientType: OAuthClientTypeConfidential,
		Scopes:     []string{"readonly", "readwrite"},
	}
	fake.exchangeResp = &OAuthTokenResponse{
		AccessToken:  "ayb_at_access1",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: "ayb_rt_refresh1",
		Scope:        "readonly",
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"code-123"},
		"redirect_uri":  {"https://client.example.com/callback"},
		"code_verifier": {"verifier-123"},
		"client_id":     {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"client_secret": {"secret-123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "application/json", w.Header().Get("Content-Type"))
	resp := decodeOAuthTokenResponse(t, w)
	testutil.Equal(t, "ayb_at_access1", resp.AccessToken)
	testutil.Equal(t, "Bearer", resp.TokenType)
	testutil.Equal(t, 3600, resp.ExpiresIn)
	testutil.Equal(t, "ayb_rt_refresh1", resp.RefreshToken)
	testutil.Equal(t, "readonly", resp.Scope)
	testutil.Equal(t, 1, fake.validateCalls)
	testutil.Equal(t, 1, fake.exchangeCalls)
	testutil.Equal(t, "code-123", fake.lastAuthCode)
	testutil.Equal(t, "https://client.example.com/callback", fake.lastRedirectURI)
	testutil.Equal(t, "verifier-123", fake.lastCodeVerifier)
	testutil.Equal(t, "secret-123", fake.lastClientSecret)
}

func TestOAuthTokenAuthorizationCodeSupportsClientBasicAuth(t *testing.T) {
	t.Parallel()

	h, fake := newOAuthTokenTestHandler()
	fake.validateClient = &OAuthClient{
		ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ClientType: OAuthClientTypeConfidential,
		Scopes:     []string{"readonly", "readwrite"},
	}
	fake.exchangeResp = &OAuthTokenResponse{
		AccessToken:  "ayb_at_access2",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: "ayb_rt_refresh2",
		Scope:        "readonly",
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"code-abc"},
		"redirect_uri":  {"https://client.example.com/callback"},
		"code_verifier": {"verifier-abc"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "secret-basic")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, fake.validateCalls)
	testutil.Equal(t, "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", fake.lastClientID)
	testutil.Equal(t, "secret-basic", fake.lastClientSecret)
}

func TestOAuthTokenRejectsMultipleClientAuthMethods(t *testing.T) {
	t.Parallel()

	h, _ := newOAuthTokenTestHandler()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"code-123"},
		"redirect_uri":  {"https://client.example.com/callback"},
		"code_verifier": {"verifier-123"},
		"client_id":     {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"client_secret": {"secret-123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "secret-basic")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthTokenError(t, w)
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
}

func TestOAuthTokenInvalidClientCredentials(t *testing.T) {
	t.Parallel()

	h, fake := newOAuthTokenTestHandler()
	fake.validateErr = NewOAuthError(OAuthErrInvalidClient, "invalid client credentials")

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"code-123"},
		"redirect_uri":  {"https://client.example.com/callback"},
		"code_verifier": {"verifier-123"},
		"client_id":     {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"client_secret": {"bad-secret"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	oe := decodeOAuthTokenError(t, w)
	testutil.Equal(t, OAuthErrInvalidClient, oe.Code)
	testutil.Equal(t, 1, fake.validateCalls)
}

func TestOAuthTokenAuthorizationCodeGrantErrorMapped(t *testing.T) {
	t.Parallel()

	h, fake := newOAuthTokenTestHandler()
	fake.validateClient = &OAuthClient{
		ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ClientType: OAuthClientTypeConfidential,
		Scopes:     []string{"readonly", "readwrite"},
	}
	fake.exchangeErr = NewOAuthError(OAuthErrInvalidGrant, "redirect_uri mismatch")

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"code-123"},
		"redirect_uri":  {"https://evil.example.com/callback"},
		"code_verifier": {"verifier-123"},
		"client_id":     {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"client_secret": {"secret-123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthTokenError(t, w)
	testutil.Equal(t, OAuthErrInvalidGrant, oe.Code)
	testutil.Contains(t, oe.Description, "redirect_uri")
}

func TestOAuthTokenAuthorizationCodeExpiredAndReplayRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		desc string
	}{
		{name: "expired code", desc: "authorization code expired"},
		{name: "replayed code", desc: "authorization code already used"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, fake := newOAuthTokenTestHandler()
			fake.validateClient = &OAuthClient{
				ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ClientType: OAuthClientTypeConfidential,
				Scopes:     []string{"readonly", "readwrite"},
			}
			fake.exchangeErr = NewOAuthError(OAuthErrInvalidGrant, tt.desc)

			form := url.Values{
				"grant_type":    {"authorization_code"},
				"code":          {"code-123"},
				"redirect_uri":  {"https://client.example.com/callback"},
				"code_verifier": {"verifier-123"},
				"client_id":     {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				"client_secret": {"secret-123"},
			}
			req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			h.Routes().ServeHTTP(w, req)

			testutil.Equal(t, http.StatusBadRequest, w.Code)
			oe := decodeOAuthTokenError(t, w)
			testutil.Equal(t, OAuthErrInvalidGrant, oe.Code)
			testutil.Contains(t, oe.Description, tt.desc)
		})
	}
}

func TestOAuthTokenClientCredentialsSuccess(t *testing.T) {
	t.Parallel()

	h, fake := newOAuthTokenTestHandler()
	fake.validateClient = &OAuthClient{
		ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ClientType: OAuthClientTypeConfidential,
		Scopes:     []string{"readonly"},
	}
	fake.clientCredentialsResp = &OAuthTokenResponse{
		AccessToken: "ayb_at_clientcred1",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
		Scope:       "readonly",
	}

	form := url.Values{
		"grant_type":     {"client_credentials"},
		"scope":          {"readonly"},
		"client_id":      {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"client_secret":  {"secret-123"},
		"allowed_tables": {"users,profiles"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	resp := decodeOAuthTokenResponse(t, w)
	testutil.Equal(t, "ayb_at_clientcred1", resp.AccessToken)
	testutil.Equal(t, "Bearer", resp.TokenType)
	testutil.Equal(t, 3600, resp.ExpiresIn)
	testutil.Equal(t, "", resp.RefreshToken)
	testutil.Equal(t, "readonly", resp.Scope)
	testutil.Equal(t, 1, fake.clientCredentialsCalls)
	testutil.Equal(t, "readonly", fake.lastClientCredentialsScope)
	testutil.Equal(t, 2, len(fake.lastClientCredentialsTables))
}

func TestOAuthTokenClientCredentialsRejectsPublicClient(t *testing.T) {
	t.Parallel()

	h, fake := newOAuthTokenTestHandler()
	fake.validateClient = &OAuthClient{
		ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ClientType: OAuthClientTypePublic,
		Scopes:     []string{"readonly"},
	}

	form := url.Values{
		"grant_type": {"client_credentials"},
		"scope":      {"readonly"},
		"client_id":  {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthTokenError(t, w)
	testutil.Equal(t, OAuthErrUnauthorizedClient, oe.Code)
}

func TestOAuthTokenRefreshSuccess(t *testing.T) {
	t.Parallel()

	h, fake := newOAuthTokenTestHandler()
	fake.validateClient = &OAuthClient{
		ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ClientType: OAuthClientTypeConfidential,
		Scopes:     []string{"readonly"},
	}
	fake.refreshResp = &OAuthTokenResponse{
		AccessToken:  "ayb_at_refresh_new",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: "ayb_rt_refresh_new",
		Scope:        "readonly",
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {"ayb_rt_old"},
		"client_id":     {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"client_secret": {"secret-123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	resp := decodeOAuthTokenResponse(t, w)
	testutil.Equal(t, "ayb_at_refresh_new", resp.AccessToken)
	testutil.Equal(t, "ayb_rt_refresh_new", resp.RefreshToken)
	testutil.Equal(t, 1, fake.refreshCalls)
	testutil.Equal(t, "ayb_rt_old", fake.lastRefreshToken)
}

func TestOAuthTokenUnsupportedGrantType(t *testing.T) {
	t.Parallel()

	h, fake := newOAuthTokenTestHandler()
	form := url.Values{
		"grant_type": {"password"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthTokenError(t, w)
	testutil.Equal(t, OAuthErrUnsupportedGrantType, oe.Code)
	testutil.Equal(t, 0, fake.validateCalls)
}

func TestOAuthTokenRequiresFormContentType(t *testing.T) {
	t.Parallel()

	h, _ := newOAuthTokenTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(`{"grant_type":"authorization_code"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthTokenError(t, w)
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
}

func TestOAuthTokenMissingGrantType(t *testing.T) {
	t.Parallel()
	h, _ := newOAuthTokenTestHandler()

	form := url.Values{"client_id": {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthTokenError(t, w)
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
	testutil.Contains(t, oe.Description, "grant_type")
}

func TestOAuthTokenAuthCodeMissingRequiredParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		omit    string
		errDesc string
	}{
		{"missing code", "code", "code"},
		{"missing redirect_uri", "redirect_uri", "redirect_uri"},
		{"missing code_verifier", "code_verifier", "code_verifier"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, fake := newOAuthTokenTestHandler()
			fake.validateClient = &OAuthClient{
				ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ClientType: OAuthClientTypeConfidential,
				Scopes:     []string{"readonly"},
			}

			form := url.Values{
				"grant_type":    {"authorization_code"},
				"code":          {"code-123"},
				"redirect_uri":  {"https://client.example.com/callback"},
				"code_verifier": {"verifier-123"},
				"client_id":     {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				"client_secret": {"secret-123"},
			}
			form.Del(tt.omit)

			req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			h.Routes().ServeHTTP(w, req)

			testutil.Equal(t, http.StatusBadRequest, w.Code)
			oe := decodeOAuthTokenError(t, w)
			testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
			testutil.Contains(t, oe.Description, tt.errDesc)
		})
	}
}

func TestOAuthTokenRefreshMissingRefreshToken(t *testing.T) {
	t.Parallel()
	h, fake := newOAuthTokenTestHandler()
	fake.validateClient = &OAuthClient{
		ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ClientType: OAuthClientTypeConfidential,
		Scopes:     []string{"readonly"},
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"client_secret": {"secret-123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthTokenError(t, w)
	testutil.Equal(t, OAuthErrInvalidRequest, oe.Code)
	testutil.Contains(t, oe.Description, "refresh_token")
}

func TestOAuthTokenClientCredentialsMissingScope(t *testing.T) {
	t.Parallel()
	h, fake := newOAuthTokenTestHandler()
	fake.validateClient = &OAuthClient{
		ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ClientType: OAuthClientTypeConfidential,
		Scopes:     []string{"readonly"},
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"client_secret": {"secret-123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthTokenError(t, w)
	testutil.Equal(t, OAuthErrInvalidScope, oe.Code)
}

func TestOAuthTokenClientCredentialsScopeNotSubset(t *testing.T) {
	t.Parallel()
	h, fake := newOAuthTokenTestHandler()
	fake.validateClient = &OAuthClient{
		ClientID:   "ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ClientType: OAuthClientTypeConfidential,
		Scopes:     []string{"readonly"},
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"scope":         {"readwrite"},
		"client_id":     {"ayb_cid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"client_secret": {"secret-123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	oe := decodeOAuthTokenError(t, w)
	testutil.Equal(t, OAuthErrInvalidScope, oe.Code)
	testutil.Contains(t, oe.Description, "not allowed")
}
