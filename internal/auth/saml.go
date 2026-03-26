// Package auth Implements SAML service provider integration for federated authentication, handling request creation, assertion parsing and validation, provider configuration, and certificate management.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/config"
)

const samlRequestTTL = 5 * time.Minute

var errSAMLProviderNotFound = errors.New("saml provider not configured")

var samlProviderNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type SAMLAssertion struct {
	SubjectNameID string
	Attributes    map[string]string
	NotBefore     *time.Time
	NotOnOrAfter  *time.Time
}

type samlProviderState struct {
	name             string
	entityID         string
	idpMetadataXML   string
	ssoURL           string
	attributeMapping map[string]string
	certPEM          string
}

type samlRequestState struct {
	provider  string
	expiresAt time.Time
}

type SAMLService struct {
	baseURL   string
	dataDir   string
	authSvc   *Service
	logger    *slog.Logger
	now       func() time.Time
	mu        sync.RWMutex
	providers map[string]*samlProviderState
	requests  map[string]samlRequestState

	// Test-only seams.
	parseAssertionFn func(*http.Request) (*SAMLAssertion, error)
	oauthLoginFn     func(ctx context.Context, provider string, info *OAuthUserInfo) (*User, string, string, error)
}

// Creates and initializes a new SAMLService with the provided base URL, data directory, and auth service reference. Ensures the data directory is writable, falling back to a temporary directory if necessary. Returns an error if required parameters are empty or directory creation fails.
func NewSAMLService(baseURL, dataDir string, authSvc *Service, logger *slog.Logger) (*SAMLService, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("data directory is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		fallbackDir := filepath.Join(os.TempDir(), "ayb", "saml")
		if mkErr := os.MkdirAll(fallbackDir, 0o700); mkErr != nil {
			return nil, fmt.Errorf("creating SAML data directory: %w", err)
		}
		logger.Warn("SAML data directory not writable, falling back to temp directory", "path", dataDir, "fallback", fallbackDir, "error", err)
		dataDir = fallbackDir
	}

	return &SAMLService{
		baseURL:   strings.TrimRight(baseURL, "/"),
		dataDir:   dataDir,
		authSvc:   authSvc,
		logger:    logger,
		now:       time.Now,
		providers: make(map[string]*samlProviderState),
		requests:  make(map[string]samlRequestState),
	}, nil
}

// DefaultSAMLDataDir resolves the on-disk directory for generated SP certs/keys.
func DefaultSAMLDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".ayb/saml"
	}
	return filepath.Join(home, ".ayb", "saml")
}

// ValidateSAMLProviderName enforces a filesystem-safe provider identifier.
// Allowed characters: letters, digits, underscore, hyphen; max length 64.
func ValidateSAMLProviderName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("provider name is required")
	}
	if !samlProviderNameRE.MatchString(name) {
		return fmt.Errorf("invalid provider name")
	}
	return nil
}

