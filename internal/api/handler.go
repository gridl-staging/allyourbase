// Package api Handler serves the auto-generated CRUD REST API, providing HTTP handlers for list, create, read, update, delete, import, export, and other operations on database tables.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/replica"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

// hubPublisher is the narrow interface for publishing realtime events.
// *realtime.Hub satisfies this interface; tests can supply a stub or nil.
type hubPublisher interface {
	Publish(event *realtime.Event)
}

func normalizeHubPublisher(hub hubPublisher) hubPublisher {
	if hub == nil {
		return nil
	}
	value := reflect.ValueOf(hub)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil
		}
	}
	return hub
}

// EmbedFunc embeds text inputs into vectors. Used for semantic search.
type EmbedFunc func(ctx context.Context, texts []string) ([][]float64, error)

// HandlerOption configures optional Handler features.
type HandlerOption func(*Handler)

// WithEmbedder enables semantic search by providing an embedding function.
func WithEmbedder(fn EmbedFunc) HandlerOption {
	return func(h *Handler) { h.embedFn = fn }
}

// WithConfiguredEmbeddingDimension sets an expected embedding dimension derived
// from configuration for the selected provider/model pair.
func WithConfiguredEmbeddingDimension(dim int) HandlerOption {
	return func(h *Handler) { h.configEmbeddingDim = dim }
}

// WithAPILimits overrides runtime API limits and feature flags.
func WithAPILimits(cfg config.APIConfig) HandlerOption {
	return func(h *Handler) {
		h.apiCfg = cfg
	}
}

// WithPoolRouter wires optional read-routing for unauthenticated read-only requests.
// Nil routers are ignored to preserve existing behavior.
func WithPoolRouter(router *replica.PoolRouter) HandlerOption {
	return func(h *Handler) {
		if router == nil {
			return
		}
		h.poolRouter = router
	}
}

// Handler serves the auto-generated CRUD REST API.
type Handler struct {
	pool               *pgxpool.Pool
	poolRouter         *replica.PoolRouter
	schema             *schema.CacheHolder
	logger             *slog.Logger
	hub                hubPublisher // nil when realtime is unused
	dispatcher         EventSink    // nil when webhooks are unused
	auditSink          audit.Sink   // nil when audit logging is disabled
	apiCfg             config.APIConfig
	fieldEncryptor     *FieldEncryptor
	embedFn            EmbedFunc // nil when semantic search is not configured
	configEmbeddingDim int
}

type txFinalizer interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// NewHandler creates a new API handler.
func NewHandler(pool *pgxpool.Pool, schemaCache *schema.CacheHolder, logger *slog.Logger, hub hubPublisher, dispatcher EventSink, auditSink audit.Sink, fieldEncryptors ...*FieldEncryptor) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	apiCfg := config.Default().API
	var fieldEncryptor *FieldEncryptor
	if len(fieldEncryptors) > 0 {
		fieldEncryptor = fieldEncryptors[0]
	}
	return &Handler{
		pool:           pool,
		schema:         schemaCache,
		apiCfg:         apiCfg,
		logger:         logger,
		hub:            normalizeHubPublisher(hub),
		dispatcher:     dispatcher,
		auditSink:      auditSink,
		fieldEncryptor: fieldEncryptor,
	}
}

// effectiveAPIConfig returns the effective API configuration by applying defaults to any unset numeric limits (ImportMaxSizeMB, ImportMaxRows, ExportMaxRows). When only some limits are configured without explicitly setting AggregateEnabled, the default aggregate behavior is preserved to maintain backward compatibility.
func (h *Handler) effectiveAPIConfig() config.APIConfig {
	defaults := config.Default().API
	apiCfg := h.apiCfg
	if apiCfg.ImportMaxSizeMB <= 0 {
		apiCfg.ImportMaxSizeMB = defaults.ImportMaxSizeMB
	}
	if apiCfg.ImportMaxRows <= 0 {
		apiCfg.ImportMaxRows = defaults.ImportMaxRows
	}
	if apiCfg.ExportMaxRows <= 0 {
		apiCfg.ExportMaxRows = defaults.ExportMaxRows
	}
	// When callers partially override numeric limits with WithAPILimits and omit
	// AggregateEnabled, preserve the default aggregate behavior.
	if !h.apiCfg.AggregateEnabled {
		configuredLimits := configuredAPILimitCount(h.apiCfg)
		if configuredLimits > 0 && configuredLimits < 3 {
			apiCfg.AggregateEnabled = defaults.AggregateEnabled
		}
	}
	return apiCfg
}

