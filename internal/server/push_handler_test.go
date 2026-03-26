package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/push"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

// --- Fake push service for handler tests ---

type fakePushAdmin struct {
	tokens     map[string]*push.DeviceToken
	deliveries map[string]*push.PushDelivery
	nextTokenN int
	nextDelivN int

	registerErr error
	revokeErr   error
	sendErr     error
	listTokErr  error
	listDelErr  error
	getDelErr   error

	lastSendAppID  string
	lastSendUserID string
	lastSendTitle  string
	lastSendBody   string
}

func newFakePushAdmin() *fakePushAdmin {
	return &fakePushAdmin{
		tokens:     make(map[string]*push.DeviceToken),
		deliveries: make(map[string]*push.PushDelivery),
	}
}

func (f *fakePushAdmin) RegisterToken(ctx context.Context, appID, userID, provider, platform, token, deviceName string) (*push.DeviceToken, error) {
	if f.registerErr != nil {
		return nil, f.registerErr
	}
	f.nextTokenN++
	id := fmt.Sprintf("tok-%d", f.nextTokenN)
	dt := &push.DeviceToken{
		ID:       id,
		AppID:    appID,
		UserID:   userID,
		Provider: provider,
		Platform: platform,
		Token:    token,
		IsActive: true,
	}
	if deviceName != "" {
		dt.DeviceName = &deviceName
	}
	f.tokens[id] = dt
	return dt, nil
}

func (f *fakePushAdmin) RevokeToken(ctx context.Context, tokenID string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	t, ok := f.tokens[tokenID]
	if !ok {
		return push.ErrNotFound
	}
	t.IsActive = false
	return nil
}

func (f *fakePushAdmin) ListUserTokens(ctx context.Context, appID, userID string) ([]*push.DeviceToken, error) {
	if f.listTokErr != nil {
		return nil, f.listTokErr
	}
	var result []*push.DeviceToken
	for _, t := range f.tokens {
		if t.AppID == appID && t.UserID == userID && t.IsActive {
			result = append(result, t)
		}
	}
	return result, nil
}

