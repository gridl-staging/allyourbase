// Package auth oauth_clients.go provides OAuth 2.0 client management including registration, validation, credential handling, and database CRUD operations.
package auth

import (
	"errors"
	"time"
)

// OAuth client ID/secret prefixes.
const (
	OAuthClientIDPrefix     = "ayb_cid_"
	OAuthClientSecretPrefix = "ayb_cs_"
)

// OAuth token prefixes.
const (
	OAuthAccessTokenPrefix  = "ayb_at_"
	OAuthRefreshTokenPrefix = "ayb_rt_"
)

// OAuth client types.
const (
	OAuthClientTypeConfidential = "confidential"
	OAuthClientTypePublic       = "public"
)

// OAuth error codes per RFC 6749 §5.2.
const (
	OAuthErrInvalidRequest       = "invalid_request"
	OAuthErrInvalidClient        = "invalid_client"
	OAuthErrInvalidGrant         = "invalid_grant"
	OAuthErrUnauthorizedClient   = "unauthorized_client"
	OAuthErrUnsupportedGrantType = "unsupported_grant_type"
	OAuthErrInvalidScope         = "invalid_scope"
	OAuthErrAccessDenied         = "access_denied"
)

// OAuthClient represents a registered OAuth 2.0 client.
// OAuthClient represents a registered OAuth 2.0 client with configuration for redirect URIs, scopes, and client type (confidential or public). It includes creation and revocation timestamps and computed statistics on active tokens and total grants.
type OAuthClient struct {
	ID                      string     `json:"id"`
	AppID                   string     `json:"appId"`
	ClientID                string     `json:"clientId"`
	Name                    string     `json:"name"`
	RedirectURIs            []string   `json:"redirectUris"`
	Scopes                  []string   `json:"scopes"`
	ClientType              string     `json:"clientType"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
	RevokedAt               *time.Time `json:"revokedAt"`
	ActiveAccessTokenCount  int        `json:"activeAccessTokenCount"`
	ActiveRefreshTokenCount int        `json:"activeRefreshTokenCount"`
	TotalGrants             int        `json:"totalGrants"`
	LastTokenIssuedAt       *time.Time `json:"lastTokenIssuedAt"`
}

// OAuthClientListResult is a paginated list of OAuth clients.
type OAuthClientListResult struct {
	Items      []OAuthClient `json:"items"`
	Page       int           `json:"page"`
	PerPage    int           `json:"perPage"`
	TotalItems int           `json:"totalItems"`
	TotalPages int           `json:"totalPages"`
}

// Sentinel errors for OAuth clients.
var (
	ErrOAuthClientNotFound            = errors.New("oauth client not found")
	ErrOAuthClientRevoked             = errors.New("oauth client has been revoked")
	ErrOAuthClientNameRequired        = errors.New("oauth client name is required")
	ErrOAuthAppRequired               = errors.New("app_id is required for oauth client")
	ErrOAuthClientPublicSecretRotator = errors.New("cannot regenerate secret for public client")
)
