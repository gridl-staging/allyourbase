package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/testutil"
)

type stubPushStore struct {
	registerTokenFn       func(ctx context.Context, appID, userID, provider, platform, token, deviceName string) (*DeviceToken, error)
	getTokenFn            func(ctx context.Context, id string) (*DeviceToken, error)
	listUserTokensFn      func(ctx context.Context, appID, userID string) ([]*DeviceToken, error)
	listTokensFn          func(ctx context.Context, appID, userID string, includeInactive bool) ([]*DeviceToken, error)
	revokeTokenFn         func(ctx context.Context, id string) error
	revokeAllUserTokensFn func(ctx context.Context, appID, userID string) (int64, error)
	deleteTokenFn         func(ctx context.Context, id string) error
	recordDeliveryFn      func(ctx context.Context, delivery *PushDelivery) (*PushDelivery, error)
	setDeliveryJobIDFn    func(ctx context.Context, deliveryID, jobID string) error
	getDeliveryFn         func(ctx context.Context, id string) (*PushDelivery, error)
	updateDeliveryStatus  []updateDeliveryCall
	updateLastUsedCalls   []string
	markInactiveCalls     []string
	cleanupStaleTokensFn  func(ctx context.Context, staleDays int) (int64, error)
	listDeliveriesFn      func(ctx context.Context, appID, userID, status string, limit, offset int) ([]*PushDelivery, error)
}

type updateDeliveryCall struct {
	id        string
	status    string
	errorCode string
	errorMsg  string
	messageID string
}

func (s *stubPushStore) RegisterToken(ctx context.Context, appID, userID, provider, platform, token, deviceName string) (*DeviceToken, error) {
	if s.registerTokenFn != nil {
		return s.registerTokenFn(ctx, appID, userID, provider, platform, token, deviceName)
	}
	return nil, fmt.Errorf("unexpected RegisterToken call")
}

func (s *stubPushStore) GetToken(ctx context.Context, id string) (*DeviceToken, error) {
	if s.getTokenFn != nil {
		return s.getTokenFn(ctx, id)
	}
	return nil, fmt.Errorf("unexpected GetToken call")
}

func (s *stubPushStore) ListUserTokens(ctx context.Context, appID, userID string) ([]*DeviceToken, error) {
	if s.listUserTokensFn != nil {
		return s.listUserTokensFn(ctx, appID, userID)
	}
	return nil, fmt.Errorf("unexpected ListUserTokens call")
}

func (s *stubPushStore) ListTokens(ctx context.Context, appID, userID string, includeInactive bool) ([]*DeviceToken, error) {
	if s.listTokensFn != nil {
		return s.listTokensFn(ctx, appID, userID, includeInactive)
	}
	return nil, fmt.Errorf("unexpected ListTokens call")
}

func (s *stubPushStore) RevokeToken(ctx context.Context, id string) error {
	if s.revokeTokenFn != nil {
		return s.revokeTokenFn(ctx, id)
	}
	return fmt.Errorf("unexpected RevokeToken call")
}

func (s *stubPushStore) RevokeAllUserTokens(ctx context.Context, appID, userID string) (int64, error) {
	if s.revokeAllUserTokensFn != nil {
		return s.revokeAllUserTokensFn(ctx, appID, userID)
	}
	return 0, fmt.Errorf("unexpected RevokeAllUserTokens call")
}

func (s *stubPushStore) DeleteToken(ctx context.Context, id string) error {
	if s.deleteTokenFn != nil {
		return s.deleteTokenFn(ctx, id)
	}
	return fmt.Errorf("unexpected DeleteToken call")
}

func (s *stubPushStore) RecordDelivery(ctx context.Context, delivery *PushDelivery) (*PushDelivery, error) {
	if s.recordDeliveryFn != nil {
		return s.recordDeliveryFn(ctx, delivery)
	}
	return nil, fmt.Errorf("unexpected RecordDelivery call")
}