// Adds or updates a SAML provider configuration. Validates the provider name, extracts the SSO URL from the metadata, ensures the SP certificate and key exist, and stores the provider state. Returns an error if any required fields are missing or invalid.
func (s *SAMLService) UpsertProvider(_ context.Context, p config.SAMLProvider) error {
	name := strings.TrimSpace(p.Name)
	if err := ValidateSAMLProviderName(name); err != nil {
		return err
	}
	entityID := strings.TrimSpace(p.EntityID)
	if entityID == "" {
		return fmt.Errorf("provider entity_id is required")
	}
	metadata := strings.TrimSpace(p.IDPMetadataXML)
	if metadata == "" {
		return fmt.Errorf("idp_metadata_xml is required")
	}
	ssoURL, err := extractSSOURL(metadata)
	if err != nil {
		return fmt.Errorf("invalid idp metadata: %w", err)
	}
	certPEM, _, err := s.ensureSPCertKey(name, p.SPCertFile, p.SPKeyFile)
	if err != nil {
		return err
	}

	mapping := map[string]string{}
	for k, v := range p.AttributeMapping {
		mapping[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[name] = &samlProviderState{
		name:             name,
		entityID:         entityID,
		idpMetadataXML:   metadata,
		ssoURL:           ssoURL,
		attributeMapping: mapping,
		certPEM:          certPEM,
	}
	return nil
}

func (s *SAMLService) DeleteProvider(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.providers, strings.TrimSpace(name))
}

// Initiates a SAML login flow by creating an AuthnRequest. Generates a request ID, constructs the SAML request XML, base64-encodes it, and builds a redirect URL to the IdP's SSO endpoint. Returns the redirect URL, request ID, and an error if any step fails.
func (s *SAMLService) InitiateLogin(providerName, relayState string) (*url.URL, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.providers[strings.TrimSpace(providerName)]
	if !ok {
		return nil, "", errSAMLProviderNotFound
	}

	requestID, err := newSAMLRequestID()
	if err != nil {
		return nil, "", fmt.Errorf("generating SAML request ID: %w", err)
	}
	s.requests[requestID] = samlRequestState{
		provider:  state.name,
		expiresAt: s.now().Add(samlRequestTTL),
	}
	s.pruneExpiredLocked()

	requestXML := fmt.Sprintf(`<AuthnRequest ID="%s" Version="2.0" IssueInstant="%s" AssertionConsumerServiceURL="%s/api/auth/saml/%s/acs"><Issuer>%s</Issuer></AuthnRequest>`,
		requestID,
		s.now().UTC().Format(time.RFC3339),
		s.baseURL,
		url.PathEscape(state.name),
		xmlEscape(state.entityID),
	)
	reqB64 := base64.StdEncoding.EncodeToString([]byte(requestXML))

	redirectURL, err := url.Parse(state.ssoURL)
	if err != nil {
		return nil, "", fmt.Errorf("parsing IdP SSO URL: %w", err)
	}
	q := redirectURL.Query()
	q.Set("SAMLRequest", reqB64)
	if strings.TrimSpace(relayState) != "" {
		q.Set("RelayState", relayState)
	}
	redirectURL.RawQuery = q.Encode()
	return redirectURL, requestID, nil
}

// Processes the SAML assertion callback from the IdP after user authentication. Validates the request, parses and validates the assertion, maps attributes to email and name, then delegates to the auth service to create or link a user account. Returns the authenticated user, access token, refresh token, and relay state, or an error if validation fails.
func (s *SAMLService) HandleCallback(r *http.Request, providerName, requestID string) (*User, string, string, string, error) {
	state, err := s.validateRequest(providerName, requestID)
	if err != nil {
		return nil, "", "", "", err
	}
	assertion, err := s.parseAssertion(r)
	if err != nil {
		return nil, "", "", "", err
	}
	if assertion == nil {
		return nil, "", "", "", fmt.Errorf("missing SAML assertion")
	}
	now := s.now()
	if assertion.NotBefore != nil && now.Before(assertion.NotBefore.Add(-30*time.Second)) {
		return nil, "", "", "", fmt.Errorf("assertion is not yet valid")
	}
	if assertion.NotOnOrAfter != nil && !now.Before(*assertion.NotOnOrAfter) {
		return nil, "", "", "", fmt.Errorf("assertion is expired")
	}

	emailKey := "email"
	nameKey := "name"
	if v := strings.TrimSpace(state.attributeMapping["email"]); v != "" {
		emailKey = v
	}
	if v := strings.TrimSpace(state.attributeMapping["name"]); v != "" {
		nameKey = v
	}
	email := strings.TrimSpace(assertion.Attributes[emailKey])
	displayName := strings.TrimSpace(assertion.Attributes[nameKey])
	subject := strings.TrimSpace(assertion.SubjectNameID)
	if subject == "" {
		subject = email
	}
	if subject == "" {
		return nil, "", "", "", fmt.Errorf("assertion subject is required")
	}

	login := s.oauthLoginFn
	if login == nil {
		login = s.authSvc.OAuthLogin
	}
	if login == nil {
		return nil, "", "", "", fmt.Errorf("auth service is not configured")
	}
	user, accessToken, refreshToken, err := login(r.Context(), "saml:"+state.name, &OAuthUserInfo{
		ProviderUserID: subject,
		Email:          email,
		Name:           displayName,
	})
	if err != nil {
		return nil, "", "", "", err
	}
	return user, accessToken, refreshToken, r.FormValue("RelayState"), nil
}

// Generates SAML Service Provider metadata XML. Returns the SP's entity descriptor including the signing certificate and assertion consumer service endpoint. Returns an error if the provider is not found or the certificate is invalid.
func (s *SAMLService) SPMetadataXML(providerName string) ([]byte, error) {
	s.mu.RLock()
	state, ok := s.providers[strings.TrimSpace(providerName)]
	s.mu.RUnlock()
	if !ok {
		return nil, errSAMLProviderNotFound
	}

	certBlock, _ := pem.Decode([]byte(state.certPEM))
	if certBlock == nil {
		return nil, fmt.Errorf("invalid SP certificate")
	}
	certB64 := base64.StdEncoding.EncodeToString(certBlock.Bytes)
	metadata := fmt.Sprintf(`<?xml version="1.0"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="%s">
  <SPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol" WantAssertionsSigned="true">
    <KeyDescriptor use="signing">
      <KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#">
        <X509Data><X509Certificate>%s</X509Certificate></X509Data>
      </KeyInfo>
    </KeyDescriptor>
    <AssertionConsumerService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="%s/api/auth/saml/%s/acs" index="1"/>
  </SPSSODescriptor>
</EntityDescriptor>`, xmlEscape(state.entityID), certB64, s.baseURL, url.PathEscape(state.name))
	return []byte(metadata), nil
}

// Parses a SAML assertion from an HTTP request. Extracts the SAMLResponse form parameter, base64-decodes it, and parses the XML. If a test seam is configured, uses that instead. Returns the parsed SAMLAssertion or an error if parsing fails.
func (s *SAMLService) parseAssertion(r *http.Request) (*SAMLAssertion, error) {
	if s.parseAssertionFn != nil {
		return s.parseAssertionFn(r)
	}
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("parsing form: %w", err)
	}
	raw := strings.TrimSpace(r.FormValue("SAMLResponse"))
	if raw == "" {
		return nil, fmt.Errorf("missing SAMLResponse")
	}
	xmlBytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding SAMLResponse: %w", err)
	}
	return decodeSAMLAssertion(xmlBytes)
}

