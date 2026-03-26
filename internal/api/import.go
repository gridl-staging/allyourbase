// Package api Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_6_test_verification_and_lint/allyourbase_dev/internal/api/import.go.
package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// TODO: Document Handler.handleImport.
func (h *Handler) handleImport(w http.ResponseWriter, r *http.Request) {
	tbl := h.resolveTable(w, r)
	if tbl == nil {
		return
	}
	if !requireWriteScope(w, r) {
		return
	}
	if !requireWritable(w, tbl) {
		return
	}
	if !requirePK(w, tbl) {
		return
	}

	q := r.URL.Query()

	mode, err := parseImportMode(q.Get("mode"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	onConflict := q.Get("on_conflict")
	if err := validateImportOnConflict(onConflict); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	contentType, err := importContentType(r.Header.Get("Content-Type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Enforce request size limit.
	apiCfg := h.effectiveAPIConfig()
	r.Body = http.MaxBytesReader(w, r.Body, int64(apiCfg.ImportMaxSizeMB)<<20)

	// Build column map for validation.
	colMap := buildColumnMap(tbl)

	if contentType == "text/csv" {
		h.handleImportCSV(w, r, tbl, mode, onConflict, colMap, apiCfg.ImportMaxRows)
	} else {
		h.handleImportJSON(w, r, tbl, mode, onConflict, colMap, apiCfg.ImportMaxRows)
	}
}

// TODO: Document Handler.handleImportCSV.
func (h *Handler) handleImportCSV(
	w http.ResponseWriter, r *http.Request,
	tbl *schema.Table, mode, onConflict string,
	colMap map[string]bool,
	maxRows int,
) {
	headers, validCols, csvReader, err := parseCSVHeaders(tbl, r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sqlStr := buildImportSQL(tbl, validCols, onConflict)

	// Continue using the same csv.Reader returned by parseCSVHeaders
	// (creating a new one would lose buffered data).
	csvReader.FieldsPerRecord = len(headers)

	var records []map[string]any
	var parseErrors []ImportRowError
	for rowNum := 1; ; rowNum++ {
		fields, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if rowNum > maxRows {
			writeError(w, http.StatusBadRequest, importRowLimitMessage(maxRows))
			return
		}
		if err != nil {
			// MaxBytesReader triggers this
			if isMaxBytesError(err) {
				writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			if mode == "full" {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("CSV parse error at row %d: %s", rowNum, err.Error()))
				return
			}
			parseErrors = append(parseErrors, ImportRowError{Row: rowNum, Message: "CSV parse error: " + err.Error()})
			continue
		}

		record := make(map[string]any, len(validCols))
		for i, header := range headers {
			if colMap[header] {
				record[header] = fields[i]
			}
		}
		records = append(records, record)
	}

	h.executeImport(w, r, tbl, sqlStr, validCols, records, parseErrors, mode, onConflict)
}

// TODO: Document Handler.handleImportJSON.
func (h *Handler) handleImportJSON(
	w http.ResponseWriter, r *http.Request,
	tbl *schema.Table, mode, onConflict string,
	colMap map[string]bool,
	maxRows int,
) {
	// Use sorted column list from schema for deterministic SQL.
	allCols := schemaColumnNames(tbl)

	var records []map[string]any
	var parseErrors []ImportRowError

	err := streamJSONRows(r.Body, colMap, maxRows, func(_ int, record map[string]any) error {
		records = append(records, record)
		return nil
	}, &parseErrors)
	if err != nil {
		if isMaxBytesError(err) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sqlStr := buildImportSQL(tbl, allCols, onConflict)
	h.executeImport(w, r, tbl, sqlStr, allCols, records, parseErrors, mode, onConflict)
}

// TODO: Document Handler.executeImport.
func (h *Handler) executeImport(
	w http.ResponseWriter, r *http.Request,
	tbl *schema.Table, sqlStr string, cols []string,
	records []map[string]any, parseErrors []ImportRowError,
	mode, onConflict string,
) {
	// In full mode, any parse errors are a hard failure.
	if mode == "full" && len(parseErrors) > 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("import failed: %d row(s) had parse errors", len(parseErrors)))
		return
	}

	querier, done, err := h.importQuerier(r, tbl, mode)
	if err != nil {
		h.logger.Error("import querier setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := ImportResponse{
		Processed: len(records) + len(parseErrors),
		Failed:    len(parseErrors),
		Errors:    parseErrors,
	}

	for i, record := range records {
		rowNum := i + 1 // 1-indexed

		// Encrypt if needed.
		if h.fieldEncryptor != nil {
			if err := h.fieldEncryptor.EncryptRecord(tbl.Name, record); err != nil {
				if mode == "full" {
					if doneErr := done(fmt.Errorf("encryption error: %w", err)); doneErr != nil {
						h.logger.Error("tx finalize error", "error", doneErr)
					}
					writeError(w, http.StatusInternalServerError, "internal error")
					return
				}
				resp.Failed++
				resp.Errors = append(resp.Errors, ImportRowError{Row: rowNum, Message: "encryption error"})
				continue
			}
		}

		// Build args in column order.
		args := make([]any, len(cols))
		for j, col := range cols {
			args[j] = record[col]
		}

		tag, execErr := querier.Exec(r.Context(), sqlStr, args...)
		if execErr != nil {
			if mode == "full" {
				if doneErr := done(execErr); doneErr != nil {
					h.logger.Error("tx finalize error", "error", doneErr)
				}
				msg := mapImportPGError(execErr)
				writeJSON(w, http.StatusConflict, ImportResponse{
					Processed: len(records),
					Failed:    1,
					Errors:    []ImportRowError{{Row: rowNum, Message: msg}},
				})
				return
			}
			resp.Failed++
			resp.Errors = append(resp.Errors, ImportRowError{Row: rowNum, Message: mapImportPGError(execErr)})
			continue
		}

		affected := tag.RowsAffected()
		if onConflict == "skip" && affected == 0 {
			resp.Skipped++
		} else {
			resp.Inserted++
		}
	}

	if err := done(nil); err != nil {
		h.logger.Error("tx commit error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if len(resp.Errors) == 0 {
		resp.Errors = nil
	}
	writeJSON(w, http.StatusOK, resp)
}

// TODO: Document Handler.importQuerier.
func (h *Handler) importQuerier(r *http.Request, tbl *schema.Table, mode string) (Querier, func(error) error, error) {
	r = exportRLSRequest(r, tbl)

	// Full-mode imports must be atomic even when no claims context is present.
	// withRLS() intentionally returns the pool (autocommit) for claims-less requests,
	// so enforce an explicit transaction for this path.
	if mode == "full" && auth.ClaimsFromContext(r.Context()) == nil {
		tx, err := h.beginTx(r.Context())
		if err != nil {
			return nil, nil, err
		}
		done := func(queryErr error) error { return finalizeTx(r.Context(), tx, queryErr, h.logger) }
		return tx, done, nil
	}

	return h.withRLS(r)
}

func parseImportMode(mode string) (string, error) {
	if mode == "" {
		return "full", nil
	}
	if mode != "full" && mode != "partial" {
		return "", fmt.Errorf("invalid mode: must be full or partial")
	}
	return mode, nil
}

func validateImportOnConflict(onConflict string) error {
	if onConflict != "" && onConflict != "skip" && onConflict != "update" {
		return fmt.Errorf("invalid on_conflict: must be skip or update")
	}
	return nil
}

func importContentType(contentType string) (string, error) {
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	if contentType != "text/csv" && contentType != "application/json" {
		return "", fmt.Errorf("unsupported content type: expected text/csv or application/json")
	}
	return contentType, nil
}

func importRowLimitMessage(maxRows int) string {
	return fmt.Sprintf("import row limit exceeded (max %d)", maxRows)
}

func buildImportSQL(tbl *schema.Table, cols []string, onConflict string) string {
	switch onConflict {
	case "skip":
		return buildImportSkipSQL(tbl, cols)
	case "update":
		return buildImportUpdateSQL(tbl, cols)
	default:
		return buildImportInsertSQL(tbl, cols)
	}
}

// --- SQL Builders ---

// buildImportInsertSQL builds a plain INSERT statement.
func buildImportInsertSQL(tbl *schema.Table, cols []string) string {
	quoted := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, col := range cols {
		quoted[i] = sqlutil.QuoteIdent(col)
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name),
		strings.Join(quoted, ", "),
		strings.Join(placeholders, ", "),
	)
}

// buildImportSkipSQL builds an INSERT ... ON CONFLICT DO NOTHING statement.
func buildImportSkipSQL(tbl *schema.Table, cols []string) string {
	base := buildImportInsertSQL(tbl, cols)
	pkCols := make([]string, len(tbl.PrimaryKey))
	for i, pk := range tbl.PrimaryKey {
		pkCols[i] = sqlutil.QuoteIdent(pk)
	}
	return fmt.Sprintf("%s ON CONFLICT (%s) DO NOTHING",
		base, strings.Join(pkCols, ", "))
}

// buildImportUpdateSQL builds an INSERT ... ON CONFLICT DO UPDATE SET ... statement.
// PK columns are excluded from the SET clause.
func buildImportUpdateSQL(tbl *schema.Table, cols []string) string {
	base := buildImportInsertSQL(tbl, cols)
	pkCols := make([]string, len(tbl.PrimaryKey))
	pkSet := make(map[string]bool, len(tbl.PrimaryKey))
	for i, pk := range tbl.PrimaryKey {
		pkCols[i] = sqlutil.QuoteIdent(pk)
		pkSet[pk] = true
	}

	var setClauses []string
	for _, col := range cols {
		if pkSet[col] {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = EXCLUDED.%s", sqlutil.QuoteIdent(col), sqlutil.QuoteIdent(col)))
	}

	if len(setClauses) == 0 {
		// All columns are PKs; fall back to DO NOTHING.
		return fmt.Sprintf("%s ON CONFLICT (%s) DO NOTHING",
			base, strings.Join(pkCols, ", "))
	}

	return fmt.Sprintf("%s ON CONFLICT (%s) DO UPDATE SET %s",
		base, strings.Join(pkCols, ", "), strings.Join(setClauses, ", "))
}

// --- Column Helpers ---

// buildColumnMap returns a set of valid column names for the table.
func buildColumnMap(tbl *schema.Table) map[string]bool {
	m := make(map[string]bool, len(tbl.Columns))
	for _, col := range tbl.Columns {
		m[col.Name] = true
	}
	return m
}

// schemaColumnNames returns column names in schema order.
func schemaColumnNames(tbl *schema.Table) []string {
	names := make([]string, len(tbl.Columns))
	for i, col := range tbl.Columns {
		names[i] = col.Name
	}
	return names
}

// filterRecordColumns returns a copy of record with only known columns.
func filterRecordColumns(record map[string]any, colMap map[string]bool) map[string]any {
	filtered := make(map[string]any, len(record))
	for k, v := range record {
		if colMap[k] {
			filtered[k] = v
		}
	}
	return filtered
}

// --- CSV Parsing ---

// parseCSVHeaders reads the CSV header row, validates columns against the schema,
// and returns the raw headers, the valid column names, and the csv.Reader for continued reading.
// The returned csv.Reader must be used for subsequent row reads (not a new reader on the
// same body) because csv.Reader internally buffers via bufio.Reader.
func parseCSVHeaders(tbl *schema.Table, body io.Reader) ([]string, []string, *csv.Reader, error) {
	csvReader := csv.NewReader(body)
	csvReader.LazyQuotes = true

	headers, err := csvReader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil, nil, fmt.Errorf("empty CSV: no header row found")
		}
		return nil, nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	if len(headers) == 0 {
		return nil, nil, nil, fmt.Errorf("empty CSV: no header columns")
	}

	// Strip UTF-8 BOM from first cell.
	headers[0] = strings.TrimPrefix(headers[0], "\xEF\xBB\xBF")

	// Trim whitespace and check for duplicates.
	seen := make(map[string]bool, len(headers))
	for i, h := range headers {
		headers[i] = strings.TrimSpace(h)
	}
	for _, h := range headers {
		if h == "" {
			continue
		}
		if seen[h] {
			return nil, nil, nil, fmt.Errorf("duplicate CSV header: %s", h)
		}
		seen[h] = true
	}

	// Build valid column list.
	colMap := buildColumnMap(tbl)
	var validCols []string
	for _, h := range headers {
		if colMap[h] {
			validCols = append(validCols, h)
		}
	}

	if len(validCols) == 0 {
		return nil, nil, nil, fmt.Errorf("no recognized columns in CSV headers")
	}

	return headers, validCols, csvReader, nil
}

// --- JSON Streaming ---

// streamJSONRows reads a JSON array of objects from r, calling onRow for each valid record.
// Non-object items are recorded as errors in errs. Returns a non-nil error for structural
// problems (not an array, malformed JSON, etc.).
func streamJSONRows(r io.Reader, colMap map[string]bool, maxRows int, onRow func(row int, record map[string]any) error, errs *[]ImportRowError) error {
	dec := json.NewDecoder(r)

	// Expect opening bracket.
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return fmt.Errorf("expected JSON array, got %T", tok)
	}

	rowNum := 0
	for dec.More() {
		rowNum++
		if rowNum > maxRows {
			return fmt.Errorf("import row limit exceeded (max %d)", maxRows)
		}

		// Try to decode each element as a JSON object.
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("JSON decode error at row %d: %w", rowNum, err)
		}

		var record map[string]any
		if err := json.Unmarshal(raw, &record); err != nil {
			// Not an object — could be a string, number, array, etc.
			*errs = append(*errs, ImportRowError{Row: rowNum, Message: "expected JSON object"})
			continue
		}

		// Filter to known columns.
		filtered := filterRecordColumns(record, colMap)
		if err := onRow(rowNum, filtered); err != nil {
			return err
		}
	}

	// Expect closing bracket.
	_, err = dec.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return nil
}

// --- Error Helpers ---

// mapImportPGError converts a PostgreSQL error to a user-friendly message for import responses.
func mapImportPGError(err error) string {
	if err == nil {
		return ""
	}
	// Try to extract a PgError for a cleaner message.
	var pgErr interface{ Error() string }
	if errors.As(err, &pgErr) {
		return pgErr.Error()
	}
	return err.Error()
}

// isMaxBytesError checks if an error is from http.MaxBytesReader.
func isMaxBytesError(err error) bool {
	if err == nil {
		return false
	}
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError)
}