func (f *fakePushAdmin) ListTokens(ctx context.Context, appID, userID string, includeInactive bool) ([]*push.DeviceToken, error) {
	if f.listTokErr != nil {
		return nil, f.listTokErr
	}
	var result []*push.DeviceToken
	for _, t := range f.tokens {
		if appID != "" && t.AppID != appID {
			continue
		}
		if userID != "" && t.UserID != userID {
			continue
		}
		if !includeInactive && !t.IsActive {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

func (f *fakePushAdmin) GetToken(ctx context.Context, id string) (*push.DeviceToken, error) {
	t, ok := f.tokens[id]
	if !ok {
		return nil, push.ErrNotFound
	}
	return t, nil
}

func (f *fakePushAdmin) SendToUser(ctx context.Context, appID, userID, title, body string, data map[string]string) ([]*push.PushDelivery, error) {
	f.lastSendAppID = appID
	f.lastSendUserID = userID
	f.lastSendTitle = title
	f.lastSendBody = body
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	f.nextDelivN++
	d := &push.PushDelivery{
		ID:     fmt.Sprintf("deliv-%d", f.nextDelivN),
		AppID:  appID,
		UserID: userID,
		Title:  title,
		Body:   body,
		Status: push.DeliveryStatusPending,
	}
	f.deliveries[d.ID] = d
	return []*push.PushDelivery{d}, nil
}

func (f *fakePushAdmin) SendToToken(ctx context.Context, tokenID, title, body string, data map[string]string) (*push.PushDelivery, error) {
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	f.nextDelivN++
	d := &push.PushDelivery{
		ID:            fmt.Sprintf("deliv-%d", f.nextDelivN),
		DeviceTokenID: tokenID,
		Title:         title,
		Body:          body,
		Status:        push.DeliveryStatusPending,
	}
	f.deliveries[d.ID] = d
	return d, nil
}

func (f *fakePushAdmin) ListDeliveries(ctx context.Context, appID, userID, status string, limit, offset int) ([]*push.PushDelivery, error) {
	if f.listDelErr != nil {
		return nil, f.listDelErr
	}
	var result []*push.PushDelivery
	for _, d := range f.deliveries {
		if appID != "" && d.AppID != appID {
			continue
		}
		if userID != "" && d.UserID != userID {
			continue
		}
		if status != "" && d.Status != status {
			continue
		}
		result = append(result, d)
	}
	return result, nil
}

func (f *fakePushAdmin) GetDelivery(ctx context.Context, id string) (*push.PushDelivery, error) {
	if f.getDelErr != nil {
		return nil, f.getDelErr
	}
	d, ok := f.deliveries[id]
	if !ok {
		return nil, push.ErrNotFound
	}
	return d, nil
}

// --- Helpers ---

func pushClaims(userID string) *auth.Claims {
	return &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: userID},
		Email:            userID + "@example.com",
	}
}

func pushRouter(svc pushAdmin) *chi.Mux {
	r := chi.NewRouter()
	// User-facing routes (inject claims via context directly in tests).
	r.Post("/api/push/devices", handleUserPushRegister(svc))
	r.Get("/api/push/devices", handleUserPushListDevices(svc))
	r.Delete("/api/push/devices/{id}", handleUserPushRevokeDevice(svc))
	// Admin routes.
	r.Get("/api/admin/push/devices", handleAdminPushListDevices(svc))
	r.Post("/api/admin/push/devices", handleAdminPushRegisterDevice(svc))
	r.Delete("/api/admin/push/devices/{id}", handleAdminPushRevokeDevice(svc))
	r.Post("/api/admin/push/send", handleAdminPushSend(svc))
	r.Post("/api/admin/push/send-to-token", handleAdminPushSendToToken(svc))
	r.Get("/api/admin/push/deliveries", handleAdminPushListDeliveries(svc))
	r.Get("/api/admin/push/deliveries/{id}", handleAdminPushGetDelivery(svc))
	return r
}

func newPushServerWithAdminToken(t *testing.T) (*Server, string) {
	t.Helper()
	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cache := schema.NewCacheHolder(nil, logger)
	srv := New(cfg, logger, cache, nil, nil, nil)
	return srv, srv.adminAuth.token()
}

func TestServerPushAdminRoutes_NotEnabled_Returns503(t *testing.T) {
	t.Parallel()
	srv, token := newPushServerWithAdminToken(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/push/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestServerPushAdminRoutes_WiredWithSetPushService(t *testing.T) {
	t.Parallel()
	srv, token := newPushServerWithAdminToken(t)
	fake := newFakePushAdmin()
	fake.tokens["tok-1"] = &push.DeviceToken{ID: "tok-1", AppID: "app-1", UserID: "user-1", IsActive: true}
	srv.SetPushService(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/push/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Items []*push.DeviceToken `json:"items"`
	}
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, 1, len(resp.Items))
	testutil.Equal(t, "tok-1", resp.Items[0].ID)
}

// --- User-facing endpoint tests ---

func TestUserPushRegisterDevice(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	body := `{"app_id":"app-1","provider":"fcm","platform":"android","token":"fcm-tok-123","device_name":"Pixel 7"}`
	req := httptest.NewRequest("POST", "/api/push/devices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithClaims(req.Context(), pushClaims("user-1")))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusCreated, rec.Code)

	var resp push.DeviceToken
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, "app-1", resp.AppID)
	testutil.Equal(t, "user-1", resp.UserID)
	testutil.Equal(t, "fcm", resp.Provider)
	testutil.Equal(t, "android", resp.Platform)
	testutil.Equal(t, "fcm-tok-123", resp.Token)
}

func TestUserPushRegisterDevice_NoClaims(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	body := `{"app_id":"app-1","provider":"fcm","platform":"android","token":"tok"}`
	req := httptest.NewRequest("POST", "/api/push/devices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUserPushRegisterDevice_MissingFields(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	body := `{"provider":"fcm","platform":"android","token":"tok"}`
	req := httptest.NewRequest("POST", "/api/push/devices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithClaims(req.Context(), pushClaims("user-1")))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUserPushRegisterDevice_InvalidProvider(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.registerErr = fmt.Errorf("%w: \"invalid\"", push.ErrInvalidProvider)
	router := pushRouter(fake)

	body := `{"app_id":"app-1","provider":"invalid","platform":"android","token":"tok"}`
	req := httptest.NewRequest("POST", "/api/push/devices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithClaims(req.Context(), pushClaims("user-1")))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUserPushListDevices(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.tokens["tok-1"] = &push.DeviceToken{ID: "tok-1", AppID: "app-1", UserID: "user-1", Provider: "fcm", IsActive: true}
	fake.tokens["tok-2"] = &push.DeviceToken{ID: "tok-2", AppID: "app-1", UserID: "user-2", Provider: "fcm", IsActive: true}
	router := pushRouter(fake)

	req := httptest.NewRequest("GET", "/api/push/devices?app_id=app-1", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), pushClaims("user-1")))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []*push.DeviceToken `json:"items"`
	}
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, 1, len(resp.Items))
	testutil.Equal(t, "user-1", resp.Items[0].UserID)
}

