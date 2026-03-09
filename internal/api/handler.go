package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the common interface satisfied by both *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// EventSink receives events for async processing (e.g., webhook delivery).
type EventSink interface {
	Enqueue(event *realtime.Event)
}

// Handler serves the auto-generated CRUD REST API.
type Handler struct {
	pool       *pgxpool.Pool
	schema     *schema.CacheHolder
	logger     *slog.Logger
	hub        *realtime.Hub // nil when realtime is unused
	dispatcher EventSink     // nil when webhooks are unused
}

// NewHandler creates a new API handler.
func NewHandler(pool *pgxpool.Pool, schemaCache *schema.CacheHolder, logger *slog.Logger, hub *realtime.Hub, dispatcher EventSink) *Handler {
	return &Handler{
		pool:       pool,
		schema:     schemaCache,
		logger:     logger,
		hub:        hub,
		dispatcher: dispatcher,
	}
}

// API limits to prevent abuse and overflow.
const (
	maxPage            = 100000 // cap page number to prevent integer overflow in offset
	maxFilterLen       = 10000  // max characters in filter expression
	maxSearchLen       = 1000   // max characters in search term
	maxSortFields      = 10     // max number of sort fields
	maxExpandRelations = 10     // max number of expand relations
)

// Routes returns a chi.Router with all CRUD routes mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/collections/{table}", func(r chi.Router) {
		r.Get("/", h.handleList)
		r.Post("/", h.handleCreate)
		r.Post("/batch", h.handleBatch)
		r.Get("/{id}", h.handleRead)
		r.Patch("/{id}", h.handleUpdate)
		r.Delete("/{id}", h.handleDelete)
	})

	r.Post("/rpc/{function}", h.handleRPC)

	return r
}

// withRLS returns a Querier for executing database operations. When JWT claims
// are present in the request context, it begins a transaction, sets RLS session
// variables, and returns the tx. The caller must invoke the returned cleanup
// function when done (commits the tx on success, rolls back on error).
// When no claims are present, returns the pool directly with a no-op cleanup.
func (h *Handler) withRLS(r *http.Request) (Querier, func(error), error) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		return h.pool, func(error) {}, nil
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		return nil, nil, err
	}

	if err := auth.SetRLSContext(r.Context(), tx, claims); err != nil {
		_ = tx.Rollback(r.Context())
		return nil, nil, err
	}

	done := func(queryErr error) {
		if queryErr != nil {
			_ = tx.Rollback(r.Context())
		} else {
			if err := tx.Commit(r.Context()); err != nil {
				h.logger.Error("tx commit failed", "error", err)
			}
		}
	}
	return tx, done, nil
}

// resolveTable looks up the table in the schema cache, validates it exists,
// and checks API key table scope restrictions.
func (h *Handler) resolveTable(w http.ResponseWriter, r *http.Request) *schema.Table {
	sc := h.schema.Get()
	if sc == nil {
		writeError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return nil
	}

	tableName := chi.URLParam(r, "table")
	tbl := sc.TableByName(tableName)
	if tbl == nil {
		writeError(w, http.StatusNotFound, "collection not found: "+tableName)
		return nil
	}

	// Check API key table restrictions.
	if err := auth.CheckTableScope(auth.ClaimsFromContext(r.Context()), tableName); err != nil {
		writeErrorWithDoc(w, http.StatusForbidden, "api key does not have access to table: "+tableName, docURL("/guide/api-reference"))
		return nil
	}

	return tbl
}

// requireWriteScope checks that the current API key scope permits write operations.
func requireWriteScope(w http.ResponseWriter, r *http.Request) bool {
	if err := auth.CheckWriteScope(auth.ClaimsFromContext(r.Context())); err != nil {
		writeErrorWithDoc(w, http.StatusForbidden, "api key scope does not permit write operations", docURL("/guide/api-reference"))
		return false
	}
	return true
}

// requireWritable checks that the table supports write operations (not a view).
func requireWritable(w http.ResponseWriter, tbl *schema.Table) bool {
	if tbl.Kind != "table" && tbl.Kind != "partitioned_table" {
		writeError(w, http.StatusMethodNotAllowed, "write operations not allowed on "+tbl.Kind)
		return false
	}
	return true
}

