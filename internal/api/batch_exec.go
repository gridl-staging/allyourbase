package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
)

// execBatchOp executes a single batch operation within a transaction.
// Returns the result, an optional event for publish, and any error.
func (h *Handler) execBatchOp(r *http.Request, q Querier, tbl *schema.Table, op BatchOperation) (BatchResult, *realtime.Event, error) {
	auditMutation := h.auditSink != nil && h.auditSink.ShouldAudit(tbl.Name)

	switch op.Method {
	case "create":
		return h.execBatchCreate(r, q, tbl, op, auditMutation)
	case "update":
		return h.execBatchUpdate(r, q, tbl, op, auditMutation)
	case "delete":
		return h.execBatchDelete(r, q, tbl, op, auditMutation)
	default:
		// Already validated — this shouldn't happen.
		return BatchResult{}, nil, fmt.Errorf("unknown method %q", op.Method)
	}
}

func (h *Handler) execBatchCreate(r *http.Request, q Querier, tbl *schema.Table, op BatchOperation, auditMutation bool) (BatchResult, *realtime.Event, error) {
	if err := maybeEncryptBatchRecord(h.fieldEncryptor, tbl.Name, op.Body); err != nil {
		return BatchResult{}, nil, err
	}

	query, args := buildInsert(tbl, op.Body)
	record, err := queryBatchRecord(r.Context(), q, query, args...)
	if err != nil {
		return BatchResult{}, nil, err
	}
	if err := maybeDecryptBatchRecord(h.fieldEncryptor, tbl.Name, record); err != nil {
		return BatchResult{}, nil, err
	}

	if err := maybeLogBatchAudit(
		r.Context(),
		h.auditSink,
		auditMutation,
		q,
		audit.AuditEntry{
			TableName: tbl.Name,
			RecordID:  pkMap(tbl, record),
			Operation: "INSERT",
			NewValues: record,
		},
	); err != nil {
		return BatchResult{}, nil, err
	}

	event := &realtime.Event{Action: "create", Table: tbl.Name, Record: record}
	return BatchResult{Status: http.StatusCreated, Body: record}, event, nil
}

func (h *Handler) execBatchUpdate(r *http.Request, q Querier, tbl *schema.Table, op BatchOperation, auditMutation bool) (BatchResult, *realtime.Event, error) {
	if err := maybeEncryptBatchRecord(h.fieldEncryptor, tbl.Name, op.Body); err != nil {
		return BatchResult{}, nil, err
	}

	pkValues, err := parseBatchOpPrimaryKey(tbl, op.ID, op.Method)
	if err != nil {
		return BatchResult{}, nil, err
	}

	// Always use the CTE variant to capture the pre-update row for
	// realtime column-level filter enter/leave semantics.
	query, args := buildUpdateWithAudit(tbl, op.Body, pkValues)
	record, err := queryBatchRecord(r.Context(), q, query, args...)
	if err != nil {
		return BatchResult{}, nil, err
	}
	if err := maybeDecryptBatchRecord(h.fieldEncryptor, tbl.Name, record); err != nil {
		return BatchResult{}, nil, err
	}
	if record == nil {
		return BatchResult{}, nil, fmt.Errorf("%w: %s", errBatchNotFound, op.ID)
	}

	oldRecord := extractOldRecord(record)
	if err := maybeLogBatchAudit(
		r.Context(),
		h.auditSink,
		auditMutation,
		q,
		audit.AuditEntry{
			TableName: tbl.Name,
			RecordID:  pkMap(tbl, record),
			Operation: "UPDATE",
			OldValues: oldRecord,
			NewValues: record,
		},
	); err != nil {
		return BatchResult{}, nil, err
	}

	event := &realtime.Event{Action: "update", Table: tbl.Name, Record: record, OldRecord: oldRecord}
	return BatchResult{Status: http.StatusOK, Body: record}, event, nil
}

func (h *Handler) execBatchDelete(r *http.Request, q Querier, tbl *schema.Table, op BatchOperation, auditMutation bool) (BatchResult, *realtime.Event, error) {
	pkValues, err := parseBatchOpPrimaryKey(tbl, op.ID, op.Method)
	if err != nil {
		return BatchResult{}, nil, err
	}

	// Always use RETURNING to capture the full deleted row for realtime
	// filter evaluation.
	query, args := buildDeleteReturning(tbl, pkValues)
	deletedRecord, err := queryBatchRecord(r.Context(), q, query, args...)
	if err != nil {
		return BatchResult{}, nil, err
	}
	if deletedRecord == nil {
		return BatchResult{}, nil, fmt.Errorf("%w: %s", errBatchNotFound, op.ID)
	}

	if err := maybeLogBatchAudit(
		r.Context(),
		h.auditSink,
		auditMutation,
		q,
		audit.AuditEntry{
			TableName: tbl.Name,
			RecordID:  pkMap(tbl, deletedRecord),
			Operation: "DELETE",
			OldValues: deletedRecord,
		},
	); err != nil {
		return BatchResult{}, nil, err
	}

	event := &realtime.Event{
		Action:    "delete",
		Table:     tbl.Name,
		Record:    buildBatchDeleteEventRecord(tbl, pkValues),
		OldRecord: deletedRecord,
	}
	return BatchResult{Status: http.StatusNoContent}, event, nil
}

func queryBatchRecord(ctx context.Context, q Querier, query string, args ...any) (map[string]any, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	record, scanErr := scanRow(rows)
	rows.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	return record, nil
}

func maybeEncryptBatchRecord(encryptor *FieldEncryptor, table string, record map[string]any) error {
	if encryptor == nil {
		return nil
	}
	return encryptor.EncryptRecord(table, record)
}

func maybeDecryptBatchRecord(encryptor *FieldEncryptor, table string, record map[string]any) error {
	if encryptor == nil {
		return nil
	}
	return encryptor.DecryptRecord(table, record)
}

func maybeLogBatchAudit(ctx context.Context, sink audit.Sink, auditMutation bool, q Querier, entry audit.AuditEntry) error {
	if !auditMutation || sink == nil {
		return nil
	}
	return sink.LogMutationWithQuerier(ctx, q, entry)
}

func parseBatchOpPrimaryKey(tbl *schema.Table, rawID, method string) ([]string, error) {
	pkValues := parsePKValues(rawID, len(tbl.PrimaryKey))
	if len(pkValues) != len(tbl.PrimaryKey) {
		return nil, fmt.Errorf("invalid primary key for %s", method)
	}
	return pkValues, nil
}

func buildBatchDeleteEventRecord(tbl *schema.Table, pkValues []string) map[string]any {
	record := make(map[string]any, len(tbl.PrimaryKey))
	for i, pk := range tbl.PrimaryKey {
		record[pk] = pkValues[i]
	}
	return record
}
