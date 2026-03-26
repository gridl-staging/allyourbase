// Package push coordinates device-token lifecycle and push notification delivery.
package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/allyourbase/ayb/internal/jobs"
)

// Service coordinates device-token lifecycle and asynchronous push delivery.
type Service struct {
	store     PushStore
	providers map[string]Provider
	jobs      jobEnqueuer
	logger    *slog.Logger
}

// NewService creates a push service.
func NewService(store PushStore, providers map[string]Provider, jobsSvc jobEnqueuer) *Service {
	providerMap := map[string]Provider{}
	for k, v := range providers {
		providerMap[strings.ToLower(strings.TrimSpace(k))] = v
	}
	return &Service{
		store:     store,
		providers: providerMap,
		jobs:      jobsSvc,
		logger:    slog.Default(),
	}
}

// SetLogger overrides the service logger.
func (s *Service) SetLogger(logger *slog.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

// RegisterToken creates or refreshes a device token registration.
func (s *Service) RegisterToken(ctx context.Context, appID, userID, provider, platform, token, deviceName string) (*DeviceToken, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	platform = strings.ToLower(strings.TrimSpace(platform))
	if !isValidProvider(provider) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidProvider, provider)
	}
	if !isValidPlatform(platform) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPlatform, platform)
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("%w: token is required", ErrInvalidToken)
	}
	return s.store.RegisterToken(ctx, appID, userID, provider, platform, token, deviceName)
}

// RevokeToken deactivates one token.
func (s *Service) RevokeToken(ctx context.Context, tokenID string) error {
	return s.store.RevokeToken(ctx, tokenID)
}

// RevokeAllUserTokens deactivates all active tokens for a user/app.
func (s *Service) RevokeAllUserTokens(ctx context.Context, appID, userID string) error {
	_, err := s.store.RevokeAllUserTokens(ctx, appID, userID)
	return err
}

// ListUserTokens returns active tokens for a user/app.
func (s *Service) ListUserTokens(ctx context.Context, appID, userID string) ([]*DeviceToken, error) {
	return s.store.ListUserTokens(ctx, appID, userID)
}

// SendToUser fans out to all active tokens and enqueues async delivery jobs.
func (s *Service) SendToUser(ctx context.Context, appID, userID, title, body string, data map[string]string) ([]*PushDelivery, error) {
	if err := validatePayloadSize(title, body, data); err != nil {
		return nil, err
	}

	tokens, err := s.store.ListUserTokens(ctx, appID, userID)
	if err != nil {
		return nil, fmt.Errorf("list user tokens: %w", err)
	}
	if len(tokens) == 0 {
		return []*PushDelivery{}, nil
	}
	if s.jobs == nil {
		return nil, fmt.Errorf("job queue is not configured")
	}

	deliveries := make([]*PushDelivery, 0, len(tokens))
	for _, token := range tokens {
		delivery, err := s.store.RecordDelivery(ctx, &PushDelivery{
			DeviceTokenID: token.ID,
			AppID:         token.AppID,
			UserID:        token.UserID,
			Provider:      token.Provider,
			Title:         title,
			Body:          body,
			DataPayload:   cloneStringMap(data),
			Status:        DeliveryStatusPending,
		})
		if err != nil {
			return nil, fmt.Errorf("record delivery: %w", err)
		}

		payload, err := json.Marshal(map[string]string{"delivery_id": delivery.ID})
		if err != nil {
			return nil, fmt.Errorf("marshal job payload: %w", err)
		}

		job, err := s.jobs.Enqueue(ctx, JobTypePushDelivery, payload, jobs.EnqueueOpts{MaxAttempts: 3})
		if err != nil {
			_ = s.store.UpdateDeliveryStatus(ctx, delivery.ID, DeliveryStatusFailed, "enqueue_error", err.Error(), "")
			return nil, fmt.Errorf("enqueue delivery job: %w", err)
		}
		if job != nil && job.ID != "" {
			if err := s.store.SetDeliveryJobID(ctx, delivery.ID, job.ID); err != nil {
				return nil, fmt.Errorf("set delivery job id: %w", err)
			}
			delivery.JobID = &job.ID
		}

		deliveries = append(deliveries, delivery)
	}

	return deliveries, nil
}