func (s *stubPushStore) SetDeliveryJobID(ctx context.Context, deliveryID, jobID string) error {
	if s.setDeliveryJobIDFn != nil {
		return s.setDeliveryJobIDFn(ctx, deliveryID, jobID)
	}
	return nil
}

func (s *stubPushStore) GetDelivery(ctx context.Context, id string) (*PushDelivery, error) {
	if s.getDeliveryFn != nil {
		return s.getDeliveryFn(ctx, id)
	}
	return nil, fmt.Errorf("unexpected GetDelivery call")
}

func (s *stubPushStore) UpdateDeliveryStatus(ctx context.Context, id, status, errorCode, errorMsg, messageID string) error {
	s.updateDeliveryStatus = append(s.updateDeliveryStatus, updateDeliveryCall{
		id: id, status: status, errorCode: errorCode, errorMsg: errorMsg, messageID: messageID,
	})
	return nil
}

func (s *stubPushStore) UpdateLastUsed(ctx context.Context, id string) error {
	s.updateLastUsedCalls = append(s.updateLastUsedCalls, id)
	return nil
}

func (s *stubPushStore) MarkInactive(ctx context.Context, id string) error {
	s.markInactiveCalls = append(s.markInactiveCalls, id)
	return nil
}

func (s *stubPushStore) CleanupStaleTokens(ctx context.Context, staleDays int) (int64, error) {
	if s.cleanupStaleTokensFn != nil {
		return s.cleanupStaleTokensFn(ctx, staleDays)
	}
	return 0, nil
}

func (s *stubPushStore) ListDeliveries(ctx context.Context, appID, userID, status string, limit, offset int) ([]*PushDelivery, error) {
	if s.listDeliveriesFn != nil {
		return s.listDeliveriesFn(ctx, appID, userID, status, limit, offset)
	}
	return nil, fmt.Errorf("unexpected ListDeliveries call")
}

type stubEnqueuer struct {
	enqueueCalls []enqueueCall
	enqueueFn    func(ctx context.Context, jobType string, payload json.RawMessage, opts jobs.EnqueueOpts) (*jobs.Job, error)
}

type enqueueCall struct {
	jobType string
	payload json.RawMessage
	opts    jobs.EnqueueOpts
}

type stubProvider struct {
	sendFn func(ctx context.Context, token string, msg *Message) (*Result, error)
}

func (s *stubProvider) Send(ctx context.Context, token string, msg *Message) (*Result, error) {
	if s.sendFn != nil {
		return s.sendFn(ctx, token, msg)
	}
	return &Result{MessageID: "stub-msg"}, nil
}

func (s *stubEnqueuer) Enqueue(ctx context.Context, jobType string, payload json.RawMessage, opts jobs.EnqueueOpts) (*jobs.Job, error) {
	s.enqueueCalls = append(s.enqueueCalls, enqueueCall{jobType: jobType, payload: payload, opts: opts})
	if s.enqueueFn != nil {
		return s.enqueueFn(ctx, jobType, payload, opts)
	}
	return &jobs.Job{ID: "job-default"}, nil
}

func TestServiceRegisterTokenValidation(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	svc := NewService(store, nil, nil)

	_, err := svc.RegisterToken(context.Background(), "app-1", "user-1", "invalid", PlatformIOS, "tok", "iPhone")
	testutil.True(t, errors.Is(err, ErrInvalidProvider), "expected ErrInvalidProvider, got %v", err)

	_, err = svc.RegisterToken(context.Background(), "app-1", "user-1", ProviderFCM, "invalid", "tok", "iPhone")
	testutil.True(t, errors.Is(err, ErrInvalidPlatform), "expected ErrInvalidPlatform, got %v", err)
}