// Decodes and parses a SAML assertion from XML bytes. Unmarshals the assertion structure, extracts the subject name ID, attributes, and validity conditions. Returns a SAMLAssertion struct or an error if parsing fails.
func decodeSAMLAssertion(xmlBytes []byte) (*SAMLAssertion, error) {
	type xmlAttributeValue struct {
		Value string `xml:",chardata"`
	}
	type xmlAttribute struct {
		Name   string              `xml:"Name,attr"`
		Values []xmlAttributeValue `xml:"AttributeValue"`
	}
	type xmlAttributeStatement struct {
		Attributes []xmlAttribute `xml:"Attribute"`
	}
	type xmlNameID struct {
		Value string `xml:",chardata"`
	}
	type xmlSubject struct {
		NameID xmlNameID `xml:"NameID"`
	}
	type xmlConditions struct {
		NotBefore    string `xml:"NotBefore,attr"`
		NotOnOrAfter string `xml:"NotOnOrAfter,attr"`
	}
	type xmlAssertion struct {
		Subject            xmlSubject              `xml:"Subject"`
		Conditions         xmlConditions           `xml:"Conditions"`
		AttributeStatement []xmlAttributeStatement `xml:"AttributeStatement"`
	}
	type xmlResponse struct {
		Assertions []xmlAssertion `xml:"Assertion"`
	}

	var resp xmlResponse
	if err := xml.Unmarshal(xmlBytes, &resp); err != nil {
		return nil, fmt.Errorf("parsing assertion XML: %w", err)
	}
	if len(resp.Assertions) == 0 {
		return nil, fmt.Errorf("no assertion in response")
	}
	a := resp.Assertions[0]
	out := &SAMLAssertion{
		SubjectNameID: strings.TrimSpace(a.Subject.NameID.Value),
		Attributes:    map[string]string{},
	}
	for _, stmt := range a.AttributeStatement {
		for _, attr := range stmt.Attributes {
			if len(attr.Values) == 0 {
				continue
			}
			out.Attributes[strings.TrimSpace(attr.Name)] = strings.TrimSpace(attr.Values[0].Value)
		}
	}
	if ts := strings.TrimSpace(a.Conditions.NotBefore); ts != "" {
		parsed, err := parseSAMLTime(ts)
		if err != nil {
			return nil, fmt.Errorf("invalid NotBefore: %w", err)
		}
		out.NotBefore = &parsed
	}
	if ts := strings.TrimSpace(a.Conditions.NotOnOrAfter); ts != "" {
		parsed, err := parseSAMLTime(ts)
		if err != nil {
			return nil, fmt.Errorf("invalid NotOnOrAfter: %w", err)
		}
		out.NotOnOrAfter = &parsed
	}
	return out, nil
}

