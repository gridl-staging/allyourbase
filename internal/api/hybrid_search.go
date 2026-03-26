// Package api implements hybrid search by combining full-text search and vector similarity with reciprocal rank fusion to merge results.
package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/vector"
)

const (
	defaultRRFConstant        = 60
	hybridOverFetchMultiplier = 3
	hybridOverFetchCap        = 500
)

// constructs a SQL query for full-text search that ranks rows by relevance, optionally combining with a filter condition, and limits results to the specified count.
func buildFTSHybridQuery(tbl *schema.Table, searchTerm string, limit int, filterSQL string, filterArgs []any) (string, []any, error) {
	searchSQL, rankSQL, searchArgs, err := buildSearchSQL(tbl, searchTerm, len(filterArgs)+1)
	if err != nil {
		return "", nil, err
	}

	whereParts := make([]string, 0, 2)
	if filterSQL != "" {
		whereParts = append(whereParts, filterSQL)
	}
	whereParts = append(whereParts, searchSQL)

	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	args := append(append([]any{}, filterArgs...), searchArgs...)
	limitArg := len(args) + 1
	sql := fmt.Sprintf(
		"SELECT %s, %s AS _fts_rank FROM %s%s ORDER BY _fts_rank DESC LIMIT $%d",
		buildColumnList(tbl, nil),
		rankSQL,
		sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name),
		whereClause,
		limitArg,
	)
	args = append(args, limit)
	return sql, args, nil
}

func buildVectorHybridQuery(tbl *schema.Table, col *schema.Column, queryVec []float64, metric string, limit int, filterSQL string, filterArgs []any) (string, []any, error) {
	return vector.BuildNearestQuery(vector.NearestParams{
		Table:        tbl,
		VectorColumn: col.Name,
		QueryVector:  queryVec,
		Metric:       metric,
		Limit:        limit,
		FilterSQL:    filterSQL,
		FilterArgs:   filterArgs,
	})
}

// executes a full-text search query with row-level security applied and returns the matching rows as a slice of maps keyed by column name.
func (h *Handler) executeFTSQuery(r *http.Request, tbl *schema.Table, searchTerm string, limit int, filterSQL string, filterArgs []any) ([]map[string]any, error) {
	sql, args, err := buildFTSHybridQuery(tbl, searchTerm, limit, filterSQL, filterArgs)
	if err != nil {
		return nil, err
	}

	querier, done, err := h.withRLS(r)
	if err != nil {
		return nil, err
	}

	rows, err := querier.Query(r.Context(), sql, args...)
	if err != nil {
		_ = done(err)
		return nil, err
	}

	items, err := scanRows(rows)
	rows.Close()
	if err != nil {
		_ = done(err)
		return nil, err
	}
	if err := done(nil); err != nil {
		return nil, err
	}
	return items, nil
}

// executes a vector similarity query with row-level security applied and returns the matching rows as a slice of maps keyed by column name.
func (h *Handler) executeVectorQuery(r *http.Request, tbl *schema.Table, col *schema.Column, queryVec []float64, metric string, limit int, filterSQL string, filterArgs []any) ([]map[string]any, error) {
	sql, args, err := buildVectorHybridQuery(tbl, col, queryVec, metric, limit, filterSQL, filterArgs)
	if err != nil {
		return nil, err
	}

	querier, done, err := h.withRLS(r)
	if err != nil {
		return nil, err
	}

	rows, err := querier.Query(r.Context(), sql, args...)
	if err != nil {
		_ = done(err)
		return nil, err
	}

	items, err := scanRows(rows)
	rows.Close()
	if err != nil {
		_ = done(err)
		return nil, err
	}
	if err := done(nil); err != nil {
		return nil, err
	}
	return items, nil
}

// is the HTTP handler for hybrid search that embeds the search term, executes both full-text and vector similarity queries with over-fetching, merges results using reciprocal rank fusion, and writes a paginated JSON response.
func (h *Handler) handleHybridSearch(w http.ResponseWriter, r *http.Request, tbl *schema.Table, searchTerm, vectorColName, distanceParam string, perPage int, filterSQL string, filterArgs []any) {
	if h.embedFn == nil {
		writeError(w, http.StatusNotImplemented, "hybrid search is not configured — no embedding provider available")
		return
	}

	sc := h.schema.Get()
	if sc == nil || !sc.HasPgVector {
		writeError(w, http.StatusBadRequest, "hybrid search requires the pgvector extension")
		return
	}

	col, err := findVectorColumn(tbl, vectorColName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(textColumns(tbl)) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("table %q has no text columns to search", tbl.Name))
		return
	}

	metric, err := resolveDistanceMetric(distanceParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	embeddings, err := h.embedFn(r.Context(), []string{searchTerm})
	if err != nil {
		mapEmbeddingError(w, err)
		return
	}
	if len(embeddings) == 0 {
		writeError(w, http.StatusBadGateway, "embedding provider returned no embedding")
		return
	}
	queryVec := embeddings[0]
	if col.VectorDim > 0 && len(queryVec) != col.VectorDim {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("embedding dimension mismatch: provider returned %d dimensions, column %q expects %d",
				len(queryVec), col.Name, col.VectorDim))
		return
	}

	fetchLimit := perPage * hybridOverFetchMultiplier
	if fetchLimit > hybridOverFetchCap {
		fetchLimit = hybridOverFetchCap
	}

	ftsResults, err := h.executeFTSQuery(r, tbl, searchTerm, fetchLimit, filterSQL, filterArgs)
	if err != nil {
		if !mapPGError(w, err) {
			h.logger.Error("hybrid FTS query error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	vectorResults, err := h.executeVectorQuery(r, tbl, col, queryVec, metric, fetchLimit, filterSQL, filterArgs)
	if err != nil {
		if !mapPGError(w, err) {
			h.logger.Error("hybrid vector query error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	merged := rrfMerge(ftsResults, vectorResults, tbl.PrimaryKey, defaultRRFConstant)
	if len(merged) > perPage {
		merged = merged[:perPage]
	}

	writeJSON(w, http.StatusOK, ListResponse{
		Page:       1,
		PerPage:    perPage,
		TotalItems: len(merged),
		TotalPages: 1,
		Items:      merged,
	})
}
