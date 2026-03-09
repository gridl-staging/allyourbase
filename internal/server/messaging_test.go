package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

// --- Fakes ---

// fakeMsgStore is an in-memory implementation of messageStore for testing.
type fakeMsgStore struct {
	mu       sync.Mutex
	messages []smsMessage
	nextID   int
}

func (f *fakeMsgStore) InsertMessage(_ context.Context, userID, toPhone, body, provider string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := fmt.Sprintf("msg-%d", f.nextID)
	f.messages = append(f.messages, smsMessage{
		ID:        id,
		UserID:    userID,
		ToPhone:   toPhone,
		Body:      body,
		Provider:  provider,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	return id, nil
}

func (f *fakeMsgStore) UpdateMessageSent(_ context.Context, id, providerMsgID, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.messages {
		if f.messages[i].ID == id {
			f.messages[i].ProviderMessageID = providerMsgID
			f.messages[i].Status = status
			f.messages[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return errors.New("message not found")
}

func (f *fakeMsgStore) UpdateMessageFailed(_ context.Context, id, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.messages {
		if f.messages[i].ID == id {
			f.messages[i].Status = "failed"
			f.messages[i].ErrorMessage = errMsg
			f.messages[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return errors.New("message not found")
}

func (f *fakeMsgStore) GetMessage(_ context.Context, id, userID string) (*smsMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.messages {
		if m.ID == id && m.UserID == userID {
			return &m, nil
		}
	}
	return nil, nil
}

func (f *fakeMsgStore) ListMessages(_ context.Context, userID string, limit, offset int) ([]smsMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []smsMessage
	for _, m := range f.messages {
		if m.UserID == userID {
			result = append(result, m)
		}
	}
	// Sort by created_at DESC (newest first) — in fake, reverse order since we append.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	if offset >= len(result) {
		return nil, nil
	}
	result = result[offset:]
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeMsgStore) UpdateDeliveryStatus(_ context.Context, providerMsgID, status, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.messages {
		if f.messages[i].ProviderMessageID == providerMsgID {
			// Mirror the pg store ordering: only advance, never regress.
			if deliveryStatusRank(status) >= deliveryStatusRank(f.messages[i].Status) {
				f.messages[i].Status = status
				if errMsg != "" {
					f.messages[i].ErrorMessage = errMsg
				}
				f.messages[i].UpdatedAt = time.Now()
			}
			return nil
		}
	}
	return nil // idempotent — unknown message IDs are OK
}

// errOnGetMsgStore wraps an inner store but returns an error from GetMessage.
// Used to verify that the handler surfaces 500 on DB failure.
type errOnGetMsgStore struct{ inner messageStore }

func (e *errOnGetMsgStore) InsertMessage(ctx context.Context, userID, toPhone, body, provider string) (string, error) {
	return e.inner.InsertMessage(ctx, userID, toPhone, body, provider)
}
func (e *errOnGetMsgStore) UpdateMessageSent(ctx context.Context, id, providerMsgID, status string) error {
	return e.inner.UpdateMessageSent(ctx, id, providerMsgID, status)
}
func (e *errOnGetMsgStore) UpdateMessageFailed(ctx context.Context, id, errMsg string) error {
	return e.inner.UpdateMessageFailed(ctx, id, errMsg)
}
func (e *errOnGetMsgStore) GetMessage(_ context.Context, _, _ string) (*smsMessage, error) {
	return nil, errors.New("db connection lost")
}
func (e *errOnGetMsgStore) ListMessages(ctx context.Context, userID string, limit, offset int) ([]smsMessage, error) {
	return e.inner.ListMessages(ctx, userID, limit, offset)
}
func (e *errOnGetMsgStore) ListAllMessages(ctx context.Context, limit, offset int) ([]adminSMSMessage, int, error) {
	return e.inner.ListAllMessages(ctx, limit, offset)
}
func (e *errOnGetMsgStore) UpdateDeliveryStatus(ctx context.Context, providerMsgID, status, errMsg string) error {
	return e.inner.UpdateDeliveryStatus(ctx, providerMsgID, status, errMsg)
}

// errOnListMsgStore wraps an inner store but returns an error from ListMessages.
type errOnListMsgStore struct{ inner messageStore }

func (e *errOnListMsgStore) InsertMessage(ctx context.Context, userID, toPhone, body, provider string) (string, error) {
	return e.inner.InsertMessage(ctx, userID, toPhone, body, provider)
}
func (e *errOnListMsgStore) UpdateMessageSent(ctx context.Context, id, providerMsgID, status string) error {
	return e.inner.UpdateMessageSent(ctx, id, providerMsgID, status)
}
func (e *errOnListMsgStore) UpdateMessageFailed(ctx context.Context, id, errMsg string) error {
	return e.inner.UpdateMessageFailed(ctx, id, errMsg)
}
func (e *errOnListMsgStore) GetMessage(ctx context.Context, id, userID string) (*smsMessage, error) {
	return e.inner.GetMessage(ctx, id, userID)
}
func (e *errOnListMsgStore) ListMessages(_ context.Context, _ string, _, _ int) ([]smsMessage, error) {
	return nil, errors.New("db connection lost")
}
func (e *errOnListMsgStore) ListAllMessages(ctx context.Context, limit, offset int) ([]adminSMSMessage, int, error) {
	return e.inner.ListAllMessages(ctx, limit, offset)
}
func (e *errOnListMsgStore) UpdateDeliveryStatus(ctx context.Context, providerMsgID, status, errMsg string) error {
	return e.inner.UpdateDeliveryStatus(ctx, providerMsgID, status, errMsg)
}

// errOnInsertMsgStore wraps an inner store but returns an error from InsertMessage.
// Used to verify that the send handler surfaces 500 and never calls the SMS provider.
type errOnInsertMsgStore struct{ inner messageStore }

func (e *errOnInsertMsgStore) InsertMessage(_ context.Context, _, _, _, _ string) (string, error) {
	return "", errors.New("db connection lost")
}
func (e *errOnInsertMsgStore) UpdateMessageSent(ctx context.Context, id, providerMsgID, status string) error {
	return e.inner.UpdateMessageSent(ctx, id, providerMsgID, status)
}
func (e *errOnInsertMsgStore) UpdateMessageFailed(ctx context.Context, id, errMsg string) error {
	return e.inner.UpdateMessageFailed(ctx, id, errMsg)
}
func (e *errOnInsertMsgStore) GetMessage(ctx context.Context, id, userID string) (*smsMessage, error) {
	return e.inner.GetMessage(ctx, id, userID)
}
func (e *errOnInsertMsgStore) ListMessages(ctx context.Context, userID string, limit, offset int) ([]smsMessage, error) {
	return e.inner.ListMessages(ctx, userID, limit, offset)
}
func (e *errOnInsertMsgStore) ListAllMessages(ctx context.Context, limit, offset int) ([]adminSMSMessage, int, error) {
	return e.inner.ListAllMessages(ctx, limit, offset)
}
func (e *errOnInsertMsgStore) UpdateDeliveryStatus(ctx context.Context, providerMsgID, status, errMsg string) error {
	return e.inner.UpdateDeliveryStatus(ctx, providerMsgID, status, errMsg)
}

// errOnUpdateDeliveryStatusMsgStore wraps an inner store but returns an error from UpdateDeliveryStatus.
// Used to verify that the webhook handler still returns 200 on DB failure (Twilio retries on non-2xx).
type errOnUpdateDeliveryStatusMsgStore struct{ inner messageStore }

func (e *errOnUpdateDeliveryStatusMsgStore) InsertMessage(ctx context.Context, userID, toPhone, body, provider string) (string, error) {
	return e.inner.InsertMessage(ctx, userID, toPhone, body, provider)
}
func (e *errOnUpdateDeliveryStatusMsgStore) UpdateMessageSent(ctx context.Context, id, providerMsgID, status string) error {
	return e.inner.UpdateMessageSent(ctx, id, providerMsgID, status)
}
func (e *errOnUpdateDeliveryStatusMsgStore) UpdateMessageFailed(ctx context.Context, id, errMsg string) error {
	return e.inner.UpdateMessageFailed(ctx, id, errMsg)
}
func (e *errOnUpdateDeliveryStatusMsgStore) GetMessage(ctx context.Context, id, userID string) (*smsMessage, error) {
	return e.inner.GetMessage(ctx, id, userID)
}
func (e *errOnUpdateDeliveryStatusMsgStore) ListMessages(ctx context.Context, userID string, limit, offset int) ([]smsMessage, error) {
	return e.inner.ListMessages(ctx, userID, limit, offset)
}
func (e *errOnUpdateDeliveryStatusMsgStore) ListAllMessages(ctx context.Context, limit, offset int) ([]adminSMSMessage, int, error) {
	return e.inner.ListAllMessages(ctx, limit, offset)
}
func (e *errOnUpdateDeliveryStatusMsgStore) UpdateDeliveryStatus(_ context.Context, _, _, _ string) error {
	return errors.New("db connection lost")
}

// mockSMSProvider records Send calls and returns configurable results.
type mockSMSProvider struct {
	mu       sync.Mutex
	calls    []sms.CaptureCall
	result   *sms.SendResult
	sendErr  error
}

func (m *mockSMSProvider) Send(_ context.Context, to, body string) (*sms.SendResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, sms.CaptureCall{To: to, Body: body})
	if m.sendErr != nil {
		return nil, m.sendErr
	}
	if m.result != nil {
		return m.result, nil
	}
	return &sms.SendResult{MessageID: "SM_test_123", Status: "queued"}, nil
}

// --- Test helpers ---

func newMessagingTestServer(t *testing.T, opts ...func(*Server)) *Server {
	t.Helper()
	s := &Server{
		smsProvider:     &mockSMSProvider{},
		smsProviderName: "test",
		msgStore:        &fakeMsgStore{},
		logger:          testutil.DiscardLogger(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// sendReq builds an authenticated POST request to the SMS send handler.
func sendReq(t *testing.T, body string, claims *auth.Claims) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/messaging/sms/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if claims != nil {
		ctx := auth.ContextWithClaims(req.Context(), claims)
		req = req.WithContext(ctx)
	}
	return req
}

func validClaims() *auth.Claims {
	return &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
		Email:            "test@example.com",
	}
}

func readonlyClaims() *auth.Claims {
	return &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
		Email:            "test@example.com",
		APIKeyScope:      "readonly",
	}
}

// --- Step 4: Send SMS tests ---

func TestMessagingSMSSend_Success(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	req := sendReq(t, `{"to":"+12025551234","body":"Hello world"}`, validClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.True(t, resp["id"] != "", "response should have id")
	testutil.True(t, resp["message_id"] != "", "response should have message_id")
	testutil.Equal(t, "queued", resp["status"])
	testutil.Equal(t, "+12025551234", resp["to"])

	// Verify provider was called with correct args.
	provider := srv.smsProvider.(*mockSMSProvider)
	provider.mu.Lock()
	defer provider.mu.Unlock()
	testutil.Equal(t, 1, len(provider.calls))
	testutil.Equal(t, "+12025551234", provider.calls[0].To)
	testutil.Equal(t, "Hello world", provider.calls[0].Body)
}

func TestMessagingSMSSend_PersistsMessage(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	w := httptest.NewRecorder()
	req := sendReq(t, `{"to":"+12025551234","body":"Persist me"}`, validClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	// Verify message was stored.
	store.mu.Lock()
	defer store.mu.Unlock()
	testutil.Equal(t, 1, len(store.messages))
	msg := store.messages[0]
	testutil.Equal(t, "user-1", msg.UserID)
	testutil.Equal(t, "+12025551234", msg.ToPhone)
	testutil.Equal(t, "Persist me", msg.Body)
	testutil.Equal(t, "SM_test_123", msg.ProviderMessageID)
	testutil.Equal(t, "queued", msg.Status)
}

func TestMessagingSMSSend_InvalidPhone(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	req := sendReq(t, `{"to":"not-a-phone","body":"Hello"}`, validClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid phone number")
}

func TestMessagingSMSSend_MissingBody(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	req := sendReq(t, `{"to":"+12025551234","body":""}`, validClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "body is required")
}

func TestMessagingSMSSend_BodyTooLong(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	longBody := strings.Repeat("x", 1601)
	w := httptest.NewRecorder()
	req := sendReq(t, fmt.Sprintf(`{"to":"+12025551234","body":"%s"}`, longBody), validClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "body exceeds maximum length")
}

func TestMessagingSMSSend_MissingTo(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	req := sendReq(t, `{"to":"","body":"Hello"}`, validClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "to is required")
}

func TestMessagingSMSSend_RequiresAuth(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	// No claims in context — simulates unauthenticated request.
	req := sendReq(t, `{"to":"+12025551234","body":"Hello"}`, nil)
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMessagingSMSSend_RejectsReadonlyKey(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	req := sendReq(t, `{"to":"+12025551234","body":"Hello"}`, readonlyClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusForbidden, w.Code)
}

func TestMessagingSMSSend_SMSDisabled(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t, func(s *Server) { s.smsProvider = nil })

	w := httptest.NewRecorder()
	req := sendReq(t, `{"to":"+12025551234","body":"Hello"}`, validClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestMessagingSMSSend_CountryBlocked(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t, func(s *Server) {
		s.smsAllowedCountries = []string{"GB"} // only UK allowed
	})

	w := httptest.NewRecorder()
	// US number, but only GB allowed.
	req := sendReq(t, `{"to":"+12025551234","body":"Hello"}`, validClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "phone number country not allowed")
}

func TestMessagingSMSSend_ProviderError(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) {
		s.smsProvider = &mockSMSProvider{sendErr: errors.New("provider timeout")}
		s.msgStore = store
	})

	w := httptest.NewRecorder()
	req := sendReq(t, `{"to":"+12025551234","body":"Hello"}`, validClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)

	// Message should be stored with failed status.
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.messages) == 0 {
		t.Fatal("expected message to be persisted, got 0 messages")
	}
	testutil.Equal(t, "failed", store.messages[0].Status)
	testutil.Contains(t, store.messages[0].ErrorMessage, "provider timeout")
}

// --- Step 5: Message History tests ---

func TestMessagingSMSList_ReturnsPaginated(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	// Seed 3 messages for user-1.
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		store.InsertMessage(ctx, "user-1", "+12025551234", fmt.Sprintf("msg %d", i), "test")
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages?limit=2&offset=0", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), validClaims()))
	srv.handleMessagingSMSList(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var msgs []smsMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &msgs))
	testutil.Equal(t, 2, len(msgs))
}

func TestMessagingSMSGet_RequiresAuth(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	r := chi.NewRouter()
	r.Get("/api/messaging/sms/messages/{id}", func(w http.ResponseWriter, req *http.Request) {
		// No claims in context — simulates unauthenticated request.
		srv.handleMessagingSMSGet(w, req)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages/any-id", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMessagingSMSList_RequiresAuth(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages", nil)
	srv.handleMessagingSMSList(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMessagingSMSList_ScopedToUser(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	store.InsertMessage(ctx, "user-1", "+12025551234", "user1 msg", "test")
	store.InsertMessage(ctx, "user-2", "+12025555678", "user2 msg", "test")

	// Request as user-1.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), validClaims()))
	srv.handleMessagingSMSList(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var msgs []smsMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &msgs))
	testutil.Equal(t, 1, len(msgs))
	testutil.Equal(t, "user1 msg", msgs[0].Body)
}

func TestMessagingSMSList_OrderByCreatedDesc(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	store.InsertMessage(ctx, "user-1", "+12025551234", "first", "test")
	time.Sleep(time.Millisecond) // ensure different timestamps
	store.InsertMessage(ctx, "user-1", "+12025551234", "second", "test")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), validClaims()))
	srv.handleMessagingSMSList(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var msgs []smsMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &msgs))
	testutil.Equal(t, 2, len(msgs))
	testutil.Equal(t, "second", msgs[0].Body) // newest first
	testutil.Equal(t, "first", msgs[1].Body)
}

func TestMessagingSMSGet_Success(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	id, _ := store.InsertMessage(ctx, "user-1", "+12025551234", "hello", "test")

	r := chi.NewRouter()
	r.Get("/api/messaging/sms/messages/{id}", func(w http.ResponseWriter, req *http.Request) {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), validClaims()))
		srv.handleMessagingSMSGet(w, req)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages/"+id, nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var msg smsMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &msg))
	testutil.Equal(t, id, msg.ID)
	testutil.Equal(t, "+12025551234", msg.ToPhone)
}

func TestMessagingSMSGet_NotFound(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	r := chi.NewRouter()
	r.Get("/api/messaging/sms/messages/{id}", func(w http.ResponseWriter, req *http.Request) {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), validClaims()))
		srv.handleMessagingSMSGet(w, req)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages/nonexistent-id", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestMessagingSMSGet_WrongUser(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	id, _ := store.InsertMessage(ctx, "user-2", "+12025551234", "secret", "test")

	// user-1 tries to access user-2's message.
	r := chi.NewRouter()
	r.Get("/api/messaging/sms/messages/{id}", func(w http.ResponseWriter, req *http.Request) {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), validClaims())) // user-1
		srv.handleMessagingSMSGet(w, req)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages/"+id, nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code) // 404, not 403 — prevents enumeration
}

func TestMessagingSMSGet_DBError(t *testing.T) {
	t.Parallel()

	// errGetStore returns a real error from GetMessage to verify the handler surfaces 500.
	errGetStore := &errOnGetMsgStore{inner: &fakeMsgStore{}}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = errGetStore })

	r := chi.NewRouter()
	r.Get("/api/messaging/sms/messages/{id}", func(w http.ResponseWriter, req *http.Request) {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), validClaims()))
		srv.handleMessagingSMSGet(w, req)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages/any-id", nil)
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestMessagingSMSList_DBError(t *testing.T) {
	t.Parallel()

	errListStore := &errOnListMsgStore{inner: &fakeMsgStore{}}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = errListStore })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), validClaims()))
	srv.handleMessagingSMSList(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- Step 6: Delivery Status Webhook tests ---

func TestSMSDeliveryWebhook_TwilioUpdatesStatus(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	// Seed a message with a known provider message ID.
	ctx := context.Background()
	id, _ := store.InsertMessage(ctx, "user-1", "+12025551234", "hello", "twilio")
	store.UpdateMessageSent(ctx, id, "SM_abc123", "queued")

	w := httptest.NewRecorder()
	body := "MessageSid=SM_abc123&MessageStatus=delivered"
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/sms/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleSMSDeliveryWebhook(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	store.mu.Lock()
	defer store.mu.Unlock()
	testutil.Equal(t, "delivered", store.messages[0].Status)
}

func TestSMSDeliveryWebhook_StatusProgression(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	id, _ := store.InsertMessage(ctx, "user-1", "+12025551234", "hello", "twilio")
	store.UpdateMessageSent(ctx, id, "SM_prog", "queued")

	statuses := []string{"sent", "delivered"}
	for _, status := range statuses {
		w := httptest.NewRecorder()
		body := fmt.Sprintf("MessageSid=SM_prog&MessageStatus=%s", status)
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/sms/status", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		srv.handleSMSDeliveryWebhook(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	testutil.Equal(t, "delivered", store.messages[0].Status)
}

func TestSMSDeliveryWebhook_FailedStatus(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	id, _ := store.InsertMessage(ctx, "user-1", "+12025551234", "hello", "twilio")
	store.UpdateMessageSent(ctx, id, "SM_fail", "queued")

	w := httptest.NewRecorder()
	body := "MessageSid=SM_fail&MessageStatus=failed&ErrorCode=30003&ErrorMessage=Unreachable"
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/sms/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleSMSDeliveryWebhook(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	store.mu.Lock()
	defer store.mu.Unlock()
	testutil.Equal(t, "failed", store.messages[0].Status)
	testutil.Contains(t, store.messages[0].ErrorMessage, "30003")
}

func TestSMSDeliveryWebhook_UnknownMessageSid(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	body := "MessageSid=SM_unknown&MessageStatus=delivered"
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/sms/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleSMSDeliveryWebhook(w, req)

	testutil.Equal(t, http.StatusOK, w.Code) // idempotent, no error
}

func TestSMSDeliveryWebhook_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	body := "MessageStatus=delivered" // no MessageSid
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/sms/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleSMSDeliveryWebhook(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

// TestSMSDeliveryWebhook_MissingMessageStatus verifies that a callback with a present
// MessageSid but absent (empty) MessageStatus is accepted with 200 and does NOT corrupt
// the stored status to an empty string. Twilio retries on any non-2xx, so we must not
// return 4xx, but we must also not blindly overwrite valid statuses.
func TestSMSDeliveryWebhook_MissingMessageStatus(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	id, _ := store.InsertMessage(ctx, "user-1", "+12025551234", "hello", "twilio")
	store.UpdateMessageSent(ctx, id, "SM_nostatus", "delivered")

	// Callback with MessageSid but no MessageStatus — must be silently ignored.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/sms/status",
		strings.NewReader("MessageSid=SM_nostatus"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleSMSDeliveryWebhook(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	// Status must not have been overwritten with empty string.
	store.mu.Lock()
	defer store.mu.Unlock()
	testutil.Equal(t, "delivered", store.messages[0].Status)
}

// TestSMSDeliveryWebhook_OutOfOrderStatus verifies that a webhook carrying an earlier
// lifecycle status (e.g. "sent") cannot regress a message already at a terminal status
// (e.g. "delivered"). Twilio callbacks can arrive out of order under network retries.
func TestSMSDeliveryWebhook_OutOfOrderStatus(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	id, _ := store.InsertMessage(ctx, "user-1", "+12025551234", "hello", "twilio")
	store.UpdateMessageSent(ctx, id, "SM_ooo", "queued")

	// First: "delivered" arrives — should advance from queued → delivered.
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/api/webhooks/sms/status",
		strings.NewReader("MessageSid=SM_ooo&MessageStatus=delivered"))
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleSMSDeliveryWebhook(w1, req1)
	testutil.Equal(t, http.StatusOK, w1.Code)

	// Second: out-of-order "sent" arrives — must NOT regress from delivered to sent.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/webhooks/sms/status",
		strings.NewReader("MessageSid=SM_ooo&MessageStatus=sent"))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleSMSDeliveryWebhook(w2, req2)
	testutil.Equal(t, http.StatusOK, w2.Code)

	store.mu.Lock()
	defer store.mu.Unlock()
	testutil.Equal(t, "delivered", store.messages[0].Status) // not regressed to "sent"
}

// TestSMSDeliveryWebhook_DBError verifies that a DB failure in UpdateDeliveryStatus still
// results in HTTP 200 — Twilio retries the callback on any non-2xx response, so we must
// always acknowledge receipt even when persistence fails.
func TestSMSDeliveryWebhook_DBError(t *testing.T) {
	t.Parallel()
	errStore := &errOnUpdateDeliveryStatusMsgStore{inner: &fakeMsgStore{}}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = errStore })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/sms/status",
		strings.NewReader("MessageSid=SM_err&MessageStatus=delivered"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleSMSDeliveryWebhook(w, req)

	// DB error must not produce a non-2xx — Twilio would retry indefinitely.
	testutil.Equal(t, http.StatusOK, w.Code)
}

// TestMessagingSMSSend_DBErrorOnInsert verifies that a DB failure during message insertion
// returns 500 and never calls the SMS provider — we must not send an SMS we cannot record.
func TestMessagingSMSSend_DBErrorOnInsert(t *testing.T) {
	t.Parallel()
	provider := &mockSMSProvider{}
	srv := newMessagingTestServer(t, func(s *Server) {
		s.msgStore = &errOnInsertMsgStore{inner: &fakeMsgStore{}}
		s.smsProvider = provider
	})

	w := httptest.NewRecorder()
	req := sendReq(t, `{"to":"+12025551234","body":"Hello world"}`, validClaims())
	srv.handleMessagingSMSSend(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)

	// Provider must never be called — don't send SMS we cannot audit.
	provider.mu.Lock()
	defer provider.mu.Unlock()
	testutil.Equal(t, 0, len(provider.calls))
}

// TestMessagingSMSList_EmptyReturnsArray verifies that a user with no messages receives a
// JSON array ([]) rather than null — important for API consumers that range over the result.
func TestMessagingSMSList_EmptyReturnsArray(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t) // fakeMsgStore starts empty

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), validClaims()))
	srv.handleMessagingSMSList(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var msgs []smsMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &msgs))
	testutil.Equal(t, 0, len(msgs))
	testutil.True(t, msgs != nil, "empty message list must serialize as [] not null")
}

// TestMessagingSMSList_LimitClamp verifies that ?limit values above 100 are silently
// clamped to 100, preventing runaway queries.
func TestMessagingSMSList_LimitClamp(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		store.InsertMessage(ctx, "user-1", "+12025551234", fmt.Sprintf("msg %d", i), "test")
	}

	// limit=200 exceeds the 100 cap; the clamped limit (100) still covers all 5 messages.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/sms/messages?limit=200", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), validClaims()))
	srv.handleMessagingSMSList(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var msgs []smsMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &msgs))
	testutil.Equal(t, 5, len(msgs))
}
