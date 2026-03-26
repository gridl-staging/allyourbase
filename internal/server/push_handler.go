// Package server Provides HTTP handlers for push notification device registration, management, and message delivery. Supports both user-facing endpoints for personal device management and admin endpoints for bulk operations.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/push"
	"github.com/go-chi/chi/v5"
)

// pushAdmin is the interface for push notification admin/user operations.
type pushAdmin interface {
	RegisterToken(ctx context.Context, appID, userID, provider, platform, token, deviceName string) (*push.DeviceToken, error)
	RevokeToken(ctx context.Context, tokenID string) error
	ListUserTokens(ctx context.Context, appID, userID string) ([]*push.DeviceToken, error)
	ListTokens(ctx context.Context, appID, userID string, includeInactive bool) ([]*push.DeviceToken, error)
	GetToken(ctx context.Context, id string) (*push.DeviceToken, error)
	SendToUser(ctx context.Context, appID, userID, title, body string, data map[string]string) ([]*push.PushDelivery, error)
	SendToToken(ctx context.Context, tokenID, title, body string, data map[string]string) (*push.PushDelivery, error)
	ListDeliveries(ctx context.Context, appID, userID, status string, limit, offset int) ([]*push.PushDelivery, error)
	GetDelivery(ctx context.Context, id string) (*push.PushDelivery, error)
}

func mapPushError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, push.ErrNotFound):
		httputil.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, push.ErrInvalidProvider), errors.Is(err, push.ErrInvalidPlatform),
		errors.Is(err, push.ErrInvalidToken), errors.Is(err, push.ErrInvalidPayload),
		errors.Is(err, push.ErrPayloadTooLarge):
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}

// --- User-facing endpoints ---

type userPushRegisterRequest struct {
	AppID      string `json:"app_id"`
	Provider   string `json:"provider"`
	Platform   string `json:"platform"`
	Token      string `json:"token"`
	DeviceName string `json:"device_name"`
}

// handleUserPushRegister registers a push notification device token for the authenticated user.
func handleUserPushRegister(svc pushAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			httputil.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		var req userPushRegisterRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.AppID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "app_id is required")
			return
		}
		if req.Token == "" {
			httputil.WriteError(w, http.StatusBadRequest, "token is required")
			return
		}

		dt, err := svc.RegisterToken(r.Context(), req.AppID, claims.Subject, req.Provider, req.Platform, req.Token, req.DeviceName)
		if err != nil {
			mapPushError(w, err)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, dt)
	}
}

// handleUserPushListDevices lists all push notification device tokens registered to the authenticated user for a specified app.
func handleUserPushListDevices(svc pushAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			httputil.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		appID := r.URL.Query().Get("app_id")
		if appID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "app_id query parameter is required")
			return
		}

		tokens, err := svc.ListUserTokens(r.Context(), appID, claims.Subject)
		if err != nil {
			mapPushError(w, err)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"items": tokens,
		})
	}
}

// handleUserPushRevokeDevice revokes a push notification device token owned by the authenticated user, preventing further notifications to that device.
func handleUserPushRevokeDevice(svc pushAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			httputil.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		id := chi.URLParam(r, "id")

		// Ownership validation: verify token belongs to the authenticated user.
		token, err := svc.GetToken(r.Context(), id)
		if err != nil {
			if errors.Is(err, push.ErrNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "device token not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if token.UserID != claims.Subject {
			httputil.WriteError(w, http.StatusNotFound, "device token not found")
			return
		}

		if err := svc.RevokeToken(r.Context(), id); err != nil {
			mapPushError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Admin endpoints ---

type adminPushRegisterRequest struct {
	AppID      string `json:"app_id"`
	UserID     string `json:"user_id"`
	Provider   string `json:"provider"`
	Platform   string `json:"platform"`
	Token      string `json:"token"`
	DeviceName string `json:"device_name"`
}

// handleAdminPushListDevices lists push notification device tokens, optionally filtering by app and user, and optionally including inactive tokens.
func handleAdminPushListDevices(svc pushAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		appID := r.URL.Query().Get("app_id")
		userID := r.URL.Query().Get("user_id")
		includeInactive := r.URL.Query().Get("include_inactive") == "true"

		tokens, err := svc.ListTokens(r.Context(), appID, userID, includeInactive)
		if err != nil {
			mapPushError(w, err)
			return
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"items": tokens,
		})
	}
}

// handleAdminPushRegisterDevice registers a push notification device token for a specified user.
func handleAdminPushRegisterDevice(svc pushAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req adminPushRegisterRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.AppID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "app_id is required")
			return
		}
		if req.UserID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "user_id is required")
			return
		}
		if req.Token == "" {
			httputil.WriteError(w, http.StatusBadRequest, "token is required")
			return
		}

		dt, err := svc.RegisterToken(r.Context(), req.AppID, req.UserID, req.Provider, req.Platform, req.Token, req.DeviceName)
		if err != nil {
			mapPushError(w, err)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, dt)
	}
}

func handleAdminPushRevokeDevice(svc pushAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.RevokeToken(r.Context(), id); err != nil {
			mapPushError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type adminPushSendRequest struct {
	AppID  string            `json:"app_id"`
	UserID string            `json:"user_id"`
	Title  string            `json:"title"`
	Body   string            `json:"body"`
	Data   map[string]string `json:"data"`
}

// handleAdminPushSend sends a push notification to all registered devices for a user in a specified app.
func handleAdminPushSend(svc pushAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req adminPushSendRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.AppID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "app_id is required")
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
		if req.Body == "" {
			httputil.WriteError(w, http.StatusBadRequest, "body is required")
			return
		}

		deliveries, err := svc.SendToUser(r.Context(), req.AppID, req.UserID, req.Title, req.Body, req.Data)
		if err != nil {
			mapPushError(w, err)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"deliveries": deliveries,
		})
	}
}