// requirePK checks that the table has a primary key for write operations.
func requirePK(w http.ResponseWriter, tbl *schema.Table) bool {
	if len(tbl.PrimaryKey) == 0 {
		writeError(w, http.StatusBadRequest, "table has no primary key")
		return false
	}
	return true
}

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
	if record == nil {
		done(nil)
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

	done(nil)
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

	done(nil)
	writeJSON(w, http.StatusCreated, record)
	h.publishEvent("create", tbl.Name, record)
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

	query, args := buildUpdate(tbl, data, pkValues)

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
		done(nil)
		writeError(w, http.StatusNotFound, "record not found")
		return
	}

	done(nil)
	writeJSON(w, http.StatusOK, record)
	h.publishEvent("update", tbl.Name, record)
}

// handleDelete handles DELETE /collections/{table}/{id}
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

	query, args := buildDelete(tbl, pkValues)

	q, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tag, err := q.Exec(r.Context(), query, args...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("delete error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	if tag.RowsAffected() == 0 {
		done(nil)
		writeError(w, http.StatusNotFound, "record not found")
		return
	}

	done(nil)
	w.WriteHeader(http.StatusNoContent)

	// Publish delete event with PK values.
	record := make(map[string]any, len(tbl.PrimaryKey))
	for i, pk := range tbl.PrimaryKey {
		record[pk] = pkValues[i]
	}
	h.publishEvent("delete", tbl.Name, record)
}