func TestServiceSendToUserFanoutEnqueuesJobs(t *testing.T) {
	t.Parallel()

	tokens := []*DeviceToken{
		{ID: "tok-1", AppID: "app-1", UserID: "user-1", Provider: ProviderFCM, Platform: PlatformAndroid, Token: "fcm-token"},
		{ID: "tok-2", AppID: "app-1", UserID: "user-1", Provider: ProviderAPNS, Platform: PlatformIOS, Token: "apns-token"},
	}

	store := &stubPushStore{}
	store.listUserTokensFn = func(ctx context.Context, appID, userID string) ([]*DeviceToken, error) {
		return tokens, nil
	}
	seq := 0
	recorded := make([]*PushDelivery, 0, 2)
	store.recordDeliveryFn = func(ctx context.Context, d *PushDelivery) (*PushDelivery, error) {
		seq++
		copy := *d
		copy.ID = fmt.Sprintf("deliv-%d", seq)
		recorded = append(recorded, &copy)
		return &copy, nil
	}
	jobLinks := make(map[string]string)
	store.setDeliveryJobIDFn = func(ctx context.Context, deliveryID, jobID string) error {
		jobLinks[deliveryID] = jobID
		return nil
	}

	enqueuer := &stubEnqueuer{}
	jobSeq := 0
	enqueuer.enqueueFn = func(ctx context.Context, jobType string, payload json.RawMessage, opts jobs.EnqueueOpts) (*jobs.Job, error) {
		jobSeq++
		return &jobs.Job{ID: fmt.Sprintf("job-%d", jobSeq)}, nil
	}

	svc := NewService(store, map[string]Provider{}, enqueuer)
	deliveries, err := svc.SendToUser(context.Background(), "app-1", "user-1", "hello", "world", map[string]string{"k": "v"})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(deliveries))
	testutil.Equal(t, 2, len(enqueuer.enqueueCalls))
	testutil.Equal(t, "push_delivery", enqueuer.enqueueCalls[0].jobType)
	testutil.Equal(t, "pending", recorded[0].Status)
	testutil.Equal(t, "pending", recorded[1].Status)
	testutil.Equal(t, "job-1", jobLinks["deliv-1"])
	testutil.Equal(t, "job-2", jobLinks["deliv-2"])

	for i := range enqueuer.enqueueCalls {
		var payload map[string]string
		testutil.NoError(t, json.Unmarshal(enqueuer.enqueueCalls[i].payload, &payload))
		testutil.True(t, strings.HasPrefix(payload["delivery_id"], "deliv-"), "missing delivery_id in payload")
	}
}

func TestServiceSendToUserRejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.listUserTokensFn = func(ctx context.Context, appID, userID string) ([]*DeviceToken, error) {
		return []*DeviceToken{{ID: "tok-1", AppID: appID, UserID: userID, Provider: ProviderFCM, Platform: PlatformAndroid, Token: "fcm-token"}}, nil
	}
	store.recordDeliveryFn = func(ctx context.Context, d *PushDelivery) (*PushDelivery, error) {
		return nil, fmt.Errorf("should not record delivery for oversized payload")
	}
	enqueuer := &stubEnqueuer{}
	enqueuer.enqueueFn = func(ctx context.Context, jobType string, payload json.RawMessage, opts jobs.EnqueueOpts) (*jobs.Job, error) {
		return nil, fmt.Errorf("should not enqueue for oversized payload")
	}

	svc := NewService(store, nil, enqueuer)
	_, err := svc.SendToUser(context.Background(), "app-1", "user-1", "title", "body", map[string]string{"blob": strings.Repeat("x", 5000)})
	testutil.True(t, errors.Is(err, ErrPayloadTooLarge), "expected ErrPayloadTooLarge, got %v", err)
}

