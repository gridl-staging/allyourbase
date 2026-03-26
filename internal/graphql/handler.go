// Package graphql Handler serves GraphQL queries and subscriptions over HTTP and WebSocket, with support for transaction-based mutations, realtime event delivery filtered by row-level security, and query complexity analysis.
package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	gql "github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/gqlerrors"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
)

// graphQLRequest is the expected JSON body for a GraphQL POST request.
type graphQLRequest struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

// graphQLResponse is the JSON response envelope per the GraphQL spec.
type graphQLResponse struct {
	Data   interface{} `json:"data"`
	Errors interface{} `json:"errors,omitempty"`
}

// Handler serves GraphQL queries over HTTP.
type Handler struct {
	pool          *pgxpool.Pool
	cacheHolder   *schema.CacheHolder        // used for realtime RLS visibility checks
	cache         *schema.SchemaCache        // used directly if set (tests)
	cacheGet      func() *schema.SchemaCache // used via CacheHolder (production)
	logger        *slog.Logger
	isAdmin       func(r *http.Request) bool // nil means no introspection gating
	maxDepth      int
	maxComplexity int
	wsHandler     *GQLWSHandler
	hub           *realtime.Hub

	subMu  sync.Mutex
	wsSubs map[string]map[string]*gqlwsSubscriptionState
}

// NewHandler creates a new Handler initialized with the database pool, schema cache holder, and logger for serving GraphQL queries and subscriptions.
func NewHandler(pool *pgxpool.Pool, cacheHolder *schema.CacheHolder, logger *slog.Logger) *Handler {
	wsHandler := NewGQLWSHandler(
		pool,
		cacheHolder.Get,
		logger,
	)
	h := &Handler{
		pool:        pool,
		cacheHolder: cacheHolder,
		cacheGet:    cacheHolder.Get,
		logger:      logger,
		wsHandler:   wsHandler,
		wsSubs:      make(map[string]map[string]*gqlwsSubscriptionState),
	}
	wsHandler.OnSubscribe = h.onWSSubscribe
	wsHandler.OnComplete = h.onWSComplete
	wsHandler.OnDisconnect = h.onWSDisconnect
	return h
}

// SetAdminChecker sets the function used to gate introspection queries.
// When set, introspection is only allowed for requests where isAdmin returns true.
func (h *Handler) SetAdminChecker(fn func(r *http.Request) bool) {
	h.isAdmin = fn
}

// SetLimits configures pre-execution depth/complexity analysis.
// A value <= 0 disables the corresponding analysis.
func (h *Handler) SetLimits(maxDepth, maxComplexity int) {
	h.maxDepth = maxDepth
	h.maxComplexity = maxComplexity
}

func (h *Handler) SetAuthValidator(v gqlwsAuthValidator) {
	if h.wsHandler == nil {
		return
	}
	h.wsHandler.SetAuthValidator(v)
}

func (h *Handler) SetHub(hub *realtime.Hub) {
	h.hub = hub
}

// getCache returns the current schema cache, preferring a direct reference over the getter.
func (h *Handler) getCache() *schema.SchemaCache {
	if h.cache != nil {
		return h.cache
	}
	if h.cacheGet != nil {
		return h.cacheGet()
	}
	return nil
}

// isIntrospectionQuery returns true if the query string contains introspection fields.
func isIntrospectionQuery(query string) bool {
	return strings.Contains(query, "__schema") || strings.Contains(query, "__type")
}

// selectedOperationDefinition returns the operation definition from a GraphQL document, selected by name if specified or returning the sole operation if only one exists.
func selectedOperationDefinition(doc *ast.Document, operationName string) *ast.OperationDefinition {
	if doc == nil {
		return nil
	}

	operations := make([]*ast.OperationDefinition, 0)
	for _, definition := range doc.Definitions {
		op, ok := definition.(*ast.OperationDefinition)
		if !ok {
			continue
		}
		operations = append(operations, op)
	}
	if len(operations) == 0 {
		return nil
	}

	if operationName != "" {
		for _, op := range operations {
			if op.Name != nil && op.Name.Value == operationName {
				return op
			}
		}
		return nil
	}
	if len(operations) == 1 {
		return operations[0]
	}
	return nil
}