// handleList handles GET /collections/{table}
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	tbl := h.resolveTable(w, r)
	if tbl == nil {
		return
	}

	q := r.URL.Query()

	// Parse pagination.
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
	}
	perPage, _ := strconv.Atoi(q.Get("perPage"))
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 500 {
		perPage = 500
	}
	skipTotal := q.Get("skipTotal") == "true"

	// Parse fields.
	fields := parseFields(r)

	// Parse sort.
	sortSQL := parseSortSQL(tbl, q.Get("sort"))

	// Parse filter.
	var filterSQL string
	var filterArgs []any
	if filterStr := q.Get("filter"); filterStr != "" {
		if len(filterStr) > maxFilterLen {
			writeErrorWithDoc(w, http.StatusBadRequest, "filter expression too long", docURL("/guide/api-reference#filter-syntax"))
			return
		}
		var err error
		filterSQL, filterArgs, err = parseFilter(tbl, filterStr)
		if err != nil {
			writeErrorWithDoc(w, http.StatusBadRequest, "invalid filter: "+err.Error(), docURL("/guide/api-reference#filter-syntax"))
			return
		}
	}

	// Parse search (full-text search).
	var searchSQL, searchRank string
	var searchArgs []any
	if searchStr := strings.TrimSpace(q.Get("search")); searchStr != "" {
		if len(searchStr) > maxSearchLen {
			writeErrorWithDoc(w, http.StatusBadRequest, "search term too long", docURL("/guide/api-reference#full-text-search"))
			return
		}
		// Search arg index starts after all filter args.
		argOffset := len(filterArgs) + 1
		var err error
		searchSQL, searchRank, searchArgs, err = buildSearchSQL(tbl, searchStr, argOffset)
		if err != nil {
			writeErrorWithDoc(w, http.StatusBadRequest, "search not supported: "+err.Error(), docURL("/guide/api-reference#full-text-search"))
			return
		}
	}

	opts := listOpts{
		page:       page,
		perPage:    perPage,
		skipTotal:  skipTotal,
		fields:     fields,
		sortSQL:    sortSQL,
		filterSQL:  filterSQL,
		filterArgs: filterArgs,
		searchSQL:  searchSQL,
		searchRank: searchRank,
		searchArgs: searchArgs,
	}

	dataQuery, dataArgs, countQuery, countArgs := buildList(tbl, opts)

	querier, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Get total count (unless skipTotal).
	totalItems := -1
	totalPages := -1
	if !skipTotal {
		err := querier.QueryRow(r.Context(), countQuery, countArgs...).Scan(&totalItems)
		if err != nil {
			done(err)
			h.logger.Error("count error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		totalPages = int(math.Ceil(float64(totalItems) / float64(perPage)))
	}

	// Get data rows.
	rows, err := querier.Query(r.Context(), dataQuery, dataArgs...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("list error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	items, err := scanRows(rows)
	rows.Close() // Close before done() to avoid pgx "conn busy" on commit.
	if err != nil {
		done(err)
		h.logger.Error("scan error", "error", err, "table", tbl.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Handle expand if requested.
	if expandParam := q.Get("expand"); expandParam != "" && len(items) > 0 {
		sc := h.schema.Get()
		if sc != nil {
			expandRecords(r.Context(), querier, sc, tbl, items, expandParam, h.logger)
		}
	}

	done(nil)
	writeJSON(w, http.StatusOK, ListResponse{
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
		Items:      items,
	})
}

// publishEvent sends a realtime event to the hub and webhook dispatcher.
func (h *Handler) publishEvent(action, table string, record map[string]any) {
	if h.hub == nil && h.dispatcher == nil {
		return
	}
	event := &realtime.Event{
		Action: action,
		Table:  table,
		Record: record,
	}
	if h.hub != nil {
		h.hub.Publish(event)
	}
	if h.dispatcher != nil {
		h.dispatcher.Enqueue(event)
	}
}

// countKnownColumns returns the number of keys in data that match a column in the table schema.
func countKnownColumns(tbl *schema.Table, data map[string]any) int {
	n := 0
	for col := range data {
		if tbl.ColumnByName(col) != nil {
			n++
		}
	}
	return n
}

// parseFields extracts the fields query parameter.
func parseFields(r *http.Request) []string {
	f := r.URL.Query().Get("fields")
	if f == "" {
		return nil
	}
	parts := strings.Split(f, ",")
	fields := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			fields = append(fields, p)
		}
	}
	return fields
}

// parseSortSQL converts the sort parameter to a SQL ORDER BY clause.
// Format: "-created,+name" → "created" DESC, "name" ASC
func parseSortSQL(tbl *schema.Table, sortParam string) string {
	if sortParam == "" {
		return ""
	}

	parts := strings.Split(sortParam, ",")
	clauses := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		dir := "ASC"
		col := p
		if strings.HasPrefix(p, "-") {
			dir = "DESC"
			col = p[1:]
		} else if strings.HasPrefix(p, "+") {
			col = p[1:]
		}

		// Validate column exists in schema.
		if tbl.ColumnByName(col) == nil {
			continue
		}

		clauses = append(clauses, quoteIdent(col)+" "+dir)
		if len(clauses) >= maxSortFields {
			break
		}
	}

	return strings.Join(clauses, ", ")
}

// scanRow scans a single row from a pgx.Rows result using field descriptions.
// Returns nil if no rows are present.
func scanRow(rows pgx.Rows) (map[string]any, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	return scanCurrentRow(rows)
}

// scanRows scans all rows from a pgx.Rows result.
func scanRows(rows pgx.Rows) ([]map[string]any, error) {
	var result []map[string]any

	for rows.Next() {
		record, err := scanCurrentRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if result == nil {
		result = []map[string]any{}
	}
	return result, nil
}

// scanCurrentRow scans the current row into a map.
func scanCurrentRow(rows pgx.Rows) (map[string]any, error) {
	descs := rows.FieldDescriptions()
	values := make([]any, len(descs))
	ptrs := make([]any, len(descs))
	for i := range values {
		ptrs[i] = &values[i]
	}

	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}

	record := make(map[string]any, len(descs))
	for i, desc := range descs {
		record[desc.Name] = normalizeValue(values[i])
	}
	return record, nil
}

// normalizeValue converts pgx binary-protocol types into JSON-friendly forms.
// In particular, UUID columns scanned into `any` arrive as [16]byte; we convert
// them to the standard "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" string.
func normalizeValue(v any) any {
	switch val := v.(type) {
	case [16]byte:
		return fmt.Sprintf("%x-%x-%x-%x-%x", val[0:4], val[4:6], val[6:8], val[8:10], val[10:16])
	default:
		return v
	}
}