func configuredAPILimitCount(cfg config.APIConfig) int {
	count := 0
	if cfg.ImportMaxSizeMB > 0 {
		count++
	}
	if cfg.ImportMaxRows > 0 {
		count++
	}
	if cfg.ExportMaxRows > 0 {
		count++
	}
	return count
}

// ApplyOptions applies HandlerOptions after construction.
func (h *Handler) ApplyOptions(opts ...HandlerOption) {
	for _, o := range opts {
		o(h)
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
		r.With(middleware.AllowContentType("application/json")).Post("/", h.handleCreate)
		r.With(middleware.AllowContentType("application/json")).Post("/batch", h.handleBatch)
		r.Get("/export.csv", h.handleExportCSV)
		r.Get("/export.json", h.handleExportJSON)
		r.With(middleware.AllowContentType("application/json", "text/csv")).Post("/import", h.handleImport)
		r.Get("/{id}", h.handleRead)
		r.With(middleware.AllowContentType("application/json")).Patch("/{id}", h.handleUpdate)
		r.Delete("/{id}", h.handleDelete)
	})

	r.With(middleware.AllowContentType("application/json")).Post("/rpc/{function}", h.handleRPC)

	return r
}

func (h *Handler) beginTx(ctx context.Context) (pgx.Tx, error) {
	if requestConn := tenant.RequestConnFromContext(ctx); requestConn != nil {
		return requestConn.Begin(ctx)
	}
	if h.pool == nil {
		return nil, fmt.Errorf("database pool is not configured")
	}
	return h.pool.Begin(ctx)
}

// withRLS returns a Querier for executing database operations. When JWT claims
// are present in the request context, it begins a transaction, sets RLS session
// variables, and returns the tx. The caller must invoke the returned cleanup
// function when done (commits the tx on success, rolls back on error).
// When no claims are present, returns the pool directly with a no-op cleanup.
func (h *Handler) withRLS(r *http.Request) (Querier, func(error) error, error) {
	if requestConn := tenant.RequestConnFromContext(r.Context()); requestConn != nil && auth.ClaimsFromContext(r.Context()) == nil {
		return requestConn, func(error) error { return nil }, nil
	}
	if h.pool == nil && tenant.RequestConnFromContext(r.Context()) == nil {
		return nil, nil, fmt.Errorf("database pool is not configured")
	}
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		if h.poolRouter != nil && replica.IsReadOnly(r.Context()) {
			return h.poolRouter.ReadPool(), func(error) error { return nil }, nil
		}
		return h.pool, func(error) error { return nil }, nil
	}

	tx, err := h.beginTx(r.Context())
	if err != nil {
		return nil, nil, err
	}

	if err := auth.SetRLSContext(r.Context(), tx, claims); err != nil {
		_ = tx.Rollback(r.Context())
		return nil, nil, err
	}

	done := func(queryErr error) error { return finalizeTx(r.Context(), tx, queryErr, h.logger) }
	return tx, done, nil
}

func finalizeTx(ctx context.Context, tx txFinalizer, queryErr error, logger *slog.Logger) error {
	if queryErr != nil {
		if err := tx.Rollback(ctx); err != nil && logger != nil {
			logger.Error("tx rollback failed", "error", err)
		}
		return nil
	}
	if err := tx.Commit(ctx); err != nil {
		if logger != nil {
			logger.Error("tx commit failed", "error", err)
		}
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
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