func analysisDocForOperation(doc *ast.Document, operationName string) *ast.Document {
	selected := selectedOperationDefinition(doc, operationName)
	if selected == nil {
		return nil
	}

	definitions := make([]ast.Node, 0, len(doc.Definitions))
	definitions = append(definitions, selected)
	for _, definition := range doc.Definitions {
		if _, ok := definition.(*ast.FragmentDefinition); ok {
			definitions = append(definitions, definition)
		}
	}
	return &ast.Document{Definitions: definitions}
}

func isMutationDoc(doc *ast.Document, operationName string) bool {
	selected := selectedOperationDefinition(doc, operationName)
	return selected != nil && selected.Operation == ast.OperationTypeMutation
}

// ServeHTTP implements http.Handler to serve GraphQL queries over HTTP POST and handle WebSocket upgrade requests, with support for transaction-based mutations and row-level security.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.handleWebSocketUpgrade(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		http.Error(w, "websocket upgrade required for GET /graphql", http.StatusMethodNotAllowed)
		return
	}

	req, ok := decodeGraphQLRequest(w, r)
	if !ok {
		return
	}
	if !h.authorizeIntrospection(w, r, req.Query) {
		return
	}

	parsedDoc, ok := h.parseAndValidateQuery(w, req)
	if !ok {
		return
	}

	cache := h.getCache()
	schemaForRequest, ok := h.buildRequestSchema(w, cache)
	if !ok {
		return
	}

	executionContext, tx, ok := h.prepareExecutionContext(w, r.Context(), parsedDoc, req.OperationName, cache)
	if !ok {
		return
	}
	if tx != nil {
		defer tx.Rollback(executionContext) //nolint:errcheck
	}

	result := gql.Do(gql.Params{
		Schema:         *schemaForRequest,
		RequestString:  req.Query,
		VariableValues: req.Variables,
		OperationName:  req.OperationName,
		Context:        executionContext,
	})
	if tx != nil {
		result = h.finalizeMutationTransaction(executionContext, tx, result)
	}
	h.writeGraphQLResponse(w, result)
}

func (h *Handler) handleWebSocketUpgrade(w http.ResponseWriter, r *http.Request) bool {
	if !websocket.IsWebSocketUpgrade(r) {
		return false
	}
	if h.wsHandler != nil {
		h.wsHandler.ServeHTTP(w, r)
		return true
	}
	http.Error(w, "websocket not available", http.StatusNotImplemented)
	return true
}

func decodeGraphQLRequest(w http.ResponseWriter, r *http.Request) (graphQLRequest, bool) {
	var req graphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return graphQLRequest{}, false
	}
	return req, true
}

func (h *Handler) authorizeIntrospection(w http.ResponseWriter, r *http.Request, query string) bool {
	if h.isAdmin == nil || !isIntrospectionQuery(query) || h.isAdmin(r) {
		return true
	}
	writeJSONError(w, http.StatusForbidden, "introspection requires admin access")
	return false
}

// parseAndValidateQuery parses the GraphQL query and validates it by performing depth and complexity analysis based on configured limits.
func (h *Handler) parseAndValidateQuery(w http.ResponseWriter, req graphQLRequest) (*ast.Document, bool) {
	parsedDoc, err := parser.Parse(parser.ParseParams{Source: req.Query})
	if err != nil {
		return nil, true
	}
	analysisDoc := analysisDocForOperation(parsedDoc, req.OperationName)
	if analysisDoc == nil {
		return parsedDoc, true
	}
	if h.maxDepth > 0 {
		if err := CheckDepth(analysisDoc, h.maxDepth); err != nil {
			writeGraphQLErrorResponse(w, err)
			return nil, false
		}
	}
	if h.maxComplexity > 0 {
		complexitySchema, err := BuildSchemaWithFactories(h.getCache(), nil, nil, nil)
		if err != nil {
			h.logger.Error("failed to build GraphQL schema for complexity analysis", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "schema build error")
			return nil, false
		}
		if _, err := checkComplexityWithVariables(analysisDoc, complexitySchema, h.maxComplexity, req.Variables); err != nil {
			writeGraphQLErrorResponse(w, err)
			return nil, false
		}
	}
	return parsedDoc, true
}

