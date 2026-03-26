package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/vector"
)

// vectorIndexRequest is the request body for POST /admin/vector/indexes.
type vectorIndexRequest struct {
	Schema    string `json:"schema"`
	Table     string `json:"table"`
	Column    string `json:"column"`
	Method    string `json:"method"`
	Metric    string `json:"metric"`
	IndexName string `json:"index_name"`
	Lists     int    `json:"lists"`
}

// vectorIndexInfo is a single index entry in the list response.
type vectorIndexInfo struct {
	Name       string `json:"name"`
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	Method     string `json:"method"`
	Definition string `json:"definition"`
}

// handleAdminVectorIndexCreate handles POST /admin/vector/indexes.
func (s *Server) handleAdminVectorIndexCreate(w http.ResponseWriter, r *http.Request) {
	var req vectorIndexRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	// Validate required fields.
	var missing []string
	if req.Table == "" {
		missing = append(missing, "table")
	}
	if req.Column == "" {
		missing = append(missing, "column")
	}
	if req.Method == "" {
		missing = append(missing, "method")
	}
	if req.Metric == "" {
		missing = append(missing, "metric")
	}
	if len(missing) > 0 {
		httputil.WriteError(w, http.StatusBadRequest, "missing required fields: "+strings.Join(missing, ", "))
		return
	}

	// Default schema to public.
	if req.Schema == "" {
		req.Schema = "public"
	}

	// Validate the table/column exists and is a vector column via schema cache.
	sc := s.schema.Get()
	if sc != nil {
		tbl := sc.TableByName(req.Table)
		if tbl == nil {
			httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("table %q not found", req.Table))
			return
		}
		col := tbl.ColumnByName(req.Column)
		if col == nil {
			httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("column %q not found in table %q", req.Column, req.Table))
			return
		}
		if !col.IsVector {
			httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("column %q is not a vector column (type: %s)", req.Column, col.TypeName))
			return
		}
		// Use the table's actual schema.
		req.Schema = tbl.Schema
	}

	// Auto-generate index name if not provided.
	if req.IndexName == "" {
		req.IndexName = fmt.Sprintf("idx_%s_%s_%s", req.Table, req.Column, req.Method)
	}

	// Build the DDL.
	ddl, err := vector.BuildCreateIndexSQL(vector.IndexParams{
		Schema:    req.Schema,
		Table:     req.Table,
		Column:    req.Column,
		Method:    req.Method,
		Metric:    req.Metric,
		IndexName: req.IndexName,
		Lists:     req.Lists,
	})
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Execute DDL using the pool directly — CREATE INDEX CONCURRENTLY
	// cannot run inside a transaction.
	if s.pool == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "database pool not available")
		return
	}

	if _, err := s.pool.Exec(r.Context(), ddl); err != nil {
		errMsg := err.Error()
		// Detect concurrent index builds.
		if strings.Contains(errMsg, "already building") || strings.Contains(errMsg, "CONCURRENTLY") {
			httputil.WriteError(w, http.StatusConflict, "another index build is already in progress")
			return
		}
		s.logger.Error("vector index create error", "error", err, "ddl", ddl)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create index: "+errMsg)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"index_name": req.IndexName,
		"method":     req.Method,
		"metric":     req.Metric,
		"table":      req.Table,
		"column":     req.Column,
	})
}

// vectorIndexMethods are the pgvector index methods we report on.
var vectorIndexMethods = map[string]bool{
	"hnsw":    true,
	"ivfflat": true,
}

// handleAdminVectorIndexList handles GET /admin/vector/indexes.
func (s *Server) handleAdminVectorIndexList(w http.ResponseWriter, r *http.Request) {
	// Use schema cache to list vector indexes — avoids requiring a pool.
	sc := s.schema.Get()
	if sc == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"indexes": []vectorIndexInfo{}})
		return
	}

	var indexes []vectorIndexInfo
	for _, tbl := range sc.Tables {
		for _, idx := range tbl.Indexes {
			if vectorIndexMethods[idx.Method] {
				indexes = append(indexes, vectorIndexInfo{
					Name:       idx.Name,
					Schema:     tbl.Schema,
					Table:      tbl.Name,
					Method:     idx.Method,
					Definition: idx.Definition,
				})
			}
		}
	}

	if indexes == nil {
		indexes = []vectorIndexInfo{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"indexes": indexes})
}
