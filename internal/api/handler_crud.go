package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/go-chi/chi/v5"
)

// extractPK parses the "id" URL parameter into PK values and validates the count.
// Returns nil and writes a 400 error if the PK is invalid.
func extractPK(w http.ResponseWriter, r *http.Request, tbl *schema.Table) []string {
	idParam := chi.URLParam(r, "id")
	pkValues := parsePKValues(idParam, len(tbl.PrimaryKey))
	if len(pkValues) != len(tbl.PrimaryKey) {
		writeError(w, http.StatusBadRequest, "invalid primary key: expected "+strconv.Itoa(len(tbl.PrimaryKey))+" values")
		return nil
	}
	return pkValues
}

// handleRead handles GET /collections/{table}/{id}
func (h *Handler) handleRead(w http.ResponseWriter, r *http.Request) {
	tbl := h.resolveTable(w, r)
	if tbl == nil {
		return
	}
	if !requirePK(w, tbl) {
		return
	}

	pkValues := extractPK(w, r, tbl)
	if pkValues == nil {
		return
	}

	fields := parseFields(r)
	query, args := buildSelectOne(tbl, fields, pkValues)

	q, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := q.Query(r.Context(), query, args...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("query error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	record, err := scanRow(rows)
	rows.Close() // Close before done() to avoid pgx "conn busy" on commit.
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("scan error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if h.fieldEncryptor != nil {
		if err := h.fieldEncryptor.DecryptRecord(tbl.Name, record); err != nil {
			done(err)
			h.logger.Error("decrypt response record error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if record == nil {
		if err := done(nil); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeError(w, http.StatusNotFound, "record not found")
		return
	}

	// Handle expand if requested.
	if expandParam := r.URL.Query().Get("expand"); expandParam != "" {
		sc := h.schema.Get()
		if sc != nil {
			expandRecords(r.Context(), q, sc, tbl, []map[string]any{record}, expandParam, h.logger)
		}
	}

	if err := done(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

// decodeAndValidateBody reads, decodes, and validates a JSON request body against the table schema.
// Returns the decoded data and true on success. On failure, writes an error response and returns nil, false.
func decodeAndValidateBody(w http.ResponseWriter, r *http.Request, tbl *schema.Table) (map[string]any, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, httputil.MaxBodySize)
	var data map[string]any
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return nil, false
	}

	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "empty request body")
		return nil, false
	}

	if countKnownColumns(tbl, data) == 0 {
		writeError(w, http.StatusBadRequest, "no recognized columns in request body")
		return nil, false
	}

	return data, true
}

// handleCreate handles POST /collections/{table}
func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
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

	data, ok := decodeAndValidateBody(w, r, tbl)
	if !ok {
		return
	}
	if h.fieldEncryptor != nil {
		if err := h.fieldEncryptor.EncryptRecord(tbl.Name, data); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	query, args := buildInsert(tbl, data)

	q, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := q.Query(r.Context(), query, args...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("insert error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	record, err := scanRow(rows)
	rows.Close() // Close before done() to avoid pgx "conn busy" on commit.
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("scan error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if h.fieldEncryptor != nil {
		if err := h.fieldEncryptor.DecryptRecord(tbl.Name, record); err != nil {
			done(err)
			h.logger.Error("decrypt response record error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if h.auditSink != nil && h.auditSink.ShouldAudit(tbl.Name) {
		if aerr := h.auditSink.LogMutationWithQuerier(r.Context(), q, audit.AuditEntry{
			TableName: tbl.Name,
			RecordID:  pkMap(tbl, record),
			Operation: "INSERT",
			NewValues: record,
		}); aerr != nil {
			done(aerr)
			h.logger.Error("audit log insert failed", "error", aerr, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := done(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, record)
	h.publishEvent("create", tbl.Name, record, nil)
}

// handleUpdate handles PATCH /collections/{table}/{id}
func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
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

	pkValues := extractPK(w, r, tbl)
	if pkValues == nil {
		return
	}

	data, ok := decodeAndValidateBody(w, r, tbl)
	if !ok {
		return
	}
	if h.fieldEncryptor != nil {
		if err := h.fieldEncryptor.EncryptRecord(tbl.Name, data); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	auditUpdate := h.auditSink != nil && h.auditSink.ShouldAudit(tbl.Name)
	// Always use the CTE variant to capture the pre-update row. The old row
	// is needed for realtime column-level filter enter/leave semantics, and
	// optionally for audit logging. The CTE adds negligible overhead (the PK
	// index lookup hits the same buffer page as the UPDATE).
	query, args := buildUpdateWithAudit(tbl, data, pkValues)

	q, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := q.Query(r.Context(), query, args...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("update error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	record, err := scanRow(rows)
	rows.Close() // Close before done() to avoid pgx "conn busy" on commit.
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("scan error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if record == nil {
		if err := done(nil); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeError(w, http.StatusNotFound, "record not found")
		return
	}

	// Extract the pre-update row from the CTE sentinel column. This strips
	// _audit_old_values from record so it won't appear in the API response.
	oldRecord := extractOldRecord(record)

	if auditUpdate {
		if aerr := h.auditSink.LogMutationWithQuerier(r.Context(), q, audit.AuditEntry{
			TableName: tbl.Name,
			RecordID:  pkMap(tbl, record),
			Operation: "UPDATE",
			OldValues: oldRecord,
			NewValues: record,
		}); aerr != nil {
			done(aerr)
			h.logger.Error("audit log update failed", "error", aerr, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := done(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, record)
	h.publishEvent("update", tbl.Name, record, oldRecord)
}

// handleDelete handles DELETE /collections/{table}/{id}.
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
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

	pkValues := extractPK(w, r, tbl)
	if pkValues == nil {
		return
	}

	auditDelete := h.auditSink != nil && h.auditSink.ShouldAudit(tbl.Name)

	q, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Always use RETURNING to capture the full old row for realtime filter
	// evaluation. The deleted row is needed for column-level filter matching.
	query, args := buildDeleteReturning(tbl, pkValues)
	rows, qErr := q.Query(r.Context(), query, args...)
	if qErr != nil {
		done(qErr)
		if !mapPGError(w, qErr) {
			h.logger.Error("delete error", "error", qErr, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	deletedRecord, scanErr := scanRow(rows)
	rows.Close()
	if scanErr != nil {
		done(scanErr)
		if !mapPGError(w, scanErr) {
			h.logger.Error("scan error", "error", scanErr, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if deletedRecord == nil {
		if err := done(nil); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeError(w, http.StatusNotFound, "record not found")
		return
	}

	if auditDelete {
		if aerr := h.auditSink.LogMutationWithQuerier(r.Context(), q, audit.AuditEntry{
			TableName: tbl.Name,
			RecordID:  pkMap(tbl, deletedRecord),
			Operation: "DELETE",
			OldValues: deletedRecord,
		}); aerr != nil {
			done(aerr)
			h.logger.Error("audit log delete failed", "error", aerr, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := done(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)

	// Publish delete event: Record has PK values (backward compat),
	// OldRecord has the full pre-delete row for filter evaluation.
	eventRecord := make(map[string]any, len(tbl.PrimaryKey))
	for i, pk := range tbl.PrimaryKey {
		eventRecord[pk] = pkValues[i]
	}
	h.publishEvent("delete", tbl.Name, eventRecord, deletedRecord)
}

// publishEvent sends a realtime event to the hub and webhook dispatcher.
// oldRecord is the pre-mutation row (for UPDATE/DELETE filter evaluation); nil for INSERT.
func (h *Handler) publishEvent(action, table string, record, oldRecord map[string]any) {
	if h.hub == nil && h.dispatcher == nil {
		return
	}
	event := &realtime.Event{
		Action:    action,
		Table:     table,
		Record:    record,
		OldRecord: oldRecord,
	}
	if h.hub != nil {
		h.hub.Publish(event)
	}
	if h.dispatcher != nil {
		h.dispatcher.Enqueue(event)
	}
}
