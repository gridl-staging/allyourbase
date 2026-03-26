// Package server provides HTTP handlers for managing foreign data wrappers, supporting operations to create, list, import, and drop foreign servers and tables.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/fdw"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// fdwAdmin is the interface needed by FDW admin handlers.
// fdw.Service satisfies this.
type fdwAdmin interface {
	CreateServer(ctx context.Context, opts fdw.CreateServerOpts) error
	ListServers(ctx context.Context) ([]fdw.ForeignServer, error)
	DropServer(ctx context.Context, name string, cascade bool) error
	ImportTables(ctx context.Context, serverName string, opts fdw.ImportOpts) ([]fdw.ForeignTable, error)
	ListForeignTables(ctx context.Context) ([]fdw.ForeignTable, error)
	DropForeignTable(ctx context.Context, schemaName, tableName string) error
}

// handleAdminFDWCreateServer creates a new foreign server from JSON request parameters, returning the server's name and type on success.
func (s *Server) handleAdminFDWCreateServer(w http.ResponseWriter, r *http.Request) {
	if s.fdwService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "fdw management not configured")
		return
	}

	var opts fdw.CreateServerOpts
	if !httputil.DecodeJSON(w, r, &opts) {
		return
	}

	if err := s.fdwService.CreateServer(r.Context(), opts); err != nil {
		if isFDWValidationError(err) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"name": opts.Name,
		"type": opts.FDWType,
	})
}

// handleAdminFDWListServers responds with a list of all foreign servers configured in the database, returning an empty list if FDW management is not configured.
func (s *Server) handleAdminFDWListServers(w http.ResponseWriter, r *http.Request) {
	if s.fdwService == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"servers": []fdw.ForeignServer{}})
		return
	}

	servers, err := s.fdwService.ListServers(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if servers == nil {
		servers = []fdw.ForeignServer{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"servers": servers})
}

// handleAdminFDWDropServer removes a foreign server by name, accepting an optional cascade query parameter to drop dependent objects, and triggers a schema reload.
func (s *Server) handleAdminFDWDropServer(w http.ResponseWriter, r *http.Request) {
	if s.fdwService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "fdw management not configured")
		return
	}

	name := chi.URLParam(r, "name")
	cascade := false
	if raw := strings.TrimSpace(r.URL.Query().Get("cascade")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid cascade value")
			return
		}
		cascade = parsed
	}

	if err := s.fdwService.DropServer(r.Context(), name, cascade); err != nil {
		if isFDWValidationError(err) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.reloadSchemaAfterFDWDDL(r.Context(), "drop fdw server")
	w.WriteHeader(http.StatusNoContent)
}

// handleAdminFDWImportTables imports tables from a specified foreign server using options provided in the request body, triggers a schema reload, and responds with the imported foreign tables.
func (s *Server) handleAdminFDWImportTables(w http.ResponseWriter, r *http.Request) {
	if s.fdwService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "fdw management not configured")
		return
	}

	name := chi.URLParam(r, "name")
	var opts fdw.ImportOpts
	if !httputil.DecodeJSON(w, r, &opts) {
		return
	}

	tables, err := s.fdwService.ImportTables(r.Context(), name, opts)
	if err != nil {
		if isFDWValidationError(err) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tables == nil {
		tables = []fdw.ForeignTable{}
	}

	s.reloadSchemaAfterFDWDDL(r.Context(), "import fdw tables")
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"tables": tables})
}

// handleAdminFDWListTables responds with a list of all imported foreign tables, returning an empty list if FDW management is not configured.
func (s *Server) handleAdminFDWListTables(w http.ResponseWriter, r *http.Request) {
	if s.fdwService == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"tables": []fdw.ForeignTable{}})
		return
	}

	tables, err := s.fdwService.ListForeignTables(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tables == nil {
		tables = []fdw.ForeignTable{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"tables": tables})
}

// handleAdminFDWDropTable removes a foreign table from the specified schema and triggers a schema reload.
func (s *Server) handleAdminFDWDropTable(w http.ResponseWriter, r *http.Request) {
	if s.fdwService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "fdw management not configured")
		return
	}

	schemaName := chi.URLParam(r, "schema")
	tableName := chi.URLParam(r, "table")

	if err := s.fdwService.DropForeignTable(r.Context(), schemaName, tableName); err != nil {
		if isFDWValidationError(err) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.reloadSchemaAfterFDWDDL(r.Context(), "drop foreign table")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reloadSchemaAfterFDWDDL(ctx context.Context, action string) {
	if s.schema == nil {
		return
	}
	if err := s.schema.ReloadWait(ctx); err != nil {
		slog.Default().Warn("schema reload after fdw ddl failed", "action", action, "error", err)
	}
}

func isFDWValidationError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid") ||
		strings.Contains(msg, "unsupported fdw type") ||
		strings.Contains(msg, "is required") ||
		strings.Contains(msg, "must not be empty") ||
		strings.Contains(msg, "exceeds postgresql max length")
}