func parseSAMLTime(raw string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02T15:04:05Z", raw)
}

// Validates a SAML request by provider name and request ID. Checks that the provider exists, the request ID is valid and not expired, and the request is associated with the correct provider. Consumes the request upon successful validation. Returns the provider state or an error if validation fails.
func (s *SAMLService) validateRequest(providerName, requestID string) (*samlProviderState, error) {
	providerName = strings.TrimSpace(providerName)
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("missing request ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.providers[providerName]
	if !ok {
		return nil, errSAMLProviderNotFound
	}
	reqState, ok := s.requests[requestID]
	if !ok {
		return nil, fmt.Errorf("invalid or expired SAML request")
	}
	delete(s.requests, requestID)
	if s.now().After(reqState.expiresAt) {
		return nil, fmt.Errorf("invalid or expired SAML request")
	}
	if reqState.provider != providerName {
		return nil, fmt.Errorf("provider mismatch for request")
	}
	return state, nil
}

func (s *SAMLService) pruneExpiredLocked() {
	now := s.now()
	for id, req := range s.requests {
		if now.After(req.expiresAt) {
			delete(s.requests, id)
		}
	}
}

func newSAMLRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Extracts the SingleSignOnService URL from IdP metadata XML. Parses the SAML metadata and returns the first available location attribute. Returns an error if the metadata is invalid or no SingleSignOnService is found.
func extractSSOURL(metadataXML string) (string, error) {
	type mdSSO struct {
		Binding  string `xml:"Binding,attr"`
		Location string `xml:"Location,attr"`
	}
	type mdIDPDescriptor struct {
		Services []mdSSO `xml:"SingleSignOnService"`
	}
	type mdEntity struct {
		IDP []mdIDPDescriptor `xml:"IDPSSODescriptor"`
	}
	var md mdEntity
	if err := xml.Unmarshal([]byte(metadataXML), &md); err != nil {
		return "", err
	}
	for _, desc := range md.IDP {
		for _, svc := range desc.Services {
			if strings.TrimSpace(svc.Location) != "" {
				return strings.TrimSpace(svc.Location), nil
			}
		}
	}
	return "", fmt.Errorf("no SingleSignOnService found")
}

// Ensures that SP certificate and private key files exist for a provider. Reads existing files if present, otherwise generates a new self-signed certificate and RSA key pair. Writes files to the specified paths (or defaults to the data directory). Returns the certificate PEM, key PEM, and any error encountered.
func (s *SAMLService) ensureSPCertKey(providerName, certPath, keyPath string) (string, string, error) {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" {
		certPath = filepath.Join(s.dataDir, providerName+".crt")
	}
	if keyPath == "" {
		keyPath = filepath.Join(s.dataDir, providerName+".key")
	}
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr == nil && keyErr == nil {
		return string(certPEM), string(keyPEM), nil
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generating SP private key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return "", "", fmt.Errorf("generating cert serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "AYB SAML SP " + providerName,
		},
		NotBefore:             s.now().Add(-5 * time.Minute),
		NotAfter:              s.now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", fmt.Errorf("creating SP certificate: %w", err)
	}
	certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		return "", "", fmt.Errorf("creating certificate directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return "", "", fmt.Errorf("creating key directory: %w", err)
	}
	if err := os.WriteFile(certPath, certBytes, 0o600); err != nil {
		return "", "", fmt.Errorf("writing SP certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, keyBytes, 0o600); err != nil {
		return "", "", fmt.Errorf("writing SP private key: %w", err)
	}
	return string(certBytes), string(keyBytes), nil
}

func xmlEscape(v string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(v)); err != nil {
		return v
	}
	return b.String()
}