func TestUserPushListDevices_MissingAppID(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	req := httptest.NewRequest("GET", "/api/push/devices", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), pushClaims("user-1")))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUserPushRevokeDevice(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.tokens["tok-1"] = &push.DeviceToken{ID: "tok-1", AppID: "app-1", UserID: "user-1", IsActive: true}
	router := pushRouter(fake)

	req := httptest.NewRequest("DELETE", "/api/push/devices/tok-1", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), pushClaims("user-1")))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNoContent, rec.Code)
	testutil.True(t, !fake.tokens["tok-1"].IsActive, "token should be revoked")
}

func TestUserPushRevokeDevice_OwnershipValidation(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.tokens["tok-1"] = &push.DeviceToken{ID: "tok-1", AppID: "app-1", UserID: "user-2", IsActive: true}
	router := pushRouter(fake)

	req := httptest.NewRequest("DELETE", "/api/push/devices/tok-1", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), pushClaims("user-1")))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNotFound, rec.Code)
	testutil.True(t, fake.tokens["tok-1"].IsActive, "token should NOT be revoked (different user)")
}

func TestUserPushRevokeDevice_NotFound(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	req := httptest.NewRequest("DELETE", "/api/push/devices/nonexistent", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), pushClaims("user-1")))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNotFound, rec.Code)
}

// --- Admin endpoint tests ---

func TestAdminPushListDevices(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.tokens["tok-1"] = &push.DeviceToken{ID: "tok-1", AppID: "app-1", UserID: "user-1", Provider: "fcm", IsActive: true}
	fake.tokens["tok-2"] = &push.DeviceToken{ID: "tok-2", AppID: "app-1", UserID: "user-2", Provider: "apns", IsActive: false}
	router := pushRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/push/devices?include_inactive=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Items []*push.DeviceToken `json:"items"`
	}
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, 2, len(resp.Items))
}

func TestAdminPushListDevices_FilterByAppID(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.tokens["tok-1"] = &push.DeviceToken{ID: "tok-1", AppID: "app-1", UserID: "user-1", IsActive: true}
	fake.tokens["tok-2"] = &push.DeviceToken{ID: "tok-2", AppID: "app-2", UserID: "user-1", IsActive: true}
	router := pushRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/push/devices?app_id=app-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Items []*push.DeviceToken `json:"items"`
	}
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, 1, len(resp.Items))
}

func TestAdminPushListDevices_FilterByUserID(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.tokens["tok-1"] = &push.DeviceToken{ID: "tok-1", AppID: "app-1", UserID: "user-1", IsActive: true}
	fake.tokens["tok-2"] = &push.DeviceToken{ID: "tok-2", AppID: "app-1", UserID: "user-2", IsActive: true}
	router := pushRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/push/devices?user_id=user-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Items []*push.DeviceToken `json:"items"`
	}
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, 1, len(resp.Items))
	testutil.Equal(t, "user-1", resp.Items[0].UserID)
}

