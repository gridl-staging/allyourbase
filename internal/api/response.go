package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ListResponse is the envelope for paginated list endpoints.
type ListResponse struct {
	Page       int              `json:"page"`
	PerPage    int              `json:"perPage"`
	TotalItems int              `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
	Items      []map[string]any `json:"items"`
}

// Package-level aliases for the shared HTTP helpers so existing call sites
// within this package continue to compile without changes.
var (
	writeJSON         = httputil.WriteJSON
	writeError        = httputil.WriteError
	writeErrorWithDoc = httputil.WriteErrorWithDocURL
	docURL            = httputil.DocURL
)

// WriteFieldErrorWithDocURL writes an error response with field-level validation detail and a doc URL.
func writeFieldErrorWithDocURL(w http.ResponseWriter, status int, message string, field, fieldCode, fieldMsg, docURL string) {
	httputil.WriteJSON(w, status, httputil.ErrorResponse{
		Code:    status,
		Message: message,
		Data: map[string]any{
			field: map[string]string{
				"code":    fieldCode,
				"message": fieldMsg,
			},
		},
		DocURL: docURL,
	})
}

// mapPGError converts a pgx/pgconn error to an appropriate HTTP response.
// Returns true if a PG error was handled.
func mapPGError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "record not found")
		return true
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	constraintDoc := docURL("/guide/api-reference#error-format")

	switch pgErr.Code {
	case "P0001": // raise_exception — user-defined exceptions from PL/pgSQL RAISE EXCEPTION
		writeError(w, http.StatusBadRequest, pgErr.Message)
	case "23505": // unique_violation
		writeFieldErrorWithDocURL(w, http.StatusConflict, "unique constraint violation",
			pgErr.ConstraintName, "unique_violation", pgErr.Detail, constraintDoc)
	case "23503": // foreign_key_violation
		writeFieldErrorWithDocURL(w, http.StatusBadRequest, "foreign key violation",
			pgErr.ConstraintName, "foreign_key_violation", pgErr.Detail, constraintDoc)
	case "23502": // not_null_violation
		writeFieldErrorWithDocURL(w, http.StatusBadRequest, "missing required value",
			pgErr.ColumnName, "not_null_violation", pgErr.Message, constraintDoc)
	case "23514": // check_violation
		writeFieldErrorWithDocURL(w, http.StatusBadRequest, "check constraint violation",
			pgErr.ConstraintName, "check_violation", pgErr.Detail, constraintDoc)
	case "22P02": // invalid_text_representation
		writeErrorWithDoc(w, http.StatusBadRequest, friendlyTypeError(pgErr.Message), constraintDoc)
	case "42501": // insufficient_privilege — raised by RLS WITH CHECK policy violations
		writeError(w, http.StatusForbidden, "insufficient permissions")
	default:
		return false
	}
	return true
}

// typeFormatHints maps PostgreSQL type names to human-friendly format examples.
var typeFormatHints = map[string]string{
	"uuid":                        "expected format: 550e8400-e29b-41d4-a716-446655440000",
	"integer":                     "expected a whole number, e.g. 42",
	"smallint":                    "expected a whole number (-32768 to 32767)",
	"bigint":                      "expected a whole number, e.g. 42",
	"numeric":                     "expected a number, e.g. 3.14",
	"real":                        "expected a number, e.g. 3.14",
	"double precision":            "expected a number, e.g. 3.14",
	"boolean":                     "expected true or false",
	"json":                        `expected valid JSON, e.g. {"key": "value"}`,
	"jsonb":                       `expected valid JSON, e.g. {"key": "value"}`,
	"timestamp without time zone": "expected format: 2024-01-15 09:30:00",
	"timestamp with time zone":    "expected format: 2024-01-15T09:30:00Z",
	"date":                        "expected format: 2024-01-15",
	"time":                        "expected format: 09:30:00",
	"inet":                        "expected an IP address, e.g. 192.168.1.1",
	"cidr":                        "expected a network range, e.g. 192.168.1.0/24",
	"macaddr":                     "expected format: 08:00:2b:01:02:03",
}

// friendlyTypeError rewrites a Postgres 22P02 "invalid input syntax for type X"
// message into a human-friendly message with a format hint.
func friendlyTypeError(pgMsg string) string {
	// Postgres message format: `invalid input syntax for type <typename>: "<value>"`
	const prefix = "invalid input syntax for type "
	idx := strings.Index(pgMsg, prefix)
	if idx < 0 {
		return "invalid value: " + pgMsg
	}
	rest := pgMsg[idx+len(prefix):]

	// Extract type name (everything before the colon, or the full string).
	typeName := rest
	if ci := strings.Index(rest, ":"); ci >= 0 {
		typeName = rest[:ci]
	}

	if hint, ok := typeFormatHints[typeName]; ok {
		return "invalid " + typeName + " value — " + hint
	}
	return "invalid value: " + pgMsg
}