func TestServiceProcessDeliverySuccess(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		return &PushDelivery{
			ID:            id,
			DeviceTokenID: "tok-1",
			AppID:         "app-1",
			UserID:        "user-1",
			Provider:      ProviderFCM,
			Title:         "hello",
			Body:          "world",
			DataPayload:   map[string]string{"k": "v"},
			Status:        "pending",
		}, nil
	}
	store.getTokenFn = func(ctx context.Context, id string) (*DeviceToken, error) {
		return &DeviceToken{ID: id, Provider: ProviderFCM, Token: "device-token", IsActive: true}, nil
	}

	provider := &stubProvider{
		sendFn: func(ctx context.Context, token string, msg *Message) (*Result, error) {
			return &Result{MessageID: "msg-123"}, nil
		},
	}
	svc := NewService(store, map[string]Provider{ProviderFCM: provider}, nil)

	err := svc.ProcessDelivery(context.Background(), "deliv-1")
	testutil.NoError(t, err)
	testutil.SliceLen(t, store.updateDeliveryStatus, 1)
	testutil.Equal(t, "sent", store.updateDeliveryStatus[0].status)
	testutil.Equal(t, "msg-123", store.updateDeliveryStatus[0].messageID)
	testutil.SliceLen(t, store.updateLastUsedCalls, 1)
	testutil.Equal(t, "tok-1", store.updateLastUsedCalls[0])
	testutil.SliceLen(t, store.markInactiveCalls, 0)
}

func TestServiceProcessDeliveryInvalidTokenMarksInactive(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		return &PushDelivery{ID: id, DeviceTokenID: "tok-1", Provider: ProviderFCM, Title: "hello", Body: "world"}, nil
	}
	store.getTokenFn = func(ctx context.Context, id string) (*DeviceToken, error) {
		return &DeviceToken{ID: id, Provider: ProviderFCM, Token: "device-token", IsActive: true}, nil
	}

	provider := &stubProvider{
		sendFn: func(ctx context.Context, token string, msg *Message) (*Result, error) {
			return nil, fmt.Errorf("wrapped: %w", ErrUnregistered)
		},
	}
	svc := NewService(store, map[string]Provider{ProviderFCM: provider}, nil)

	err := svc.ProcessDelivery(context.Background(), "deliv-1")
	testutil.NoError(t, err)
	testutil.SliceLen(t, store.updateDeliveryStatus, 1)
	testutil.Equal(t, "invalid_token", store.updateDeliveryStatus[0].status)
	testutil.Equal(t, "unregistered", store.updateDeliveryStatus[0].errorCode)
	testutil.SliceLen(t, store.markInactiveCalls, 1)
	testutil.Equal(t, "tok-1", store.markInactiveCalls[0])
}

func TestServiceProcessDeliveryTransientFailureReturnsError(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		return &PushDelivery{ID: id, DeviceTokenID: "tok-1", Provider: ProviderFCM, Title: "hello", Body: "world"}, nil
	}
	store.getTokenFn = func(ctx context.Context, id string) (*DeviceToken, error) {
		return &DeviceToken{ID: id, Provider: ProviderFCM, Token: "device-token", IsActive: true}, nil
	}

	provider := &stubProvider{
		sendFn: func(ctx context.Context, token string, msg *Message) (*Result, error) {
			return nil, fmt.Errorf("wrapped: %w", ErrProviderError)
		},
	}
	svc := NewService(store, map[string]Provider{ProviderFCM: provider}, nil)

	err := svc.ProcessDelivery(context.Background(), "deliv-1")
	testutil.NotNil(t, err)
	testutil.SliceLen(t, store.updateDeliveryStatus, 1)
	testutil.Equal(t, "failed", store.updateDeliveryStatus[0].status)
	testutil.Equal(t, "provider_error", store.updateDeliveryStatus[0].errorCode)
	testutil.SliceLen(t, store.markInactiveCalls, 0)
}