// SendToToken queues a notification for one device token.
func (s *Service) SendToToken(ctx context.Context, tokenID, title, body string, data map[string]string) (*PushDelivery, error) {
	if err := validatePayloadSize(title, body, data); err != nil {
		return nil, err
	}
	if s.jobs == nil {
		return nil, fmt.Errorf("job queue is not configured")
	}

	token, err := s.store.GetToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	if !token.IsActive {
		return nil, fmt.Errorf("%w: device token is inactive", ErrInvalidToken)
	}

	delivery, err := s.store.RecordDelivery(ctx, &PushDelivery{
		DeviceTokenID: token.ID,
		AppID:         token.AppID,
		UserID:        token.UserID,
		Provider:      token.Provider,
		Title:         title,
		Body:          body,
		DataPayload:   cloneStringMap(data),
		Status:        DeliveryStatusPending,
	})
	if err != nil {
		return nil, fmt.Errorf("record delivery: %w", err)
	}

	payload, err := json.Marshal(map[string]string{"delivery_id": delivery.ID})
	if err != nil {
		return nil, fmt.Errorf("marshal job payload: %w", err)
	}

	job, err := s.jobs.Enqueue(ctx, JobTypePushDelivery, payload, jobs.EnqueueOpts{MaxAttempts: 3})
	if err != nil {
		_ = s.store.UpdateDeliveryStatus(ctx, delivery.ID, DeliveryStatusFailed, "enqueue_error", err.Error(), "")
		return nil, fmt.Errorf("enqueue delivery job: %w", err)
	}
	if job != nil && job.ID != "" {
		if err := s.store.SetDeliveryJobID(ctx, delivery.ID, job.ID); err != nil {
			return nil, fmt.Errorf("set delivery job id: %w", err)
		}
		delivery.JobID = &job.ID
	}

	return delivery, nil
}

// ProcessDelivery performs a single provider send attempt for a delivery row.
func (s *Service) ProcessDelivery(ctx context.Context, deliveryID string) error {
	delivery, err := s.store.GetDelivery(ctx, deliveryID)
	if err != nil {
		return fmt.Errorf("get delivery: %w", err)
	}

	// Idempotency guard: skip if already resolved (sent or invalid_token).
	// This prevents duplicate push notifications when a job retries after
	// a partial success (e.g., provider send succeeded but UpdateLastUsed failed).
	if delivery.Status == DeliveryStatusSent || delivery.Status == DeliveryStatusInvalidToken {
		return nil
	}

	token, err := s.store.GetToken(ctx, delivery.DeviceTokenID)
	if err != nil {
		return fmt.Errorf("get device token: %w", err)
	}
	if !token.IsActive {
		inactiveErr := fmt.Errorf("%w: device token is inactive", ErrInvalidToken)
		if err := s.store.UpdateDeliveryStatus(ctx, delivery.ID, DeliveryStatusInvalidToken, classifyProviderError(inactiveErr), inactiveErr.Error(), ""); err != nil {
			return fmt.Errorf("update invalid_token status: %w", err)
		}
		return nil
	}

	providerName := strings.ToLower(strings.TrimSpace(token.Provider))
	provider, ok := s.providers[providerName]
	if !ok {
		err := fmt.Errorf("provider %q not configured", providerName)
		_ = s.store.UpdateDeliveryStatus(ctx, delivery.ID, DeliveryStatusFailed, "provider_not_configured", err.Error(), "")
		return err
	}

	result, sendErr := provider.Send(ctx, token.Token, &Message{
		Title: delivery.Title,
		Body:  delivery.Body,
		Data:  cloneStringMap(delivery.DataPayload),
	})
	if sendErr == nil {
		messageID := ""
		if result != nil {
			messageID = result.MessageID
		}
		if err := s.store.UpdateDeliveryStatus(ctx, delivery.ID, DeliveryStatusSent, "", "", messageID); err != nil {
			return fmt.Errorf("update sent status: %w", err)
		}
		if err := s.store.UpdateLastUsed(ctx, token.ID); err != nil {
			return fmt.Errorf("update token last_used: %w", err)
		}
		return nil
	}

	code := classifyProviderError(sendErr)
	if errors.Is(sendErr, ErrInvalidToken) || errors.Is(sendErr, ErrUnregistered) {
		if err := s.store.UpdateDeliveryStatus(ctx, delivery.ID, DeliveryStatusInvalidToken, code, sendErr.Error(), ""); err != nil {
			return fmt.Errorf("update invalid_token status: %w", err)
		}
		if err := s.store.MarkInactive(ctx, token.ID); err != nil {
			return fmt.Errorf("mark token inactive: %w", err)
		}
		return nil
	}

	if err := s.store.UpdateDeliveryStatus(ctx, delivery.ID, DeliveryStatusFailed, code, sendErr.Error(), ""); err != nil {
		return fmt.Errorf("update failed status: %w", err)
	}
	if errors.Is(sendErr, ErrProviderAuth) {
		s.logger.Warn("push provider auth failure; retry will refresh provider auth token", "delivery_id", delivery.ID, "token_id", token.ID)
	}
	return sendErr
}

