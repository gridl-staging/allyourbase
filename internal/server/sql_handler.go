package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sqlRequest is the request body for the SQL editor endpoint.
type sqlRequest struct {
	Query string `json:"query"`
}

// sqlResponse is the response body for the SQL editor endpoint.
type sqlResponse struct {
	Columns    []string `json:"columns"`
	Rows       [][]any  `json:"rows"`
	RowCount   int      `json:"rowCount"`
	DurationMs int64    `json:"durationMs"`
}

// QueryTimeout is the maximum execution time for a SQL editor query.
const QueryTimeout = 30 * time.Second

// isDDL returns true if the query starts with a DDL keyword.
func isDDL(query string) bool {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return false
	}
	switch strings.ToUpper(fields[0]) {
	case "CREATE", "ALTER", "DROP", "TRUNCATE", "GRANT", "REVOKE", "COMMENT":
		return true
	}
	return false
}

// handleAdminSQL executes a raw SQL query and returns the results.
// This is admin-only (gated by requireAdminToken middleware).
// If the query is DDL, the schema cache is reloaded synchronously before
// responding so that subsequent /api/schema requests reflect the change.
func handleAdminSQL(pool *pgxpool.Pool, sc *schema.CacheHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req sqlRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.Query) == "" {
			httputil.WriteError(w, http.StatusBadRequest, "query is required")
			return
		}

		if pool == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "database not available")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), QueryTimeout)
		defer cancel()

		start := time.Now()

		rows, err := pool.Query(ctx, req.Query, pgx.QueryExecModeSimpleProtocol)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		defer rows.Close()

		// Read column names from the result set.
		fieldDescs := rows.FieldDescriptions()
		columns := make([]string, len(fieldDescs))
		for i, fd := range fieldDescs {
			columns[i] = fd.Name
		}

		// Read all rows.
		var resultRows [][]any
		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				httputil.WriteError(w, http.StatusInternalServerError, "reading row: "+err.Error())
				return
			}
			// Convert values to JSON-safe types.
			row := make([]any, len(values))
			for i, v := range values {
				row[i] = toJSONSafe(v)
			}
			resultRows = append(resultRows, row)
		}
		if err := rows.Err(); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		if resultRows == nil {
			resultRows = [][]any{}
		}

		// Reload schema cache synchronously after DDL so the next
		// /api/schema request returns the updated schema.
		if isDDL(req.Query) && sc != nil {
			if err := sc.ReloadWait(r.Context()); err != nil {
				// Log but don't fail the request — the DDL itself succeeded.
				slog.Default().Warn("schema reload after DDL failed", "error", err)
			}
		}

		duration := time.Since(start)
		httputil.WriteJSON(w, http.StatusOK, sqlResponse{
			Columns:    columns,
			Rows:       resultRows,
			RowCount:   len(resultRows),
			DurationMs: duration.Milliseconds(),
		})
	}
}

// toJSONSafe converts pgx values to types that json.Marshal handles cleanly.
func toJSONSafe(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case time.Time:
		return val.Format(time.RFC3339Nano)
	case [16]byte:
		// PostgreSQL UUID returned as [16]byte by pgx.
		return fmt.Sprintf("%x-%x-%x-%x-%x",
			val[0:4], val[4:6], val[6:8], val[8:10], val[10:16])
	case []byte:
		// Try to parse as JSON first.
		var j any
		if err := json.Unmarshal(val, &j); err == nil {
			return j
		}
		return string(val)
	default:
		return v
	}
}
