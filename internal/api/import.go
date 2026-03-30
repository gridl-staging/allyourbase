package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/schema"
)

// handleImport handles POST /collections/{table}/import, routing to CSV or JSON parsing based on Content-Type and applying mode/on_conflict semantics.
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

// handleImportCSV parses a CSV request body, validates headers against the table schema, and delegates row insertion to executeImport.
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

// handleImportJSON parses a JSON array request body, filters each object to known columns, and delegates row insertion to executeImport.
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

// executeImport inserts parsed records into the table row-by-row, respecting mode semantics: "full" rolls back on any error, "partial" skips failures and reports them individually.
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

// importQuerier returns a Querier and finalizer for import operations, ensuring RLS enforcement and wrapping full-mode imports in an explicit transaction even when no auth claims are present.
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
