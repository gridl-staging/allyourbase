// Package api Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_6_test_verification_and_lint/allyourbase_dev/internal/api/export.go.
package api

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/jackc/pgx/v5"
)

// buildExportQuery builds a SELECT query for exporting all matching rows.
// Identical to buildList but without LIMIT/OFFSET and without a count query.
func buildExportQuery(tbl *schema.Table, opts listOpts, maxRows int) (string, []any) {
	cols := buildColumnList(tbl, opts.fields)
	ref := sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name)

	var whereParts []string
	var allArgs []any

	if opts.filterSQL != "" {
		whereParts = append(whereParts, opts.filterSQL)
		allArgs = append(allArgs, opts.filterArgs...)
	}

	if opts.searchSQL != "" {
		whereParts = append(whereParts, opts.searchSQL)
		allArgs = append(allArgs, opts.searchArgs...)
	}

	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	orderClause := ""
	if opts.sortSQL != "" {
		orderClause = " ORDER BY " + opts.sortSQL
	} else if opts.searchRank != "" {
		orderClause = " ORDER BY " + opts.searchRank + " DESC"
	}

	query := fmt.Sprintf("SELECT %s FROM %s%s%s", cols, ref, whereClause, orderClause)
	if maxRows > 0 {
		query += fmt.Sprintf(" LIMIT %d", maxRows)
	}
	return query, allArgs
}

// exportColumnNames returns column names in schema order, filtered by the
// requested fields if provided. This ensures deterministic CSV column ordering.
func exportColumnNames(tbl *schema.Table, fields []string) []string {
	if len(fields) == 0 {
		names := make([]string, len(tbl.Columns))
		for i, col := range tbl.Columns {
			names[i] = col.Name
		}
		return names
	}

	requested := make(map[string]bool, len(fields))
	for _, f := range fields {
		requested[f] = true
	}

	var names []string
	for _, col := range tbl.Columns {
		if requested[col.Name] {
			names = append(names, col.Name)
		}
	}
	return names
}

// formatCSVValue converts a value to its CSV string representation.
func formatCSVValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case time.Time:
		return val.Format(time.RFC3339)
	case []byte:
		return base64.StdEncoding.EncodeToString(val)
	default:
		return fmt.Sprint(val)
	}
}

// parseExportParams resolves the table and parses filter/search/sort/fields
// for export handlers. Returns false if an error response was already written.
func (h *Handler) parseExportParams(w http.ResponseWriter, r *http.Request) (*schema.Table, listOpts, bool) {
	tbl := h.resolveTable(w, r)
	if tbl == nil {
		return nil, listOpts{}, false
	}

	q := r.URL.Query()

	fields := parseFields(r)
	sortSQL := parseSortSQL(tbl, q.Get("sort"))

	fs, ok := h.parseFilterAndSearch(w, tbl, q)
	if !ok {
		return nil, listOpts{}, false
	}

	opts := listOpts{
		fields:     fields,
		sortSQL:    sortSQL,
		filterSQL:  fs.filterSQL,
		filterArgs: fs.filterArgs,
		searchSQL:  fs.searchSQL,
		searchRank: fs.searchRank,
		searchArgs: fs.searchArgs,
	}
	return tbl, opts, true
}

// exportRLSRequest ensures that export queries on RLS-enabled tables go through
// the RLS path even when no auth claims are present. Without this, withRLS would
// return the raw pool and bypass row-level security entirely.
func exportRLSRequest(r *http.Request, tbl *schema.Table) *http.Request {
	if auth.ClaimsFromContext(r.Context()) == nil && tbl.RLSEnabled {
		return r.WithContext(auth.ContextWithClaims(r.Context(), &auth.Claims{}))
	}
	return r
}

// TODO: Document Handler.queryExportRows.
func (h *Handler) queryExportRows(w http.ResponseWriter, r *http.Request) (*schema.Table, listOpts, pgx.Rows, func(error) error, bool) {
	tbl, opts, ok := h.parseExportParams(w, r)
	if !ok {
		return nil, listOpts{}, nil, nil, false
	}

	apiCfg := h.effectiveAPIConfig()
	query, args := buildExportQuery(tbl, opts, apiCfg.ExportMaxRows)

	rlsReq := exportRLSRequest(r, tbl)
	querier, done, err := h.withRLS(rlsReq)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, listOpts{}, nil, nil, false
	}

	rows, err := querier.Query(rlsReq.Context(), query, args...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("export query error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return nil, listOpts{}, nil, nil, false
	}

	return tbl, opts, rows, done, true
}

func (h *Handler) scanExportRecord(rows pgx.Rows, tbl *schema.Table) (map[string]any, error) {
	record, err := scanCurrentRow(rows)
	if err != nil {
		return nil, err
	}
	if h.fieldEncryptor == nil {
		return record, nil
	}
	if err := h.fieldEncryptor.DecryptRecord(tbl.Name, record); err != nil {
		return nil, err
	}
	return record, nil
}

// handleExportCSV streams all matching rows as CSV.
func (h *Handler) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	tbl, opts, rows, done, ok := h.queryExportRows(w, r)
	if !ok {
		return
	}

	// Determine column names for header and row ordering.
	colNames := exportColumnNames(tbl, opts.fields)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, tbl.Name))
	w.WriteHeader(http.StatusOK)

	csvWriter := csv.NewWriter(w)
	if err := csvWriter.Write(colNames); err != nil {
		rows.Close()
		_ = done(err)
		h.logger.Error("export csv write header error", "error", err, "table", tbl.Name)
		return
	}

	for rows.Next() {
		record, err := h.scanExportRecord(rows, tbl)
		if err != nil {
			rows.Close()
			done(err)
			h.logger.Error("export csv row error", "error", err, "table", tbl.Name)
			return
		}

		row := make([]string, len(colNames))
		for i, col := range colNames {
			row[i] = formatCSVValue(record[col])
		}
		if err := csvWriter.Write(row); err != nil {
			rows.Close()
			_ = done(err)
			h.logger.Error("export csv write row error", "error", err, "table", tbl.Name)
			return
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		_ = done(err)
		h.logger.Error("export csv rows error", "error", err, "table", tbl.Name)
		return
	}
	rows.Close()

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		h.logger.Error("export csv flush error", "error", err, "table", tbl.Name)
	}

	if err := done(nil); err != nil {
		h.logger.Error("export csv commit error", "error", err, "table", tbl.Name)
	}
}

// TODO: Document Handler.handleExportJSON.
func (h *Handler) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	tbl, _, rows, done, ok := h.queryExportRows(w, r)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.json"`, tbl.Name))
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	_, _ = w.Write([]byte("["))
	first := true

	for rows.Next() {
		record, err := h.scanExportRecord(rows, tbl)
		if err != nil {
			rows.Close()
			done(err)
			h.logger.Error("export json row error", "error", err, "table", tbl.Name)
			return
		}

		if !first {
			_, _ = w.Write([]byte(","))
		}
		if err := enc.Encode(record); err != nil {
			rows.Close()
			_ = done(err)
			h.logger.Error("export json encode error", "error", err, "table", tbl.Name)
			return
		}
		first = false
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		_ = done(err)
		h.logger.Error("export json rows error", "error", err, "table", tbl.Name)
		return
	}
	rows.Close()

	_, _ = w.Write([]byte("]\n"))

	if err := done(nil); err != nil {
		h.logger.Error("export json commit error", "error", err, "table", tbl.Name)
	}
}
