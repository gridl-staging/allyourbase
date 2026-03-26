// Package server saml_handler.go provides HTTP handlers for administering SAML authentication providers, supporting listing, creation, updating, and deletion operations with database persistence and in-memory service synchronization.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type adminSAMLProvider struct {
	Name             string            `json:"name"`
	EntityID         string            `json:"entity_id"`
	IDPMetadataXML   string            `json:"idp_metadata_xml,omitempty"`
	AttributeMapping map[string]string `json:"attribute_mapping,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type adminSAMLUpsertRequest struct {
	Name             string            `json:"name"`
	EntityID         string            `json:"entity_id"`
	IDPMetadataURL   string            `json:"idp_metadata_url"`
	IDPMetadataXML   string            `json:"idp_metadata_xml"`
	AttributeMapping map[string]string `json:"attribute_mapping"`
}

func (s *Server) requireSAMLAdminDeps(w http.ResponseWriter) bool {
	if s.authHandler == nil || s.samlSvc == nil {
		httputil.WriteError(w, http.StatusNotFound, "auth SAML is not enabled")
		return false
	}
	if s.pool == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "database is not configured")
		return false
	}
	return true
}

// handleAdminSAMLList handles HTTP GET requests to list all SAML authentication providers from the database, returning them as a JSON array ordered by name.
func (s *Server) handleAdminSAMLList(w http.ResponseWriter, r *http.Request) {
	if !s.requireSAMLAdminDeps(w) {
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT name, entity_id, idp_metadata, COALESCE(attribute_mapping, '{}'::jsonb), created_at, updated_at
		FROM _ayb_saml_providers
		ORDER BY name ASC`)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list SAML providers")
		return
	}
	defer rows.Close()

	out := make([]adminSAMLProvider, 0)
	for rows.Next() {
		var p adminSAMLProvider
		var attrRaw []byte
		if err := rows.Scan(&p.Name, &p.EntityID, &p.IDPMetadataXML, &attrRaw, &p.CreatedAt, &p.UpdatedAt); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to decode provider row")
			return
		}
		if len(attrRaw) > 0 {
			_ = json.Unmarshal(attrRaw, &p.AttributeMapping)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list SAML providers")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"providers": out})
}

// resolveSAMLMetadata returns SAML IdP metadata XML from the request, preferring raw XML if provided, otherwise fetching from the specified URL with a 10-second timeout. It returns an error if both sources are empty or if the URL fetch or parsing fails.
func resolveSAMLMetadata(ctx context.Context, req adminSAMLUpsertRequest) (string, error) {
	if xml := strings.TrimSpace(req.IDPMetadataXML); xml != "" {
		return xml, nil
	}
	u := strings.TrimSpace(req.IDPMetadataURL)
	if u == "" {
		return "", fmt.Errorf("idp_metadata_url or idp_metadata_xml is required")
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata endpoint returned %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	xml := strings.TrimSpace(string(b))
	if xml == "" {
		return "", fmt.Errorf("metadata document is empty")
	}
	return xml, nil
}

func validateSAMLProviderNameOrWriteError(w http.ResponseWriter, rawName string) (string, bool) {
	name := strings.TrimSpace(rawName)
	if err := auth.ValidateSAMLProviderName(name); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return name, true
}

// handleAdminSAMLCreate handles HTTP POST requests to create a new SAML authentication provider, validating the provider name and entity ID, resolving IdP metadata from either a provided XML or URL, updating the in-memory SAML service, and persisting the configuration to the database.
func (s *Server) handleAdminSAMLCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireSAMLAdminDeps(w) {
		return
	}
	var req adminSAMLUpsertRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	name, ok := validateSAMLProviderNameOrWriteError(w, req.Name)
	if !ok {
		return
	}
	req.Name = name
	req.EntityID = strings.TrimSpace(req.EntityID)
	if req.EntityID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "entity_id is required")
		return
	}
	metadataXML, err := resolveSAMLMetadata(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid IdP metadata: "+err.Error())
		return
	}
	cfg := config.SAMLProvider{
		Enabled:          true,
		Name:             req.Name,
		EntityID:         req.EntityID,
		IDPMetadataXML:   metadataXML,
		AttributeMapping: req.AttributeMapping,
	}
	if err := s.samlSvc.UpsertProvider(r.Context(), cfg); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	attrJSON, _ := json.Marshal(req.AttributeMapping)
	_, err = s.pool.Exec(r.Context(), `
		INSERT INTO _ayb_saml_providers (name, entity_id, idp_metadata, attribute_mapping)
		VALUES ($1, $2, $3, $4::jsonb)`,
		req.Name, req.EntityID, metadataXML, attrJSON)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httputil.WriteError(w, http.StatusConflict, "SAML provider already exists")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to save SAML provider")
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"name": req.Name})
}

// handleAdminSAMLUpdate handles HTTP PUT requests to update an existing SAML authentication provider by name, validating the entity ID, resolving updated IdP metadata, synchronizing changes to the in-memory SAML service, and persisting updates to the database.
func (s *Server) handleAdminSAMLUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireSAMLAdminDeps(w) {
		return
	}
	name, ok := validateSAMLProviderNameOrWriteError(w, chi.URLParam(r, "name"))
	if !ok {
		return
	}
	var req adminSAMLUpsertRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.EntityID) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "entity_id is required")
		return
	}
	metadataXML, err := resolveSAMLMetadata(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid IdP metadata: "+err.Error())
		return
	}
	cfg := config.SAMLProvider{
		Enabled:          true,
		Name:             name,
		EntityID:         strings.TrimSpace(req.EntityID),
		IDPMetadataXML:   metadataXML,
		AttributeMapping: req.AttributeMapping,
	}
	if err := s.samlSvc.UpsertProvider(r.Context(), cfg); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	attrJSON, _ := json.Marshal(req.AttributeMapping)
	tag, err := s.pool.Exec(r.Context(), `
		UPDATE _ayb_saml_providers
		SET entity_id = $2, idp_metadata = $3, attribute_mapping = $4::jsonb, updated_at = NOW()
		WHERE name = $1`,
		name, cfg.EntityID, metadataXML, attrJSON)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to update SAML provider")
		return
	}
	if tag.RowsAffected() == 0 {
		httputil.WriteError(w, http.StatusNotFound, "SAML provider not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"name": name})
}

// handleAdminSAMLDelete handles HTTP DELETE requests to remove a SAML authentication provider by name, deleting it from the database and the in-memory SAML service cache.
func (s *Server) handleAdminSAMLDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireSAMLAdminDeps(w) {
		return
	}
	name, ok := validateSAMLProviderNameOrWriteError(w, chi.URLParam(r, "name"))
	if !ok {
		return
	}
	tag, err := s.pool.Exec(r.Context(), `DELETE FROM _ayb_saml_providers WHERE name = $1`, name)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete SAML provider")
		return
	}
	if tag.RowsAffected() == 0 {
		httputil.WriteError(w, http.StatusNotFound, "SAML provider not found")
		return
	}
	s.samlSvc.DeleteProvider(name)
	w.WriteHeader(http.StatusNoContent)
}
