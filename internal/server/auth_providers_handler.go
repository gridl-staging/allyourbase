// Package server Handles HTTP endpoints for managing OAuth and OIDC authentication provider configurations at runtime, including listing, updating, deleting, and testing provider connectivity.
package server

import (
	"errors"

	"github.com/allyourbase/ayb/internal/auth"
)

type authProviderListResponse struct {
	Providers []auth.OAuthProviderInfo `json:"providers"`
}

type updateAuthProviderRequest struct {
	Enabled             *bool     `json:"enabled"`
	ClientID            *string   `json:"client_id"`
	ClientSecret        *string   `json:"client_secret"`
	StoreProviderTokens *bool     `json:"store_provider_tokens"`
	TenantID            *string   `json:"tenant_id"`
	TeamID              *string   `json:"team_id"`
	KeyID               *string   `json:"key_id"`
	PrivateKey          *string   `json:"private_key"`
	FacebookAPIVersion  *string   `json:"facebook_api_version"`
	GitLabBaseURL       *string   `json:"gitlab_base_url"`
	IssuerURL           *string   `json:"issuer_url"`
	Scopes              *[]string `json:"scopes"`
	DisplayName         *string   `json:"display_name"`
}

var errAuthProviderNotFound = errors.New("auth provider not found")
