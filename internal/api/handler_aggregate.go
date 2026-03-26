// Package api Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/api/handler_aggregate.go.
package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/allyourbase/ayb/internal/schema"
)

// handleAggregate processes aggregate queries, branched from handleList.
func (h *Handler) handleAggregate(w http.ResponseWriter, r *http.Request, tbl *schema.Table, aggregateParam, groupParam string, fs filterSpatialResult) {
	q := r.URL.Query()
	queryStart := time.Now()

	// Aggregate mode is mutually exclusive with pagination, sort, fields, and expand.
	for _, param := range []string{"page", "perPage", "sort", "fields", "expand"} {
		if q.Get(param) != "" {
			writeError(w, http.StatusBadRequest, "aggregate cannot be combined with "+param)
			return
		}
	}

	// Parse aggregate expressions.
	exprs, err := parseAggregate(tbl, aggregateParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid aggregate: "+err.Error())
		return
	}

	// Parse group columns.
	groupCols, err := parseGroupColumns(tbl, groupParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group: "+err.Error())
		return
	}
	if err := h.validateAggregateEncryptionConstraints(tbl, exprs, groupCols); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	searchSQL, searchRank, searchArgs, ok := h.parseSearchParam(w, tbl, q, len(fs.filterArgs)+len(fs.spatialArgs)+1)
	if !ok {
		return
	}

	opts := listOpts{
		filterSQL:   fs.filterSQL,
		filterArgs:  fs.filterArgs,
		spatialSQL:  fs.spatialSQL,
		spatialArgs: fs.spatialArgs,
		searchSQL:   searchSQL,
		searchRank:  searchRank,
		searchArgs:  searchArgs,
	}

	aggQuery, aggArgs, err := buildAggregate(tbl, exprs, opts, groupCols)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	querier, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := querier.Query(r.Context(), aggQuery, aggArgs...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("aggregate query error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	results, err := scanRows(rows)
	rows.Close()
	if err != nil {
		done(err)
		h.logger.Error("aggregate scan error", "error", err, "table", tbl.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := done(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	durationMs := time.Since(queryStart).Milliseconds()
	w.Header().Set("X-Query-Duration-Ms", strconv.FormatInt(durationMs, 10))

	writeJSON(w, http.StatusOK, AggregateResponse{Results: results})
}

// TODO: Document Handler.validateAggregateEncryptionConstraints.
func (h *Handler) validateAggregateEncryptionConstraints(tbl *schema.Table, exprs []AggregateExpr, groupCols []string) error {
	if h.fieldEncryptor == nil {
		return nil
	}

	for _, expr := range exprs {
		if expr.Column != "" && h.fieldEncryptor.IsEncryptedColumn(tbl.Name, expr.Column) {
			return fmt.Errorf("invalid aggregate: encrypted column %q is not allowed", expr.Column)
		}
	}
	for _, groupCol := range groupCols {
		if h.fieldEncryptor.IsEncryptedColumn(tbl.Name, groupCol) {
			return fmt.Errorf("invalid group: encrypted column %q is not allowed", groupCol)
		}
	}

	return nil
}