func TestServiceProcessDeliveryInactiveTokenSkipsProviderSend(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		return &PushDelivery{
			ID:            id,
			DeviceTokenID: "tok-1",
			Provider:      ProviderFCM,
			Title:         "hello",
			Body:          "world",
			Status:        DeliveryStatusPending,
		}, nil
	}
	store.getTokenFn = func(ctx context.Context, id string) (*DeviceToken, error) {
		return &DeviceToken{ID: id, Provider: ProviderFCM, Token: "device-token", IsActive: false}, nil
	}

	providerCalled := false
	provider := &stubProvider{
		sendFn: func(ctx context.Context, token string, msg *Message) (*Result, error) {
			providerCalled = true
			return &Result{MessageID: "unexpected"}, nil
		},
	}
	svc := NewService(store, map[string]Provider{ProviderFCM: provider}, nil)

	err := svc.ProcessDelivery(context.Background(), "deliv-1")
	testutil.NoError(t, err)
	testutil.True(t, !providerCalled, "provider send should not be called for inactive tokens")
	testutil.SliceLen(t, store.updateDeliveryStatus, 1)
	testutil.Equal(t, DeliveryStatusInvalidToken, store.updateDeliveryStatus[0].status)
	testutil.Equal(t, "invalid_token", store.updateDeliveryStatus[0].errorCode)
	testutil.Contains(t, store.updateDeliveryStatus[0].errorMsg, "inactive")
	testutil.SliceLen(t, store.updateLastUsedCalls, 0)
	testutil.SliceLen(t, store.markInactiveCalls, 0)
}

func TestPushDeliveryJobHandler(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	called := 0
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		called++
		return nil, fmt.Errorf("stop")
	}
	svc := NewService(store, nil, nil)

	h := PushDeliveryJobHandler(svc)

	err := h(context.Background(), json.RawMessage(`{"delivery_id":"deliv-1"}`))
	testutil.NotNil(t, err)
	testutil.Equal(t, 1, called)

	err = h(context.Background(), json.RawMessage(`{"delivery_id":""}`))
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "delivery_id is required")
}

func TestPushTokenCleanupJobHandler(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	called := 0
	store.cleanupStaleTokensFn = func(ctx context.Context, staleDays int) (int64, error) {
		called++
		testutil.Equal(t, 270, staleDays)
		return 12, nil
	}

	h := PushTokenCleanupJobHandler(store, 270)
	err := h(context.Background(), nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, called)
}

func TestServiceRegisterTokenEmptyToken(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	svc := NewService(store, nil, nil)

	_, err := svc.RegisterToken(context.Background(), "app-1", "user-1", ProviderFCM, PlatformAndroid, "", "device")
	testutil.True(t, errors.Is(err, ErrInvalidToken), "expected ErrInvalidToken, got %v", err)

	_, err = svc.RegisterToken(context.Background(), "app-1", "user-1", ProviderFCM, PlatformAndroid, "   ", "device")
	testutil.True(t, errors.Is(err, ErrInvalidToken), "expected ErrInvalidToken for whitespace-only token, got %v", err)
}

func TestServiceSendToUserEmptyTitleBody(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	svc := NewService(store, nil, nil)

	_, err := svc.SendToUser(context.Background(), "app-1", "user-1", "", "body", nil)
	testutil.True(t, errors.Is(err, ErrInvalidPayload), "expected ErrInvalidPayload for empty title, got %v", err)

	_, err = svc.SendToUser(context.Background(), "app-1", "user-1", "title", "", nil)
	testutil.True(t, errors.Is(err, ErrInvalidPayload), "expected ErrInvalidPayload for empty body, got %v", err)
}

func TestServiceSendToUserZeroTokens(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.listUserTokensFn = func(ctx context.Context, appID, userID string) ([]*DeviceToken, error) {
		return []*DeviceToken{}, nil
	}
	enqueuer := &stubEnqueuer{}
	svc := NewService(store, nil, enqueuer)

	deliveries, err := svc.SendToUser(context.Background(), "app-1", "user-1", "hello", "world", nil)
	testutil.NoError(t, err)
	testutil.SliceLen(t, deliveries, 0)
	testutil.SliceLen(t, enqueuer.enqueueCalls, 0)
}

