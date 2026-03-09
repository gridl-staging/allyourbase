package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/go-chi/chi/v5"
)

// handleRPC handles POST /rpc/{function}
func (h *Handler) handleRPC(w http.ResponseWriter, r *http.Request) {
	if !requireWriteScope(w, r) {
		return
	}
	fn := h.resolveFunction(w, r)
	if fn == nil {
		return
	}

	// Decode JSON body as named arguments (empty body = no args).
	var args map[string]any
	if r.ContentLength > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, httputil.MaxBodySize)
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	query, queryArgs, err := buildRPCCall(fn, args)
	if err != nil {
		writeErrorWithDoc(w, http.StatusBadRequest, err.Error(), docURL("/guide/database-rpc"))
		return
	}

	q, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if fn.IsVoid {
		_, err := q.Exec(r.Context(), query, queryArgs...)
		if err != nil {
			done(err)
			if !mapPGError(w, err) {
				h.logger.Error("rpc error", "error", err, "function", fn.Name)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		done(nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	rows, err := q.Query(r.Context(), query, queryArgs...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("rpc error", "error", err, "function", fn.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	if fn.ReturnsSet {
		items, err := scanRows(rows)
		rows.Close() // Close before done() to avoid pgx "conn busy" on commit.
		if err != nil {
			done(err)
			h.logger.Error("rpc scan error", "error", err, "function", fn.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		done(nil)
		writeJSON(w, http.StatusOK, items)
		return
	}

	// Scalar or single-row return.
	record, err := scanRow(rows)
	rows.Close() // Close before done() to avoid pgx "conn busy" on commit.
	if err != nil {
		done(err)
		h.logger.Error("rpc scan error", "error", err, "function", fn.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	done(nil)

	if record == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}

	// If the result has a single column named after the function, unwrap it.
	if len(record) == 1 {
		for _, v := range record {
			writeJSON(w, http.StatusOK, v)
			return
		}
	}
	writeJSON(w, http.StatusOK, record)
}

// resolveFunction looks up the function in the schema cache and validates it exists.
func (h *Handler) resolveFunction(w http.ResponseWriter, r *http.Request) *schema.Function {
	sc := h.schema.Get()
	if sc == nil {
		writeError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return nil
	}

	funcName := chi.URLParam(r, "function")
	fn := sc.FunctionByName(funcName)
	if fn == nil {
		writeError(w, http.StatusNotFound, "function not found: "+funcName)
		return nil
	}
	return fn
}

// buildRPCCall generates the SQL and args for calling a function.
// For set-returning or OUT-param functions: SELECT * FROM schema.func($1, $2, ...)
// For scalar/void functions: SELECT schema.func($1, $2, ...)
func buildRPCCall(fn *schema.Function, args map[string]any) (string, []any, error) {
	var queryArgs []any
	placeholders := make([]string, len(fn.Parameters))

	for i, param := range fn.Parameters {
		val, ok := args[param.Name]
		if !ok {
			// If param has no name, try positional matching is not supported —
			// require named args for safety.
			if param.Name == "" {
				return "", nil, fmt.Errorf("function %q has unnamed parameters; cannot match by name", fn.Name)
			}
			// Missing param — pass NULL.
			val = nil
		}
		queryArgs = append(queryArgs, coerceRPCArg(val, param.Type))
		// Use explicit cast so pgx text-encodes the value and Postgres handles conversion.
		// VARIADIC params need the VARIADIC keyword so Postgres spreads the array.
		if param.IsVariadic {
			placeholders[i] = fmt.Sprintf("VARIADIC $%d::%s", i+1, param.Type)
		} else {
			placeholders[i] = fmt.Sprintf("$%d::%s", i+1, param.Type)
		}
	}

	funcRef := quoteIdent(fn.Schema) + "." + quoteIdent(fn.Name)
	argList := strings.Join(placeholders, ", ")

	var query string
	// Use SELECT * FROM for set-returning functions, functions with OUT params,
	// and record-returning functions so columns are unpacked into named fields.
	if fn.ReturnsSet || fn.HasOutParams || fn.ReturnType == "record" {
		query = fmt.Sprintf("SELECT * FROM %s(%s)", funcRef, argList)
	} else {
		query = fmt.Sprintf("SELECT %s(%s)", funcRef, argList)
	}

	return query, queryArgs, nil
}

// coerceRPCArg converts a JSON-decoded value to a Go type that pgx can encode
// for the given PostgreSQL type. JSON decodes numbers as float64 and arrays as
// []any, which pgx cannot always map to PG types without explicit conversion.
func coerceRPCArg(val any, pgType string) any {
	if val == nil {
		return nil
	}

	// Handle array types: convert []any to a typed slice that pgx can encode.
	if strings.HasSuffix(pgType, "[]") {
		arr, ok := val.([]any)
		if !ok {
			return val
		}
		elemType := strings.TrimSuffix(pgType, "[]")
		return coerceArray(arr, elemType)
	}

	// Handle scalar numeric types: JSON float64 -> appropriate Go type.
	if f, ok := val.(float64); ok {
		return coerceNumber(f, pgType)
	}

	return val
}

// coerceArray converts a []any slice to a typed slice based on the element type.
func coerceArray(arr []any, elemType string) any {
	switch elemType {
	case "integer", "int4", "smallint", "int2":
		out := make([]int32, len(arr))
		for i, v := range arr {
			if f, ok := v.(float64); ok {
				out[i] = int32(f)
			}
		}
		return out
	case "bigint", "int8":
		out := make([]int64, len(arr))
		for i, v := range arr {
			if f, ok := v.(float64); ok {
				out[i] = int64(f)
			}
		}
		return out
	case "real", "float4":
		out := make([]float32, len(arr))
		for i, v := range arr {
			if f, ok := v.(float64); ok {
				out[i] = float32(f)
			}
		}
		return out
	case "double precision", "float8":
		out := make([]float64, len(arr))
		for i, v := range arr {
			if f, ok := v.(float64); ok {
				out[i] = f
			}
		}
		return out
	case "text", "varchar", "character varying", "name":
		out := make([]string, len(arr))
		for i, v := range arr {
			if s, ok := v.(string); ok {
				out[i] = s
			}
		}
		return out
	case "boolean", "bool":
		out := make([]bool, len(arr))
		for i, v := range arr {
			if b, ok := v.(bool); ok {
				out[i] = b
			}
		}
		return out
	default:
		// For unrecognized types, convert to string slice and let the cast handle it.
		out := make([]string, len(arr))
		for i, v := range arr {
			out[i] = fmt.Sprint(v)
		}
		return out
	}
}

// coerceNumber converts a JSON float64 to the appropriate Go type for the given PG type.
func coerceNumber(f float64, pgType string) any {
	switch pgType {
	case "integer", "int4", "smallint", "int2":
		if f == math.Trunc(f) {
			return int64(f)
		}
	case "bigint", "int8":
		if f == math.Trunc(f) {
			return int64(f)
		}
	}
	return f
}