func TestAdminPushRegisterDevice(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	body := `{"app_id":"app-1","user_id":"user-1","provider":"apns","platform":"ios","token":"apns-tok","device_name":"iPhone"}`
	req := httptest.NewRequest("POST", "/api/admin/push/devices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusCreated, rec.Code)
	var resp push.DeviceToken
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, "app-1", resp.AppID)
	testutil.Equal(t, "user-1", resp.UserID)
}

func TestAdminPushRegisterDevice_MissingFields(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	body := `{"app_id":"app-1","provider":"fcm","platform":"android","token":"tok"}`
	req := httptest.NewRequest("POST", "/api/admin/push/devices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminPushRevokeDevice(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.tokens["tok-1"] = &push.DeviceToken{ID: "tok-1", AppID: "app-1", UserID: "user-1", IsActive: true}
	router := pushRouter(fake)

	req := httptest.NewRequest("DELETE", "/api/admin/push/devices/tok-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNoContent, rec.Code)
}

func TestAdminPushRevokeDevice_NotFound(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.revokeErr = push.ErrNotFound
	router := pushRouter(fake)

	req := httptest.NewRequest("DELETE", "/api/admin/push/devices/nonexistent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAdminPushSend(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	body := `{"app_id":"app-1","user_id":"user-1","title":"Hello","body":"World","data":{"key":"val"}}`
	req := httptest.NewRequest("POST", "/api/admin/push/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "app-1", fake.lastSendAppID)
	testutil.Equal(t, "user-1", fake.lastSendUserID)
	testutil.Equal(t, "Hello", fake.lastSendTitle)
	testutil.Equal(t, "World", fake.lastSendBody)
}

func TestAdminPushSend_MissingTitle(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	body := `{"app_id":"app-1","user_id":"user-1","body":"World"}`
	req := httptest.NewRequest("POST", "/api/admin/push/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminPushSend_InvalidPayload(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.sendErr = fmt.Errorf("%w: title is required", push.ErrInvalidPayload)
	router := pushRouter(fake)

	body := `{"app_id":"app-1","user_id":"user-1","title":"Hi","body":"There"}`
	req := httptest.NewRequest("POST", "/api/admin/push/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminPushSend_PayloadTooLarge(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.sendErr = fmt.Errorf("%w: too big", push.ErrPayloadTooLarge)
	router := pushRouter(fake)

	body := `{"app_id":"app-1","user_id":"user-1","title":"Hi","body":"There"}`
	req := httptest.NewRequest("POST", "/api/admin/push/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminPushSendToToken(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	body := `{"token_id":"tok-1","title":"Hello","body":"World"}`
	req := httptest.NewRequest("POST", "/api/admin/push/send-to-token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
}

func TestAdminPushSendToToken_MissingTokenID(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	body := `{"title":"Hello","body":"World"}`
	req := httptest.NewRequest("POST", "/api/admin/push/send-to-token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminPushListDeliveries_InvalidStatus(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/push/deliveries?status=bogus", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
	testutil.Contains(t, rec.Body.String(), "status must be one of: pending, sent, failed, invalid_token")
}

func TestAdminPushListDeliveries(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.deliveries["d-1"] = &push.PushDelivery{ID: "d-1", AppID: "app-1", UserID: "user-1", Status: "sent"}
	fake.deliveries["d-2"] = &push.PushDelivery{ID: "d-2", AppID: "app-1", UserID: "user-1", Status: "failed"}
	router := pushRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/push/deliveries?status=sent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Items []*push.PushDelivery `json:"items"`
	}
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, 1, len(resp.Items))
	testutil.Equal(t, "sent", resp.Items[0].Status)
}

func TestAdminPushGetDelivery(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	fake.deliveries["d-1"] = &push.PushDelivery{ID: "d-1", AppID: "app-1", Title: "Test", Status: "sent"}
	router := pushRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/push/deliveries/d-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
	var resp push.PushDelivery
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, "d-1", resp.ID)
	testutil.Equal(t, "Test", resp.Title)
}

func TestAdminPushGetDelivery_NotFound(t *testing.T) {
	t.Parallel()
	fake := newFakePushAdmin()
	router := pushRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/push/deliveries/nonexistent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNotFound, rec.Code)
}
