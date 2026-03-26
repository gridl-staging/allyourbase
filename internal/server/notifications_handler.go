// Package server Handlers for the notifications API, supporting creation, listing, and mark-as-read operations for user notifications.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/notifications"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/go-chi/chi/v5"
)

// notificationAdmin is the interface for notification CRUD operations.
type notificationAdmin interface {
	Create(ctx context.Context, userID, title, body string, metadata map[string]any, channel string) (*notifications.Notification, error)
	ListByUser(ctx context.Context, userID string, unreadOnly bool, page, perPage int) ([]*notifications.Notification, int, error)
	GetByID(ctx context.Context, id, userID string) (*notifications.Notification, error)
	MarkRead(ctx context.Context, id, userID string) error
	MarkAllRead(ctx context.Context, userID string) (int64, error)
}

type notificationsCreateRequest struct {
	UserID   string         `json:"user_id"`
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	Metadata map[string]any `json:"metadata"`
	Channel  string         `json:"channel"`
}

func (s *Server) notificationsNotConfigured(w http.ResponseWriter) bool {
	if s.notifSvc != nil {
		return false
	}
	httputil.WriteError(w, http.StatusNotImplemented, "notifications service is not configured")
	return true
}

// Lists notifications for the authenticated user with pagination and filtering. Accepts query parameters for page, perPage, and unread to filter results. Returns paginated results with total item and page counts.
func (s *Server) handleNotificationsList(w http.ResponseWriter, r *http.Request) {
	if s.notificationsNotConfigured(w) {
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			httputil.WriteError(w, http.StatusBadRequest, "invalid page")
			return
		}
		page = v
	}

	perPage := 20
	if raw := r.URL.Query().Get("perPage"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			httputil.WriteError(w, http.StatusBadRequest, "invalid perPage")
			return
		}
		perPage = v
	}

	unreadOnly := false
	if raw := r.URL.Query().Get("unread"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid unread")
			return
		}
		unreadOnly = v
	}

	items, total, err := s.notifSvc.ListByUser(r.Context(), claims.Subject, unreadOnly, page, perPage)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + perPage - 1) / perPage
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"page":       page,
		"perPage":    perPage,
		"totalItems": total,
		"totalPages": totalPages,
		"items":      items,
	})
}

// Creates a notification for the specified user. Validates required fields (user_id and title), publishes a realtime event if the hub is configured, and returns the created notification with HTTP 201 status.
func (s *Server) handleNotificationsCreate(w http.ResponseWriter, r *http.Request) {
	if s.notificationsNotConfigured(w) {
		return
	}

	var req notificationsCreateRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.UserID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if req.Title == "" {
		httputil.WriteError(w, http.StatusBadRequest, "title is required")
		return
	}

	n, err := s.notifSvc.Create(r.Context(), req.UserID, req.Title, req.Body, req.Metadata, req.Channel)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create notification")
		return
	}

	if s.hub != nil {
		record := map[string]any{}
		b, err := json.Marshal(n)
		if err == nil {
			_ = json.Unmarshal(b, &record)
		}
		if len(record) == 0 {
			record = map[string]any{
				"id":         n.ID,
				"user_id":    n.UserID,
				"title":      n.Title,
				"body":       n.Body,
				"metadata":   n.Metadata,
				"channel":    n.Channel,
				"read_at":    n.ReadAt,
				"created_at": n.CreatedAt,
			}
		}
		s.hub.Publish(&realtime.Event{Action: "create", Table: "_ayb_notifications", Record: record})
	}

	httputil.WriteJSON(w, http.StatusCreated, n)
}

// Marks a specific notification as read for the authenticated user. Verifies the notification exists and belongs to the user before updating it.
func (s *Server) handleNotificationMarkRead(w http.ResponseWriter, r *http.Request) {
	if s.notificationsNotConfigured(w) {
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "id is required")
		return
	}

	if _, err := s.notifSvc.GetByID(r.Context(), id, claims.Subject); err != nil {
		if errors.Is(err, notifications.ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "notification not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to fetch notification")
		return
	}

	if err := s.notifSvc.MarkRead(r.Context(), id, claims.Subject); err != nil {
		if errors.Is(err, notifications.ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "notification not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to mark notification read")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Marks all notifications as read for the authenticated user. Returns the count of updated notifications.
func (s *Server) handleNotificationMarkAllRead(w http.ResponseWriter, r *http.Request) {
	if s.notificationsNotConfigured(w) {
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	n, err := s.notifSvc.MarkAllRead(r.Context(), claims.Subject)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to mark notifications read")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]int64{"updated": n})
}
