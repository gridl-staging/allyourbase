package webhooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// webhookResponse is the API response shape — never exposes secret.
type webhookResponse struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	HasSecret bool     `json:"hasSecret"`
	Events    []string `json:"events"`
	Tables    []string `json:"tables"`
	Enabled   bool     `json:"enabled"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

func toResponse(w *Webhook) webhookResponse {
	return webhookResponse{
		ID:        w.ID,
		URL:       w.URL,
		HasSecret: w.Secret != "",
		Events:    w.Events,
		Tables:    w.Tables,
		Enabled:   w.Enabled,
		CreatedAt: w.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: w.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// Handler serves webhook CRUD HTTP endpoints.
type Handler struct {
	store     WebhookStore
	deliveryS DeliveryStore
	logger    *slog.Logger
}

// NewHandler creates a new webhook handler.
func NewHandler(store WebhookStore, deliveryStore DeliveryStore, logger *slog.Logger) *Handler {
	return &Handler{store: store, deliveryS: deliveryStore, logger: logger}
}

// Routes returns a chi.Router with webhook CRUD endpoints.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.handleList)
	r.Post("/", h.handleCreate)
	r.Get("/{id}", h.handleGet)
	r.Patch("/{id}", h.handleUpdate)
	r.Delete("/{id}", h.handleDelete)
	r.Post("/{id}/test", h.handleTest)
	r.Get("/{id}/deliveries", h.handleListDeliveries)
	r.Get("/{id}/deliveries/{deliveryId}", h.handleGetDelivery)
	return r
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	hooks, err := h.store.List(r.Context())
	if err != nil {
		h.logger.Error("list webhooks", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp := make([]webhookResponse, len(hooks))
	for i := range hooks {
		resp[i] = toResponse(&hooks[i])
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	hook, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("get webhook", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, toResponse(hook))
}

type webhookRequest struct {
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`
	Events  []string `json:"events"`
	Tables  []string `json:"tables"`
	Enabled *bool    `json:"enabled"`
}

var validEvents = map[string]bool{"create": true, "update": true, "delete": true}

func validateRequest(req *webhookRequest) string {
	if req.URL == "" {
		return "url is required"
	}
	for _, e := range req.Events {
		if !validEvents[e] {
			return "invalid event: " + e + " (must be create, update, or delete)"
		}
	}
	return ""
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req webhookRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if msg := validateRequest(&req); msg != "" {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, msg,
			"https://allyourbase.io/guide/api-reference")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	events := req.Events
	if len(events) == 0 {
		events = []string{"create", "update", "delete"}
	}
	tables := req.Tables
	if tables == nil {
		tables = []string{}
	}

	hook := &Webhook{
		URL:     req.URL,
		Secret:  req.Secret,
		Events:  events,
		Tables:  tables,
		Enabled: enabled,
	}
	if err := h.store.Create(r.Context(), hook); err != nil {
		h.logger.Error("create webhook", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, toResponse(hook))
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Load existing to merge.
	existing, err := h.store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("get webhook for update", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req webhookRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	// Apply partial updates.
	if req.URL != "" {
		existing.URL = req.URL
	}
	if req.Secret != "" {
		existing.Secret = req.Secret
	}
	if len(req.Events) > 0 {
		for _, e := range req.Events {
			if !validEvents[e] {
				httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid event: "+e+" (must be create, update, or delete)",
					"https://allyourbase.io/guide/api-reference")
				return
			}
		}
		existing.Events = req.Events
	}
	if req.Tables != nil {
		existing.Tables = req.Tables
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	if err := h.store.Update(r.Context(), id, existing); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("update webhook", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, toResponse(existing))
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	err := h.store.Delete(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("delete webhook", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type testResponse struct {
	Success    bool   `json:"success"`
	StatusCode int    `json:"statusCode,omitempty"`
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

func (h *Handler) handleTest(w http.ResponseWriter, r *http.Request) {
	hook, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("get webhook for test", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	event := &realtime.Event{
		Action: "test",
		Table:  "_ayb_test",
		Record: map[string]any{"message": "Test event from Allyourbase"},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("marshal test payload", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, hook.URL, bytes.NewReader(payload))
	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, testResponse{
			Success: false,
			Error:   "invalid webhook URL",
		})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if hook.Secret != "" {
		req.Header.Set("X-AYB-Signature", Sign(hook.Secret, payload))
	}

	start := time.Now()
	resp, err := client.Do(req)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, testResponse{
			Success:    false,
			DurationMs: durationMs,
			Error:      err.Error(),
		})
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	httputil.WriteJSON(w, http.StatusOK, testResponse{
		Success:    resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode: resp.StatusCode,
		DurationMs: durationMs,
	})
}

// --- Delivery log endpoints ---

type deliveryListResponse struct {
	Items      []Delivery `json:"items"`
	Page       int        `json:"page"`
	PerPage    int        `json:"perPage"`
	TotalItems int        `json:"totalItems"`
	TotalPages int        `json:"totalPages"`
}

func (h *Handler) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	webhookID := chi.URLParam(r, "id")

	// Verify webhook exists.
	if _, err := h.store.Get(r.Context(), webhookID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("get webhook for deliveries", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	items, total, err := h.deliveryS.ListDeliveries(r.Context(), webhookID, page, perPage)
	if err != nil {
		h.logger.Error("list deliveries", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	httputil.WriteJSON(w, http.StatusOK, deliveryListResponse{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
	})
}

func (h *Handler) handleGetDelivery(w http.ResponseWriter, r *http.Request) {
	webhookID := chi.URLParam(r, "id")
	deliveryID := chi.URLParam(r, "deliveryId")

	del, err := h.deliveryS.GetDelivery(r.Context(), webhookID, deliveryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "delivery not found")
			return
		}
		h.logger.Error("get delivery", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, del)
}
