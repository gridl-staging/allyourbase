package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapPGError(t *testing.T) {
	t.Parallel()
	constraintDoc := httputil.DocURL("/guide/api-reference#error-format")

	tests := []struct {
		name       string
		err        error
		wantCode   int
		wantMsg    string
		wantDocURL string
		wantResult bool // true if mapPGError handled the error
	}{
		{
			name:       "nil error",
			err:        nil,
			wantResult: false,
		},
		{
			name:       "ErrNoRows returns 404",
			err:        pgx.ErrNoRows,
			wantCode:   http.StatusNotFound,
			wantMsg:    "record not found",
			wantResult: true,
		},
		{
			name:       "raise_exception returns 400",
			err:        &pgconn.PgError{Code: "P0001", Message: "age must be positive"},
			wantCode:   http.StatusBadRequest,
			wantMsg:    "age must be positive",
			wantResult: true,
		},
		{
			name:       "unique_violation returns 409 with doc_url",
			err:        &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key", Detail: "Key (email)=(a@b.com) already exists."},
			wantCode:   http.StatusConflict,
			wantMsg:    "unique constraint violation",
			wantDocURL: constraintDoc,
			wantResult: true,
		},
		{
			name:       "foreign_key_violation returns 400 with doc_url",
			err:        &pgconn.PgError{Code: "23503", ConstraintName: "posts_author_id_fkey", Detail: "Key (author_id)=(999) is not present in table users."},
			wantCode:   http.StatusBadRequest,
			wantMsg:    "foreign key violation",
			wantDocURL: constraintDoc,
			wantResult: true,
		},
		{
			name:       "not_null_violation returns 400 with doc_url",
			err:        &pgconn.PgError{Code: "23502", ColumnName: "title", Message: "null value in column \"title\""},
			wantCode:   http.StatusBadRequest,
			wantMsg:    "missing required value",
			wantDocURL: constraintDoc,
			wantResult: true,
		},
		{
			name:       "check_violation returns 400 with doc_url",
			err:        &pgconn.PgError{Code: "23514", ConstraintName: "positive_price", Detail: "Failing row contains (-1)."},
			wantCode:   http.StatusBadRequest,
			wantMsg:    "check constraint violation",
			wantDocURL: constraintDoc,
			wantResult: true,
		},
		{
			name:       "invalid_text_representation uuid returns friendly hint with doc_url",
			err:        &pgconn.PgError{Code: "22P02", Message: `invalid input syntax for type uuid: "4234234"`},
			wantCode:   http.StatusBadRequest,
			wantMsg:    "invalid uuid value \u2014 expected format: 550e8400-e29b-41d4-a716-446655440000",
			wantDocURL: constraintDoc,
			wantResult: true,
		},
		{
			name:       "invalid_text_representation integer returns friendly hint with doc_url",
			err:        &pgconn.PgError{Code: "22P02", Message: `invalid input syntax for type integer: "abc"`},
			wantCode:   http.StatusBadRequest,
			wantMsg:    "invalid integer value \u2014 expected a whole number, e.g. 42",
			wantDocURL: constraintDoc,
			wantResult: true,
		},
		{
			name:       "invalid_text_representation unknown type falls back with doc_url",
			err:        &pgconn.PgError{Code: "22P02", Message: "invalid input syntax for type sometype"},
			wantCode:   http.StatusBadRequest,
			wantMsg:    "invalid value: invalid input syntax for type sometype",
			wantDocURL: constraintDoc,
			wantResult: true,
		},
		{
			name:       "insufficient_privilege (42501) returns 403 — RLS WITH CHECK violation",
			err:        &pgconn.PgError{Code: "42501", Message: "new row violates row-level security policy for table \"polls\""},
			wantCode:   http.StatusForbidden,
			wantMsg:    "insufficient permissions",
			wantResult: true,
		},
		{
			name:       "unhandled PG error code returns false",
			err:        &pgconn.PgError{Code: "42P01", Message: "relation does not exist"},
			wantResult: false,
		},
		{
			name:       "non-PG error returns false",
			err:        errors.New("connection refused"),
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			result := mapPGError(w, tt.err)
			testutil.Equal(t, tt.wantResult, result)

			if tt.wantResult {
				testutil.Equal(t, tt.wantCode, w.Code)

				var resp httputil.ErrorResponse
				err := json.NewDecoder(w.Body).Decode(&resp)
				testutil.NoError(t, err)
				testutil.Equal(t, tt.wantCode, resp.Code)
				testutil.Equal(t, tt.wantMsg, resp.Message)
				testutil.Equal(t, tt.wantDocURL, resp.DocURL)
			}
		})
	}
}

func TestFriendlyTypeError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pgMsg string
		want  string
	}{
		{
			pgMsg: `invalid input syntax for type uuid: "4234234"`,
			want:  "invalid uuid value \u2014 expected format: 550e8400-e29b-41d4-a716-446655440000",
		},
		{
			pgMsg: `invalid input syntax for type integer: "abc"`,
			want:  "invalid integer value \u2014 expected a whole number, e.g. 42",
		},
		{
			pgMsg: `invalid input syntax for type boolean: "maybe"`,
			want:  "invalid boolean value \u2014 expected true or false",
		},
		{
			pgMsg: `invalid input syntax for type jsonb: "not json"`,
			want:  `invalid jsonb value — expected valid JSON, e.g. {"key": "value"}`,
		},
		{
			pgMsg: `invalid input syntax for type date: "yesterday"`,
			want:  "invalid date value \u2014 expected format: 2024-01-15",
		},
		{
			pgMsg: `invalid input syntax for type inet: "not-an-ip"`,
			want:  "invalid inet value \u2014 expected an IP address, e.g. 192.168.1.1",
		},
		{
			// Unknown type falls back to raw message.
			pgMsg: `invalid input syntax for type customtype: "foo"`,
			want:  `invalid value: invalid input syntax for type customtype: "foo"`,
		},
		{
			// Non-standard message format falls back gracefully.
			pgMsg: "something unexpected",
			want:  "invalid value: something unexpected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.pgMsg, func(t *testing.T) {
			t.Parallel()
			got := friendlyTypeError(tt.pgMsg)
			testutil.Equal(t, tt.want, got)
		})
	}
}