func TestServiceSendToTokenSuccess(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getTokenFn = func(ctx context.Context, id string) (*DeviceToken, error) {
		return &DeviceToken{ID: id, AppID: "app-1", UserID: "user-1", Provider: ProviderFCM, Token: "fcm-tok", IsActive: true}, nil
	}
	store.recordDeliveryFn = func(ctx context.Context, d *PushDelivery) (*PushDelivery, error) {
		copy := *d
		copy.ID = "deliv-1"
		return &copy, nil
	}
	store.setDeliveryJobIDFn = func(ctx context.Context, deliveryID, jobID string) error {
		return nil
	}

	enqueuer := &stubEnqueuer{}
	enqueuer.enqueueFn = func(ctx context.Context, jobType string, payload json.RawMessage, opts jobs.EnqueueOpts) (*jobs.Job, error) {
		return &jobs.Job{ID: "job-1"}, nil
	}

	svc := NewService(store, map[string]Provider{}, enqueuer)
	delivery, err := svc.SendToToken(context.Background(), "tok-1", "hello", "world", map[string]string{"k": "v"})
	testutil.NoError(t, err)
	testutil.NotNil(t, delivery)
	testutil.Equal(t, "deliv-1", delivery.ID)
	testutil.Equal(t, 1, len(enqueuer.enqueueCalls))
	testutil.Equal(t, "push_delivery", enqueuer.enqueueCalls[0].jobType)
}

func TestServiceSendToTokenInactiveRejected(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getTokenFn = func(ctx context.Context, id string) (*DeviceToken, error) {
		return &DeviceToken{ID: id, AppID: "app-1", UserID: "user-1", Provider: ProviderFCM, Token: "fcm-tok", IsActive: false}, nil
	}

	svc := NewService(store, nil, &stubEnqueuer{})
	_, err := svc.SendToToken(context.Background(), "tok-1", "hello", "world", nil)
	testutil.True(t, errors.Is(err, ErrInvalidToken), "expected ErrInvalidToken for inactive token, got %v", err)
}

func TestServiceProcessDeliveryInvalidTokenSentinel(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		return &PushDelivery{ID: id, DeviceTokenID: "tok-1", Provider: ProviderAPNS, Title: "hello", Body: "world"}, nil
	}
	store.getTokenFn = func(ctx context.Context, id string) (*DeviceToken, error) {
		return &DeviceToken{ID: id, Provider: ProviderAPNS, Token: "apns-token", IsActive: true}, nil
	}

	provider := &stubProvider{
		sendFn: func(ctx context.Context, token string, msg *Message) (*Result, error) {
			return nil, fmt.Errorf("bad token: %w", ErrInvalidToken)
		},
	}
	svc := NewService(store, map[string]Provider{ProviderAPNS: provider}, nil)

	err := svc.ProcessDelivery(context.Background(), "deliv-1")
	testutil.NoError(t, err)
	testutil.SliceLen(t, store.updateDeliveryStatus, 1)
	testutil.Equal(t, "invalid_token", store.updateDeliveryStatus[0].status)
	testutil.Equal(t, "invalid_token", store.updateDeliveryStatus[0].errorCode)
	testutil.SliceLen(t, store.markInactiveCalls, 1)
	testutil.Equal(t, "tok-1", store.markInactiveCalls[0])
}

func TestServiceProcessDeliveryProviderAuthReturnsError(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		return &PushDelivery{ID: id, DeviceTokenID: "tok-1", Provider: ProviderFCM, Title: "hello", Body: "world"}, nil
	}
	store.getTokenFn = func(ctx context.Context, id string) (*DeviceToken, error) {
		return &DeviceToken{ID: id, Provider: ProviderFCM, Token: "device-token", IsActive: true}, nil
	}

	provider := &stubProvider{
		sendFn: func(ctx context.Context, token string, msg *Message) (*Result, error) {
			return nil, fmt.Errorf("expired: %w", ErrProviderAuth)
		},
	}
	svc := NewService(store, map[string]Provider{ProviderFCM: provider}, nil)

	err := svc.ProcessDelivery(context.Background(), "deliv-1")
	testutil.NotNil(t, err)
	testutil.True(t, errors.Is(err, ErrProviderAuth), "expected ErrProviderAuth, got %v", err)
	testutil.SliceLen(t, store.updateDeliveryStatus, 1)
	testutil.Equal(t, "failed", store.updateDeliveryStatus[0].status)
	testutil.Equal(t, "provider_auth", store.updateDeliveryStatus[0].errorCode)
	testutil.SliceLen(t, store.markInactiveCalls, 0)
}

