package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
)

// errBatchNotFound is returned when a batch update/delete targets a non-existent row.
var errBatchNotFound = errors.New("record not found")

// maxBatchSize is the maximum number of operations in a single batch request.
const maxBatchSize = 1000

// BatchRequest is the JSON body for POST /collections/{table}/batch.
type BatchRequest struct {
	Operations []BatchOperation `json:"operations"`
}

// BatchOperation is a single operation within a batch.
type BatchOperation struct {
	Method string         `json:"method"` // "create", "update", "delete"
	ID     string         `json:"id"`     // required for update/delete
	Body   map[string]any `json:"body"`   // required for create/update
}

// BatchResult is the result of a single operation within a batch.
type BatchResult struct {
	Index  int            `json:"index"`
	Status int            `json:"status"`
	Body   map[string]any `json:"body,omitempty"`
}

// handleBatch handles POST /collections/{table}/batch
func (h *Handler) handleBatch(w http.ResponseWriter, r *http.Request) {
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

	// Decode request body.
	r.Body = http.MaxBytesReader(w, r.Body, httputil.MaxBodySize)
	var req BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.Operations) == 0 {
		writeErrorWithDoc(w, http.StatusBadRequest, "operations array is empty", docURL("/guide/api-reference#batch-operations"))
		return
	}
	if len(req.Operations) > maxBatchSize {
		writeErrorWithDoc(w, http.StatusBadRequest, fmt.Sprintf("too many operations: max %d", maxBatchSize), docURL("/guide/api-reference#batch-operations"))
		return
	}

	// Validate all operations before starting the transaction.
	for i, op := range req.Operations {
		if err := validateBatchOp(tbl, op); err != nil {
			writeErrorWithDoc(w, http.StatusBadRequest, fmt.Sprintf("operation[%d]: %s", i, err.Error()), docURL("/guide/api-reference#batch-operations"))
			return
		}
	}

	// Begin transaction with RLS context.
	tx, err := h.beginTx(r.Context())
	if err != nil {
		h.logger.Error("batch: begin tx error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	// Set RLS session variables if JWT claims are present.
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		if err := auth.SetRLSContext(r.Context(), tx, claims); err != nil {
			h.logger.Error("batch: rls setup error", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	// Execute each operation within the transaction.
	results := make([]BatchResult, len(req.Operations))
	var events []*realtime.Event

	for i, op := range req.Operations {
		result, event, err := h.execBatchOp(r, tx, tbl, op)
		if err != nil {
			// Transaction will be rolled back by the deferred Rollback.
			if errors.Is(err, errBatchNotFound) {
				writeError(w, http.StatusNotFound, err.Error())
			} else if !mapPGError(w, err) {
				h.logger.Error("batch: operation error", "error", err, "index", i, "method", op.Method)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		result.Index = i
		results[i] = result
		if event != nil {
			events = append(events, event)
		}
	}

	// Commit the transaction.
	if err := tx.Commit(r.Context()); err != nil {
		h.logger.Error("batch: commit error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Publish events after successful commit.
	for _, event := range events {
		if h.hub != nil {
			h.hub.Publish(event)
		}
		if h.dispatcher != nil {
			h.dispatcher.Enqueue(event)
		}
	}

	writeJSON(w, http.StatusOK, results)
}

// validateBatchOp validates a single batch operation before execution.
func validateBatchOp(tbl *schema.Table, op BatchOperation) error {
	switch op.Method {
	case "create":
		if len(op.Body) == 0 {
			return fmt.Errorf("create requires a body")
		}
		if countKnownColumns(tbl, op.Body) == 0 {
			return fmt.Errorf("no recognized columns in body")
		}
	case "update":
		if op.ID == "" {
			return fmt.Errorf("update requires an id")
		}
		if len(op.Body) == 0 {
			return fmt.Errorf("update requires a body")
		}
		if countKnownColumns(tbl, op.Body) == 0 {
			return fmt.Errorf("no recognized columns in body")
		}
	case "delete":
		if op.ID == "" {
			return fmt.Errorf("delete requires an id")
		}
	default:
		return fmt.Errorf("unknown method %q (expected create, update, or delete)", op.Method)
	}
	return nil
}

// execBatchOp executes a single batch operation within a transaction.
// Returns the result, an optional event for publish, and any error.
func (h *Handler) execBatchOp(r *http.Request, q Querier, tbl *schema.Table, op BatchOperation) (BatchResult, *realtime.Event, error) {
	auditMutation := h.auditSink != nil && h.auditSink.ShouldAudit(tbl.Name)

	switch op.Method {
	case "create":
		if h.fieldEncryptor != nil {
			if err := h.fieldEncryptor.EncryptRecord(tbl.Name, op.Body); err != nil {
				return BatchResult{}, nil, err
			}
		}
		query, args := buildInsert(tbl, op.Body)
		rows, err := q.Query(r.Context(), query, args...)
		if err != nil {
			return BatchResult{}, nil, err
		}
		record, err := scanRow(rows)
		rows.Close()
		if err != nil {
			return BatchResult{}, nil, err
		}
		if h.fieldEncryptor != nil {
			if err := h.fieldEncryptor.DecryptRecord(tbl.Name, record); err != nil {
				return BatchResult{}, nil, err
			}
		}
		if auditMutation {
			if err := h.auditSink.LogMutationWithQuerier(r.Context(), q, audit.AuditEntry{
				TableName: tbl.Name,
				RecordID:  pkMap(tbl, record),
				Operation: "INSERT",
				NewValues: record,
			}); err != nil {
				return BatchResult{}, nil, err
			}
		}
		event := &realtime.Event{Action: "create", Table: tbl.Name, Record: record}
		return BatchResult{Status: http.StatusCreated, Body: record}, event, nil

	case "update":
		if h.fieldEncryptor != nil {
			if err := h.fieldEncryptor.EncryptRecord(tbl.Name, op.Body); err != nil {
				return BatchResult{}, nil, err
			}
		}
		pkValues := parsePKValues(op.ID, len(tbl.PrimaryKey))
		if len(pkValues) != len(tbl.PrimaryKey) {
			return BatchResult{}, nil, fmt.Errorf("invalid primary key for update")
		}
		// Always use the CTE variant to capture the pre-update row for
		// realtime column-level filter enter/leave semantics.
		query, args := buildUpdateWithAudit(tbl, op.Body, pkValues)
		rows, err := q.Query(r.Context(), query, args...)
		if err != nil {
			return BatchResult{}, nil, err
		}
		record, err := scanRow(rows)
		rows.Close()
		if err != nil {
			return BatchResult{}, nil, err
		}
		if h.fieldEncryptor != nil {
			if err := h.fieldEncryptor.DecryptRecord(tbl.Name, record); err != nil {
				return BatchResult{}, nil, err
			}
		}
		if record == nil {
			return BatchResult{}, nil, fmt.Errorf("%w: %s", errBatchNotFound, op.ID)
		}
		oldRecord := extractOldRecord(record)
		if auditMutation {
			if err := h.auditSink.LogMutationWithQuerier(r.Context(), q, audit.AuditEntry{
				TableName: tbl.Name,
				RecordID:  pkMap(tbl, record),
				Operation: "UPDATE",
				OldValues: oldRecord,
				NewValues: record,
			}); err != nil {
				return BatchResult{}, nil, err
			}
		}
		event := &realtime.Event{Action: "update", Table: tbl.Name, Record: record, OldRecord: oldRecord}
		return BatchResult{Status: http.StatusOK, Body: record}, event, nil

	case "delete":
		pkValues := parsePKValues(op.ID, len(tbl.PrimaryKey))
		if len(pkValues) != len(tbl.PrimaryKey) {
			return BatchResult{}, nil, fmt.Errorf("invalid primary key for delete")
		}
		// Always use RETURNING to capture the full deleted row for realtime
		// filter evaluation.
		query, args := buildDeleteReturning(tbl, pkValues)
		rows, err := q.Query(r.Context(), query, args...)
		if err != nil {
			return BatchResult{}, nil, err
		}
		deletedRecord, err := scanRow(rows)
		rows.Close()
		if err != nil {
			return BatchResult{}, nil, err
		}
		if deletedRecord == nil {
			return BatchResult{}, nil, fmt.Errorf("%w: %s", errBatchNotFound, op.ID)
		}
		if auditMutation {
			if err := h.auditSink.LogMutationWithQuerier(r.Context(), q, audit.AuditEntry{
				TableName: tbl.Name,
				RecordID:  pkMap(tbl, deletedRecord),
				Operation: "DELETE",
				OldValues: deletedRecord,
			}); err != nil {
				return BatchResult{}, nil, err
			}
		}
		// Event Record has PK values only (backward compat); OldRecord has
		// the full pre-delete row for filter evaluation.
		pkRecord := make(map[string]any, len(tbl.PrimaryKey))
		for i, pk := range tbl.PrimaryKey {
			pkRecord[pk] = pkValues[i]
		}
		event := &realtime.Event{Action: "delete", Table: tbl.Name, Record: pkRecord, OldRecord: deletedRecord}
		return BatchResult{Status: http.StatusNoContent}, event, nil

	default:
		// Already validated — this shouldn't happen.
		return BatchResult{}, nil, fmt.Errorf("unknown method %q", op.Method)
	}
}