// buildRequestSchema builds the GraphQL schema for the request by creating resolver factories for query, mutation, and relationship fields based on the database pool and schema cache.
func (h *Handler) buildRequestSchema(w http.ResponseWriter, cache *schema.SchemaCache) (*gql.Schema, bool) {
	var queryResolverFactory ResolverFactory
	var mutationResolverFactoryFn MutationResolverFactory
	var relationshipFactory RelationshipResolverFactory
	if h.pool != nil {
		pool := h.pool
		queryResolverFactory = func(tbl *schema.Table, schemaCache *schema.SchemaCache) gql.FieldResolveFn {
			return func(p gql.ResolveParams) (interface{}, error) {
				return resolveTable(p.Context, tbl, pool, schemaCache, p.Args)
			}
		}
		mutationResolverFactoryFn = mutationResolverFactory(pool)
		relationshipFactory = relationshipResolverFactory(pool, cache)
	}
	graphQLSchema, err := BuildSchemaWithFactories(cache, queryResolverFactory, mutationResolverFactoryFn, relationshipFactory)
	if err != nil {
		h.logger.Error("failed to build GraphQL schema", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "schema build error")
		return nil, false
	}
	return graphQLSchema, true
}

// prepareExecutionContext prepares the execution context for a GraphQL request by starting a database transaction for mutations, setting row-level security context, enabling mutation event collection, and initializing a dataloader.
func (h *Handler) prepareExecutionContext(w http.ResponseWriter, baseContext context.Context, parsedDoc *ast.Document, operationName string, cache *schema.SchemaCache) (context.Context, pgx.Tx, bool) {
	executionContext := baseContext
	var tx pgx.Tx
	if h.pool != nil && isMutationDoc(parsedDoc, operationName) {
		startedTx, err := h.pool.Begin(executionContext)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to begin mutation transaction")
			return nil, nil, false
		}
		tx = startedTx
		if claims := auth.ClaimsFromContext(executionContext); claims != nil {
			if err := auth.SetRLSContext(executionContext, tx, claims); err != nil {
				tx.Rollback(executionContext) //nolint:errcheck
				writeJSONError(w, http.StatusInternalServerError, "failed to set mutation RLS context")
				return nil, nil, false
			}
		}
		executionContext = ctxWithTx(executionContext, tx)
	}
	if h.hub != nil {
		executionContext = ctxWithMutationEventCollector(executionContext)
	}
	executionContext = ctxWithDataloader(executionContext, NewDataloader(h.pool, cache))
	return executionContext, tx, true
}

func (h *Handler) finalizeMutationTransaction(ctx context.Context, tx pgx.Tx, result *gql.Result) *gql.Result {
	if len(result.Errors) > 0 {
		tx.Rollback(ctx) //nolint:errcheck
		return result
	}
	if err := tx.Commit(ctx); err != nil {
		return &gql.Result{
			Errors: gqlerrors.FormatErrors(fmt.Errorf("commit transaction: %w", err)),
		}
	}
	h.publishCollectedMutationEvents(ctx)
	return result
}

func (h *Handler) writeGraphQLResponse(w http.ResponseWriter, result *gql.Result) {
	resp := graphQLResponse{Data: result.Data}
	if len(result.Errors) > 0 {
		resp.Errors = result.Errors
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("failed to write GraphQL response", "error", err)
	}
}

func writeGraphQLErrorResponse(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	resp := graphQLResponse{
		Data:   nil,
		Errors: gqlerrors.FormatErrors(err),
	}
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (h *Handler) publishCollectedMutationEvents(ctx context.Context) {
	if h.hub == nil {
		return
	}
	for _, event := range mutationEventsFromContext(ctx) {
		h.hub.Publish(event)
	}
}

// writeJSONError writes GraphQL-spec compliant error responses using the
// {errors: [{message: ...}]} envelope format. This is intentionally different
// from httputil.WriteError which uses the REST {code, message} format.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := graphQLResponse{
		Errors: []map[string]string{{"message": message}},
	}
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}
