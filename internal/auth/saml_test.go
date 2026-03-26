package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
)

const samlIDPMetadataTemplate = `<?xml version="1.0"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.com/metadata">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="%s"/>
  </IDPSSODescriptor>
</EntityDescriptor>`

func newTestSAMLService(t *testing.T) *SAMLService {
	t.Helper()
	authSvc := newTestService()
	samlSvc, err := NewSAMLService("http://localhost:8090", t.TempDir(), authSvc, testutil.DiscardLogger())
	testutil.NoError(t, err)
	return samlSvc
}

func registerSAMLProvider(t *testing.T, samlSvc *SAMLService, name string) {
	t.Helper()
	err := samlSvc.UpsertProvider(context.Background(), config.SAMLProvider{
		Enabled:        true,
		Name:           name,
		EntityID:       "https://sp.example.com/" + name,
		IDPMetadataXML: testSAMLIDPMetadataXML("https://idp.example.com/sso"),
		AttributeMapping: map[string]string{
			"email": "email",
			"name":  "name",
		},
	})
	testutil.NoError(t, err)
}

func testSAMLIDPMetadataXML(ssoURL string) string {
	return fmt.Sprintf(samlIDPMetadataTemplate, ssoURL)
}

func TestSAMLServiceInitiateLoginRedirectsToIDP(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	registerSAMLProvider(t, samlSvc, "okta")

	redirectURL, requestID, err := samlSvc.InitiateLogin("okta", "https://app.example.com/post-login")
	testutil.NoError(t, err)
	testutil.True(t, requestID != "", "request ID should be populated")
	testutil.Equal(t, "idp.example.com", redirectURL.Host)
	testutil.Contains(t, redirectURL.String(), "RelayState=")
	testutil.Contains(t, redirectURL.String(), url.QueryEscape("https://app.example.com/post-login"))
}

func TestSAMLServiceHandleCallbackCallsOAuthLogin(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	registerSAMLProvider(t, samlSvc, "okta")

	var gotProvider string
	var gotInfo *OAuthUserInfo
	samlSvc.oauthLoginFn = func(_ context.Context, provider string, info *OAuthUserInfo) (*User, string, string, error) {
		gotProvider = provider
		gotInfo = info
		return &User{ID: "u_123", Email: "saml-user@example.com"}, "access-token", "refresh-token", nil
	}
	samlSvc.parseAssertionFn = func(_ *http.Request) (*SAMLAssertion, error) {
		return &SAMLAssertion{
			SubjectNameID: "idp-user-1",
			Attributes: map[string]string{
				"email": "saml-user@example.com",
				"name":  "SAML User",
			},
		}, nil
	}

	_, reqID, err := samlSvc.InitiateLogin("okta", "https://app.example.com/home")
	testutil.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/saml/okta/acs", strings.NewReader("RelayState=https%3A%2F%2Fapp.example.com%2Fhome"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	testutil.NoError(t, req.ParseForm())

	user, accessToken, refreshToken, relayState, err := samlSvc.HandleCallback(req, "okta", reqID)
	testutil.NoError(t, err)
	testutil.Equal(t, "u_123", user.ID)
	testutil.Equal(t, "access-token", accessToken)
	testutil.Equal(t, "refresh-token", refreshToken)
	testutil.Equal(t, "https://app.example.com/home", relayState)
	testutil.Equal(t, "saml:okta", gotProvider)
	testutil.NotNil(t, gotInfo)
	testutil.Equal(t, "idp-user-1", gotInfo.ProviderUserID)
	testutil.Equal(t, "saml-user@example.com", gotInfo.Email)
	testutil.Equal(t, "SAML User", gotInfo.Name)
}

func TestSAMLServiceHandleCallbackRejectsInvalidAssertion(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	registerSAMLProvider(t, samlSvc, "okta")

	_, reqID, err := samlSvc.InitiateLogin("okta", "")
	testutil.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/saml/okta/acs", strings.NewReader("SAMLResponse=invalid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	testutil.NoError(t, req.ParseForm())

	_, _, _, _, err = samlSvc.HandleCallback(req, "okta", reqID)
	testutil.NotNil(t, err)
}

func TestSAMLServiceSPMetadataContainsEntityDescriptor(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	registerSAMLProvider(t, samlSvc, "okta")

	b, err := samlSvc.SPMetadataXML("okta")
	testutil.NoError(t, err)
	testutil.Contains(t, string(b), "EntityDescriptor")
	testutil.Contains(t, string(b), "okta")
}

func TestSAMLAuthRoutesLoginMetadataAndACS(t *testing.T) {
	t.Parallel()

	authSvc := newTestService()
	h := NewHandler(authSvc, testutil.DiscardLogger())
	samlSvc := newTestSAMLService(t)
	registerSAMLProvider(t, samlSvc, "okta")
	h.SetSAMLService(samlSvc)

	samlSvc.oauthLoginFn = func(_ context.Context, _ string, _ *OAuthUserInfo) (*User, string, string, error) {
		return &User{ID: "u_123", Email: "saml-route@example.com"}, "route-access", "route-refresh", nil
	}
	samlSvc.parseAssertionFn = func(_ *http.Request) (*SAMLAssertion, error) {
		return &SAMLAssertion{SubjectNameID: "sub-1", Attributes: map[string]string{"email": "saml-route@example.com"}}, nil
	}

	routes := h.Routes()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/saml/okta/login?redirect_to=https://app.example.com/post-login", nil)
	req.Host = "localhost:8090"
	routes.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	testutil.Contains(t, w.Header().Get("Location"), "RelayState=")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/saml/okta/metadata", nil)
	routes.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "EntityDescriptor")

	_, requestID, err := samlSvc.InitiateLogin("okta", "https://app.example.com/post-login")
	testutil.NoError(t, err)

	form := url.Values{}
	form.Set("RelayState", "https://app.example.com/post-login")
	form.Set("request_id", requestID)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/saml/okta/acs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	routes.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "route-access")
	testutil.Contains(t, w.Body.String(), "route-refresh")
}

func TestValidateSAMLProviderName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "simple", input: "okta", wantErr: false},
		{name: "with_hyphen", input: "azure-ad", wantErr: false},
		{name: "with_underscore", input: "google_oidc", wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "path_traversal", input: "../escape", wantErr: true},
		{name: "slash", input: "tenant/idp", wantErr: true},
		{name: "dot", input: "okta.prod", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSAMLProviderName(tt.input)
			if tt.wantErr {
				testutil.NotNil(t, err)
				return
			}
			testutil.NoError(t, err)
		})
	}
}

func TestSAMLServiceUpsertProviderRejectsInvalidProviderName(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	err := samlSvc.UpsertProvider(context.Background(), config.SAMLProvider{
		Enabled:        true,
		Name:           "../escape",
		EntityID:       "https://sp.example.com/escape",
		IDPMetadataXML: testSAMLIDPMetadataXML("https://idp.example.com/sso"),
	})
	testutil.ErrorContains(t, err, "invalid provider name")
}