// ListTokens returns device tokens (admin listing).
func (s *Service) ListTokens(ctx context.Context, appID, userID string, includeInactive bool) ([]*DeviceToken, error) {
	return s.store.ListTokens(ctx, appID, userID, includeInactive)
}

// GetToken returns a single device token by ID.
func (s *Service) GetToken(ctx context.Context, id string) (*DeviceToken, error) {
	return s.store.GetToken(ctx, id)
}

// ListDeliveries lists push delivery audit records.
func (s *Service) ListDeliveries(ctx context.Context, appID, userID, status string, limit, offset int) ([]*PushDelivery, error) {
	return s.store.ListDeliveries(ctx, appID, userID, status, limit, offset)
}

// GetDelivery returns a single delivery audit record by ID.
func (s *Service) GetDelivery(ctx context.Context, id string) (*PushDelivery, error) {
	return s.store.GetDelivery(ctx, id)
}

// RunTokenCleanup marks stale tokens inactive.
func (s *Service) RunTokenCleanup(ctx context.Context, staleDays int) (int64, error) {
	return s.store.CleanupStaleTokens(ctx, staleDays)
}

func isValidProvider(provider string) bool {
	switch provider {
	case ProviderFCM, ProviderAPNS:
		return true
	default:
		return false
	}
}

func isValidPlatform(platform string) bool {
	switch platform {
	case PlatformAndroid, PlatformIOS:
		return true
	default:
		return false
	}
}

// validatePayloadSize verifies that title and body are non-empty and the JSON-marshaled notification payload does not exceed 4096 bytes.
func validatePayloadSize(title, body string, data map[string]string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("%w: title is required", ErrInvalidPayload)
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("%w: body is required", ErrInvalidPayload)
	}

	payload := map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{
				"title": title,
				"body":  body,
			},
		},
	}
	for k, v := range data {
		payload[k] = v
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: marshal payload: %v", ErrProviderError, err)
	}
	if len(raw) > 4096 {
		return fmt.Errorf("%w: payload is %d bytes (max 4096)", ErrPayloadTooLarge, len(raw))
	}
	return nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func classifyProviderError(err error) string {
	switch {
	case errors.Is(err, ErrUnregistered):
		return "unregistered"
	case errors.Is(err, ErrInvalidToken):
		return "invalid_token"
	case errors.Is(err, ErrProviderAuth):
		return "provider_auth"
	case errors.Is(err, ErrPayloadTooLarge):
		return "payload_too_large"
	default:
		return "provider_error"
	}
}