func TestServiceProcessDeliverySkipsAlreadySent(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		return &PushDelivery{
			ID:            id,
			DeviceTokenID: "tok-1",
			Provider:      ProviderFCM,
			Title:         "hello",
			Body:          "world",
			Status:        DeliveryStatusSent, // already sent
		}, nil
	}

	sendCalled := false
	provider := &stubProvider{
		sendFn: func(ctx context.Context, token string, msg *Message) (*Result, error) {
			sendCalled = true
			return &Result{MessageID: "dup"}, nil
		},
	}
	svc := NewService(store, map[string]Provider{ProviderFCM: provider}, nil)

	err := svc.ProcessDelivery(context.Background(), "deliv-1")
	testutil.NoError(t, err)
	testutil.True(t, !sendCalled, "provider.Send should not be called for already-sent delivery")
	testutil.SliceLen(t, store.updateDeliveryStatus, 0)
	testutil.SliceLen(t, store.updateLastUsedCalls, 0)
}

func TestServiceProcessDeliverySkipsInvalidToken(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		return &PushDelivery{
			ID:            id,
			DeviceTokenID: "tok-1",
			Provider:      ProviderFCM,
			Title:         "hello",
			Body:          "world",
			Status:        DeliveryStatusInvalidToken, // already resolved
		}, nil
	}

	sendCalled := false
	provider := &stubProvider{
		sendFn: func(ctx context.Context, token string, msg *Message) (*Result, error) {
			sendCalled = true
			return nil, fmt.Errorf("should not be called")
		},
	}
	svc := NewService(store, map[string]Provider{ProviderFCM: provider}, nil)

	err := svc.ProcessDelivery(context.Background(), "deliv-1")
	testutil.NoError(t, err)
	testutil.True(t, !sendCalled, "provider.Send should not be called for invalid_token delivery")
}

func TestServiceListDeliveries(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.listDeliveriesFn = func(ctx context.Context, appID, userID, status string, limit, offset int) ([]*PushDelivery, error) {
		testutil.Equal(t, "app-1", appID)
		testutil.Equal(t, "user-1", userID)
		testutil.Equal(t, "sent", status)
		testutil.Equal(t, 10, limit)
		testutil.Equal(t, 5, offset)
		return []*PushDelivery{{ID: "d-1", Status: DeliveryStatusSent}}, nil
	}
	svc := NewService(store, nil, nil)

	deliveries, err := svc.ListDeliveries(context.Background(), "app-1", "user-1", "sent", 10, 5)
	testutil.NoError(t, err)
	testutil.SliceLen(t, deliveries, 1)
	testutil.Equal(t, "d-1", deliveries[0].ID)
}

func TestServiceRunTokenCleanup(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.cleanupStaleTokensFn = func(ctx context.Context, staleDays int) (int64, error) {
		testutil.Equal(t, 270, staleDays)
		return 5, nil
	}
	svc := NewService(store, nil, nil)

	cleaned, err := svc.RunTokenCleanup(context.Background(), 270)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(5), cleaned)
}

