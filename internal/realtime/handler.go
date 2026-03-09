package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler serves the SSE realtime endpoint.
type Handler struct {
	hub         *Hub
	pool        *pgxpool.Pool // nil when RLS filtering unavailable
	authSvc     *auth.Service // nil when auth disabled
	schemaCache *schema.CacheHolder
	logger      *slog.Logger
}

// NewHandler creates a new realtime SSE handler.
// pool may be nil; when non-nil, events are filtered per-client via RLS.
func NewHandler(hub *Hub, pool *pgxpool.Pool, authSvc *auth.Service, schemaCache *schema.CacheHolder, logger *slog.Logger) *Handler {
	return &Handler{
		hub:         hub,
		pool:        pool,
		authSvc:     authSvc,
		schemaCache: schemaCache,
		logger:      logger,
	}
}

// ServeHTTP handles GET /api/realtime with Server-Sent Events.
//
// Query parameters:
//   - tables: comma-separated table names to subscribe to (required unless oauth=true)
//   - token: JWT token (alternative to Authorization header for EventSource compatibility)
//   - oauth: when "true", creates an OAuth SSE channel (no auth required, no tables needed)
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// OAuth SSE mode: no auth required, creates a one-time channel for OAuth result.
	if r.URL.Query().Get("oauth") == "true" {
		h.serveOAuthSSE(w, r, flusher)
		return
	}

	// Authenticate when auth is enabled.
	var claims *auth.Claims
	if h.authSvc != nil {
		token := extractToken(r)
		if token == "" {
			httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized, "authentication required",
				"https://allyourbase.io/guide/realtime")
			return
		}
		var err error
		// Support both JWT tokens and API keys (ayb_ prefix).
		if auth.IsAPIKey(token) {
			claims, err = h.authSvc.ValidateAPIKey(r.Context(), token)
		} else {
			claims, err = h.authSvc.ValidateToken(token)
		}
		if err != nil {
			httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized, "invalid or expired token",
				"https://allyourbase.io/guide/realtime")
			return
		}
	}

	// Parse and validate table subscriptions.
	tablesParam := r.URL.Query().Get("tables")
	if tablesParam == "" {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "tables parameter is required",
			"https://allyourbase.io/guide/realtime")
		return
	}

	tables := make(map[string]bool)
	sc := h.schemaCache.Get()
	for _, name := range strings.Split(tablesParam, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if sc != nil && sc.TableByName(name) == nil {
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "unknown table: "+name,
				"https://allyourbase.io/guide/realtime")
			return
		}
		tables[name] = true
	}
	if len(tables) == 0 {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "at least one valid table is required",
			"https://allyourbase.io/guide/realtime")
		return
	}

	// Subscribe and ensure cleanup on disconnect.
	client := h.hub.Subscribe(tables)
	defer h.hub.Unsubscribe(client.ID)

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	// Send initial connected event.
	fmt.Fprintf(w, "event: connected\ndata: {\"clientId\":%q}\n\n", client.ID)
	flusher.Flush()

	h.logger.Info("realtime client connected", "clientID", client.ID, "tables", tablesParam)

	// Stream events until the client disconnects.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-client.Events():
			if !open {
				return
			}
			if !h.canSeeRecord(ctx, claims, event) {
				continue
			}
			data, err := json.Marshal(event)
			if err != nil {
				h.logger.Error("failed to marshal event", "error", err, "clientID", client.ID)
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// serveOAuthSSE handles the OAuth-specific SSE endpoint.
// No authentication is required (the user hasn't logged in yet).
// The client's ID is used as the CSRF state for the OAuth popup flow.
func (h *Handler) serveOAuthSSE(w http.ResponseWriter, r *http.Request, flusher http.Flusher) {
	client := h.hub.SubscribeOAuth()
	defer h.hub.Unsubscribe(client.ID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send clientId — the SDK uses this as the OAuth state parameter.
	fmt.Fprintf(w, "event: connected\ndata: {\"clientId\":%q}\n\n", client.ID)
	flusher.Flush()

	h.logger.Info("oauth SSE client connected", "clientID", client.ID)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case oauthEvent, open := <-client.OAuthEvents():
			if !open {
				return
			}
			data, err := json.Marshal(oauthEvent)
			if err != nil {
				h.logger.Error("failed to marshal oauth event", "error", err, "clientID", client.ID)
				continue
			}
			fmt.Fprintf(w, "event: oauth\ndata: %s\n\n", data)
			flusher.Flush()
			return // OAuth flow is one-shot; close after delivering the result.
		}
	}
}

// canSeeRecord checks whether the authenticated user can see the event's record
// via an RLS-scoped SELECT. This per-event SELECT is evaluated by Postgres
// under the ayb_authenticated role, so full RLS policy logic applies, including
// join/EXISTS-based policies on related tables.
//
// Returns true when:
//   - no pool is available (RLS filtering disabled)
//   - no claims (unauthenticated client, no RLS applies)
//   - the event is a delete (record is gone, can't verify)
//   - the RLS-scoped SELECT finds the row
func (h *Handler) canSeeRecord(ctx context.Context, claims *auth.Claims, event *Event) bool {
	if h.pool == nil || claims == nil || event.Action == "delete" {
		return true
	}

	sc := h.schemaCache.Get()
	if sc == nil {
		return true
	}
	tbl := sc.TableByName(event.Table)
	if tbl == nil || len(tbl.PrimaryKey) == 0 {
		return true
	}

	query, args := buildVisibilityCheck(tbl, event.Record)
	if query == "" {
		return true // missing PK values in record
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.logger.Error("rls filter: begin tx", "error", err)
		return false // fail closed
	}
	defer tx.Rollback(ctx)

	if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
		h.logger.Error("rls filter: set rls context", "error", err)
		return false
	}

	var one int
	err = tx.QueryRow(ctx, query, args...).Scan(&one)
	return err == nil
}

// buildVisibilityCheck builds a SELECT 1 query scoped to a row's PK.
// Returns ("", nil) if the record is missing any PK column value.
func buildVisibilityCheck(tbl *schema.Table, record map[string]any) (string, []any) {
	args := make([]any, 0, len(tbl.PrimaryKey))
	var sb strings.Builder
	sb.WriteString("SELECT 1 FROM ")
	sb.WriteString(quoteIdent(tbl.Schema))
	sb.WriteByte('.')
	sb.WriteString(quoteIdent(tbl.Name))
	sb.WriteString(" WHERE ")

	for i, pk := range tbl.PrimaryKey {
		v, ok := record[pk]
		if !ok {
			return "", nil
		}
		if i > 0 {
			sb.WriteString(" AND ")
		}
		sb.WriteString(quoteIdent(pk))
		sb.WriteString(" = $")
		sb.WriteString(strconv.Itoa(i + 1))
		args = append(args, v)
	}
	return sb.String(), args
}

// quoteIdent quotes a SQL identifier with double quotes.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// extractToken gets the JWT from the Authorization header or token query parameter.
// EventSource (browser SSE API) does not support custom headers, so the query
// parameter provides an alternative authentication path.
func extractToken(r *http.Request) string {
	if token, ok := httputil.ExtractBearerToken(r); ok {
		return token
	}
	return r.URL.Query().Get("token")
}