type adminPushSendToTokenRequest struct {
	TokenID string            `json:"token_id"`
	Title   string            `json:"title"`
	Body    string            `json:"body"`
	Data    map[string]string `json:"data"`
}

// handleAdminPushSendToToken sends a push notification to a specific device token.
func handleAdminPushSendToToken(svc pushAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req adminPushSendToTokenRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.TokenID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "token_id is required")
			return
		}
		if req.Title == "" {
			httputil.WriteError(w, http.StatusBadRequest, "title is required")
			return
		}
		if req.Body == "" {
			httputil.WriteError(w, http.StatusBadRequest, "body is required")
			return
		}

		delivery, err := svc.SendToToken(r.Context(), req.TokenID, req.Title, req.Body, req.Data)
		if err != nil {
			mapPushError(w, err)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, delivery)
	}
}

// handleAdminPushListDeliveries lists push notification delivery records, optionally filtered by app, user, and delivery status, with pagination support.
func handleAdminPushListDeliveries(svc pushAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		appID := r.URL.Query().Get("app_id")
		userID := r.URL.Query().Get("user_id")
		status, err := normalizeDeliveryStatusFilter(r.URL.Query().Get("status"))
		if err != nil {
			mapPushError(w, err)
			return
		}
		limit := 50
		offset := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}

		deliveries, err := svc.ListDeliveries(r.Context(), appID, userID, status, limit, offset)
		if err != nil {
			mapPushError(w, err)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"items": deliveries,
		})
	}
}

func normalizeDeliveryStatusFilter(raw string) (string, error) {
	status := strings.ToLower(strings.TrimSpace(raw))
	switch status {
	case "":
		return "", nil
	case push.DeliveryStatusPending, push.DeliveryStatusSent, push.DeliveryStatusFailed, push.DeliveryStatusInvalidToken:
		return status, nil
	default:
		return "", fmt.Errorf("%w: status must be one of: pending, sent, failed, invalid_token", push.ErrInvalidPayload)
	}
}

func handleAdminPushGetDelivery(svc pushAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		delivery, err := svc.GetDelivery(r.Context(), id)
		if err != nil {
			mapPushError(w, err)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, delivery)
	}
}

// --- Server delegation methods (nil-check + dispatch) ---

func (s *Server) handlePushUserRegister(w http.ResponseWriter, r *http.Request) {
	if s.pushSvc == nil {
		serviceUnavailable(w, serviceUnavailablePush)
		return
	}
	handleUserPushRegister(s.pushSvc).ServeHTTP(w, r)
}

func (s *Server) handlePushUserListDevices(w http.ResponseWriter, r *http.Request) {
	if s.pushSvc == nil {
		serviceUnavailable(w, serviceUnavailablePush)
		return
	}
	handleUserPushListDevices(s.pushSvc).ServeHTTP(w, r)
}

func (s *Server) handlePushUserRevokeDevice(w http.ResponseWriter, r *http.Request) {
	if s.pushSvc == nil {
		serviceUnavailable(w, serviceUnavailablePush)
		return
	}
	handleUserPushRevokeDevice(s.pushSvc).ServeHTTP(w, r)
}

func (s *Server) handlePushAdminListDevices(w http.ResponseWriter, r *http.Request) {
	if s.pushSvc == nil {
		serviceUnavailable(w, serviceUnavailablePush)
		return
	}
	handleAdminPushListDevices(s.pushSvc).ServeHTTP(w, r)
}

func (s *Server) handlePushAdminRegisterDevice(w http.ResponseWriter, r *http.Request) {
	if s.pushSvc == nil {
		serviceUnavailable(w, serviceUnavailablePush)
		return
	}
	handleAdminPushRegisterDevice(s.pushSvc).ServeHTTP(w, r)
}

func (s *Server) handlePushAdminRevokeDevice(w http.ResponseWriter, r *http.Request) {
	if s.pushSvc == nil {
		serviceUnavailable(w, serviceUnavailablePush)
		return
	}
	handleAdminPushRevokeDevice(s.pushSvc).ServeHTTP(w, r)
}

func (s *Server) handlePushAdminSend(w http.ResponseWriter, r *http.Request) {
	if s.pushSvc == nil {
		serviceUnavailable(w, serviceUnavailablePush)
		return
	}
	handleAdminPushSend(s.pushSvc).ServeHTTP(w, r)
}

func (s *Server) handlePushAdminSendToToken(w http.ResponseWriter, r *http.Request) {
	if s.pushSvc == nil {
		serviceUnavailable(w, serviceUnavailablePush)
		return
	}
	handleAdminPushSendToToken(s.pushSvc).ServeHTTP(w, r)
}

func (s *Server) handlePushAdminListDeliveries(w http.ResponseWriter, r *http.Request) {
	if s.pushSvc == nil {
		serviceUnavailable(w, serviceUnavailablePush)
		return
	}
	handleAdminPushListDeliveries(s.pushSvc).ServeHTTP(w, r)
}

func (s *Server) handlePushAdminGetDelivery(w http.ResponseWriter, r *http.Request) {
	if s.pushSvc == nil {
		serviceUnavailable(w, serviceUnavailablePush)
		return
	}
	handleAdminPushGetDelivery(s.pushSvc).ServeHTTP(w, r)
}