func TestServiceProviderCaseInsensitivity(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		return &PushDelivery{ID: id, DeviceTokenID: "tok-1", Provider: ProviderFCM, Title: "hello", Body: "world", Status: DeliveryStatusPending}, nil
	}
	store.getTokenFn = func(ctx context.Context, id string) (*DeviceToken, error) {
		return &DeviceToken{ID: id, Provider: "FCM", Token: "device-token", IsActive: true}, nil
	}

	sendCalled := false
	provider := &stubProvider{
		sendFn: func(ctx context.Context, token string, msg *Message) (*Result, error) {
			sendCalled = true
			return &Result{MessageID: "msg-ok"}, nil
		},
	}
	// Register with uppercase key — should be normalized to lowercase.
	svc := NewService(store, map[string]Provider{"FCM": provider}, nil)

	err := svc.ProcessDelivery(context.Background(), "deliv-1")
	testutil.NoError(t, err)
	testutil.True(t, sendCalled, "provider.Send should be called even with case mismatch")
	testutil.SliceLen(t, store.updateDeliveryStatus, 1)
	testutil.Equal(t, "sent", store.updateDeliveryStatus[0].status)
}

func TestServiceListTokens(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.listTokensFn = func(ctx context.Context, appID, userID string, includeInactive bool) ([]*DeviceToken, error) {
		testutil.Equal(t, "app-1", appID)
		testutil.Equal(t, "user-1", userID)
		testutil.True(t, includeInactive, "expected includeInactive=true")
		return []*DeviceToken{{ID: "tok-1"}}, nil
	}
	svc := NewService(store, nil, nil)

	tokens, err := svc.ListTokens(context.Background(), "app-1", "user-1", true)
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 1)
	testutil.Equal(t, "tok-1", tokens[0].ID)
}

func TestServiceGetToken(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getTokenFn = func(ctx context.Context, id string) (*DeviceToken, error) {
		testutil.Equal(t, "tok-42", id)
		return &DeviceToken{ID: id, Provider: ProviderFCM, Token: "fcm-tok"}, nil
	}
	svc := NewService(store, nil, nil)

	token, err := svc.GetToken(context.Background(), "tok-42")
	testutil.NoError(t, err)
	testutil.NotNil(t, token)
	testutil.Equal(t, "tok-42", token.ID)
}

func TestServiceGetDelivery(t *testing.T) {
	t.Parallel()

	store := &stubPushStore{}
	store.getDeliveryFn = func(ctx context.Context, id string) (*PushDelivery, error) {
		testutil.Equal(t, "deliv-42", id)
		return &PushDelivery{ID: id, Status: DeliveryStatusPending}, nil
	}
	svc := NewService(store, nil, nil)

	delivery, err := svc.GetDelivery(context.Background(), "deliv-42")
	testutil.NoError(t, err)
	testutil.NotNil(t, delivery)
	testutil.Equal(t, "deliv-42", delivery.ID)
}

func TestServiceSendToUserEnqueueFailureMarksDeliveryFailed(t *testing.T) {
	t.Parallel()

	tokens := []*DeviceToken{
		{ID: "tok-1", AppID: "app-1", UserID: "user-1", Provider: ProviderFCM, Platform: PlatformAndroid, Token: "fcm-token"},
	}

	store := &stubPushStore{}
	store.listUserTokensFn = func(ctx context.Context, appID, userID string) ([]*DeviceToken, error) {
		return tokens, nil
	}
	store.recordDeliveryFn = func(ctx context.Context, d *PushDelivery) (*PushDelivery, error) {
		copy := *d
		copy.ID = "deliv-1"
		return &copy, nil
	}

	enqueuer := &stubEnqueuer{}
	enqueuer.enqueueFn = func(ctx context.Context, jobType string, payload json.RawMessage, opts jobs.EnqueueOpts) (*jobs.Job, error) {
		return nil, fmt.Errorf("queue full")
	}

	svc := NewService(store, map[string]Provider{}, enqueuer)
	_, err := svc.SendToUser(context.Background(), "app-1", "user-1", "hello", "world", nil)
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "enqueue delivery job")
	// Delivery should have been marked failed.
	testutil.SliceLen(t, store.updateDeliveryStatus, 1)
	testutil.Equal(t, "failed", store.updateDeliveryStatus[0].status)
	testutil.Equal(t, "enqueue_error", store.updateDeliveryStatus[0].errorCode)
}
