package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
)

// mockWebhookStore is an in-memory WebhookStore for handler tests.
type mockWebhookStore struct {
	hooks  map[string]*Webhook
	nextID int
}

func newMockStore() *mockWebhookStore {
	return &mockWebhookStore{hooks: make(map[string]*Webhook)}
}

func (m *mockWebhookStore) List(_ context.Context) ([]Webhook, error) {
	result := make([]Webhook, 0, len(m.hooks))
	for _, h := range m.hooks {
		result = append(result, *h)
	}
	return result, nil
}

func (m *mockWebhookStore) Get(_ context.Context, id string) (*Webhook, error) {
	h, ok := m.hooks[id]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	return h, nil
}

func (m *mockWebhookStore) Create(_ context.Context, w *Webhook) error {
	m.nextID++
	w.ID = fmt.Sprintf("test-uuid-%d", m.nextID)
	m.hooks[w.ID] = w
	return nil
}

func (m *mockWebhookStore) Update(_ context.Context, id string, w *Webhook) error {
	if _, ok := m.hooks[id]; !ok {
		return pgx.ErrNoRows
	}
	w.ID = id
	m.hooks[id] = w
	return nil
}

func (m *mockWebhookStore) Delete(_ context.Context, id string) error {
	if _, ok := m.hooks[id]; !ok {
		return pgx.ErrNoRows
	}
	delete(m.hooks, id)
	return nil
}

func (m *mockWebhookStore) ListEnabled(_ context.Context) ([]Webhook, error) {
	var result []Webhook
	for _, h := range m.hooks {
		if h.Enabled {
			result = append(result, *h)
		}
	}
	return result, nil
}

// mockDeliveryStore is an in-memory DeliveryStore for handler tests.
type mockDeliveryStore struct {
	deliveries     map[string]*Delivery
	nextID         int
	pruneCalls     int
	pruneOlderThan time.Duration
	pruneResult    int64
	pruneErr       error
}

func newMockDeliveryStore() *mockDeliveryStore {
	return &mockDeliveryStore{deliveries: make(map[string]*Delivery)}
}

func (m *mockDeliveryStore) RecordDelivery(_ context.Context, d *Delivery) error {
	m.nextID++
	d.ID = fmt.Sprintf("del-uuid-%d", m.nextID)
	m.deliveries[d.ID] = d
	return nil
}

func (m *mockDeliveryStore) ListDeliveries(_ context.Context, webhookID string, page, perPage int) ([]Delivery, int, error) {
	var result []Delivery
	for _, d := range m.deliveries {
		if d.WebhookID == webhookID {
			result = append(result, *d)
		}
	}
	total := len(result)
	offset := (page - 1) * perPage
	if offset >= len(result) {
		return []Delivery{}, total, nil
	}
	end := offset + perPage
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], total, nil
}

func (m *mockDeliveryStore) GetDelivery(_ context.Context, webhookID, deliveryID string) (*Delivery, error) {
	d, ok := m.deliveries[deliveryID]
	if !ok || d.WebhookID != webhookID {
		return nil, pgx.ErrNoRows
	}
	return d, nil
}

func (m *mockDeliveryStore) PruneDeliveries(_ context.Context, olderThan time.Duration) (int64, error) {
	m.pruneCalls++
	m.pruneOlderThan = olderThan
	return m.pruneResult, m.pruneErr
}

func testHandler() (*Handler, *mockWebhookStore, *mockDeliveryStore) {
	store := newMockStore()
	ds := newMockDeliveryStore()
	h := NewHandler(store, ds, testutil.DiscardLogger())
	return h, store, ds
}

func doHandlerRequest(t *testing.T, handler http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestCreateMissingURL(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "POST", "/", `{"events":["create"]}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "url is required")
}

func TestCreateInvalidEvents(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "POST", "/", `{"url":"http://example.com","events":["invalid"]}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid event")
}

func TestCreateInvalidURL(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "POST", "/", `{"url":"not-a-url"}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "url must be an absolute http or https URL")
}

func TestCreateRejectsCredentialedURL(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "POST", "/", `{"url":"https://user:pass@example.com/hook"}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "url must not include embedded credentials")
}

func TestCreateSuccess(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "POST", "/",
		`{"url":"http://example.com/hook","secret":"mysecret","events":["create","update"]}`)
	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, "http://example.com/hook", resp["url"].(string))
	testutil.Equal(t, true, resp["hasSecret"].(bool))
}

func TestGetNotFound(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "GET", "/nonexistent-id", "")
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteNotFound(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "DELETE", "/nonexistent-id", "")
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestSecretNeverInResponse(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()

	// Create a webhook with a secret.
	w := doHandlerRequest(t, h.Routes(), "POST", "/",
		`{"url":"http://example.com/hook","secret":"super-secret"}`)
	testutil.Equal(t, http.StatusCreated, w.Code)

	body := w.Body.String()
	testutil.True(t, !strings.Contains(body, "super-secret"), "response must not contain the secret value")
	testutil.Contains(t, body, `"hasSecret":true`)
	testutil.True(t, !strings.Contains(body, `"secret"`), "response must not contain the secret key")

	// List also must not contain secret.
	w = doHandlerRequest(t, h.Routes(), "GET", "/", "")
	testutil.Equal(t, http.StatusOK, w.Code)
	body = w.Body.String()
	testutil.True(t, !strings.Contains(body, "super-secret"), "list response must not contain the secret value")
}

func TestCreateDefaultEvents(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "POST", "/", `{"url":"http://example.com/hook"}`)
	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	events := resp["events"].([]any)
	testutil.Equal(t, 3, len(events))
	got := make([]string, len(events))
	for i, e := range events {
		got[i] = e.(string)
	}
	sort.Strings(got)
	testutil.Equal(t, "create", got[0])
	testutil.Equal(t, "delete", got[1])
	testutil.Equal(t, "update", got[2])
}

func TestCreateDisabled(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "POST", "/",
		`{"url":"http://example.com/hook","enabled":false}`)
	testutil.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, false, resp["enabled"].(bool))
}

func TestGetSuccess(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()

	// Create first.
	w := doHandlerRequest(t, h.Routes(), "POST", "/",
		`{"url":"http://example.com/hook","events":["create"]}`)
	testutil.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	id := created["id"].(string)

	// GET by ID.
	w = doHandlerRequest(t, h.Routes(), "GET", "/"+id, "")
	testutil.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	testutil.Equal(t, id, got["id"].(string))
	testutil.Equal(t, "http://example.com/hook", got["url"].(string))
}

func TestDeleteSuccess(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()

	// Create first.
	w := doHandlerRequest(t, h.Routes(), "POST", "/",
		`{"url":"http://example.com/hook"}`)
	testutil.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	id := created["id"].(string)

	// Delete.
	w = doHandlerRequest(t, h.Routes(), "DELETE", "/"+id, "")
	testutil.Equal(t, http.StatusNoContent, w.Code)

	// GET should now 404.
	w = doHandlerRequest(t, h.Routes(), "GET", "/"+id, "")
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestListAfterCreate(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()

	// Empty list.
	w := doHandlerRequest(t, h.Routes(), "GET", "/", "")
	testutil.Equal(t, http.StatusOK, w.Code)
	var emptyResp map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&emptyResp))
	testutil.Equal(t, 0, len(emptyResp["items"].([]any)))

	// Create two.
	doHandlerRequest(t, h.Routes(), "POST", "/", `{"url":"http://example.com/a"}`)
	doHandlerRequest(t, h.Routes(), "POST", "/", `{"url":"http://example.com/b"}`)

	w = doHandlerRequest(t, h.Routes(), "GET", "/", "")
	testutil.Equal(t, http.StatusOK, w.Code)
	var listResp map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&listResp))
	items := listResp["items"].([]any)
	testutil.Equal(t, 2, len(items))
	// Verify the created webhooks are actually present with correct URLs.
	urls := map[string]bool{}
	for _, item := range items {
		urls[item.(map[string]any)["url"].(string)] = true
	}
	testutil.True(t, urls["http://example.com/a"], "should contain webhook a")
	testutil.True(t, urls["http://example.com/b"], "should contain webhook b")
}

func TestUpdateSuccess(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()

	// Create.
	w := doHandlerRequest(t, h.Routes(), "POST", "/",
		`{"url":"http://example.com/hook","events":["create"],"secret":"old-secret"}`)
	testutil.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	id := created["id"].(string)

	// PATCH — change URL and events, leave secret untouched.
	w = doHandlerRequest(t, h.Routes(), "PATCH", "/"+id,
		`{"url":"http://example.com/updated","events":["create","delete"]}`)
	testutil.Equal(t, http.StatusOK, w.Code)

	var updated map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	testutil.Equal(t, "http://example.com/updated", updated["url"].(string))
	events := updated["events"].([]any)
	testutil.Equal(t, 2, len(events))
	eventSet := map[string]bool{}
	for _, e := range events {
		eventSet[e.(string)] = true
	}
	testutil.True(t, eventSet["create"], "events should contain 'create'")
	testutil.True(t, eventSet["delete"], "events should contain 'delete'")
	// Secret wasn't sent in PATCH, so it should still be set.
	testutil.Equal(t, true, updated["hasSecret"].(bool))
}

func TestUpdateNotFound(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "PATCH", "/nonexistent-id",
		`{"url":"http://example.com/updated"}`)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateInvalidEvents(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()

	// Create first.
	w := doHandlerRequest(t, h.Routes(), "POST", "/",
		`{"url":"http://example.com/hook"}`)
	testutil.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	id := created["id"].(string)

	// PATCH with invalid event.
	w = doHandlerRequest(t, h.Routes(), "PATCH", "/"+id,
		`{"events":["bogus"]}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid event")
}

func TestUpdateInvalidURL(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()

	w := doHandlerRequest(t, h.Routes(), "POST", "/", `{"url":"http://example.com/hook"}`)
	testutil.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	id := created["id"].(string)

	w = doHandlerRequest(t, h.Routes(), "PATCH", "/"+id, `{"url":"not-a-url"}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "url must be an absolute http or https URL")
}

func TestUpdateRejectsCredentialedURL(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()

	w := doHandlerRequest(t, h.Routes(), "POST", "/", `{"url":"http://example.com/hook"}`)
	testutil.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	id := created["id"].(string)

	w = doHandlerRequest(t, h.Routes(), "PATCH", "/"+id, `{"url":"https://user:pass@example.com/updated"}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "url must not include embedded credentials")
}

func TestUpdateClearEventsWithExplicitEmptyArray(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()

	// Create with non-empty events so PATCH can explicitly clear them.
	w := doHandlerRequest(t, h.Routes(), "POST", "/",
		`{"url":"http://example.com/hook","events":["create","update"]}`)
	testutil.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	id := created["id"].(string)

	// PATCH with explicit empty events array.
	w = doHandlerRequest(t, h.Routes(), "PATCH", "/"+id, `{"events":[]}`)
	testutil.Equal(t, http.StatusOK, w.Code)
	var updated map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	updatedEvents := updated["events"].([]any)
	testutil.Equal(t, 0, len(updatedEvents))

	// Verify persistence through GET to ensure merge behavior is correct.
	w = doHandlerRequest(t, h.Routes(), "GET", "/"+id, "")
	testutil.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	gotEvents := got["events"].([]any)
	testutil.Equal(t, 0, len(gotEvents))
}

func TestTestNotFound(t *testing.T) {
	t.Parallel()
	h, _, _ := testHandler()
	w := doHandlerRequest(t, h.Routes(), "POST", "/nonexistent-id/test", "")
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestTestSuccess(t *testing.T) {
	t.Parallel()
	var receivedBody []byte
	var receivedSig string
	var receivedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		receivedSig = r.Header.Get("X-AYB-Signature")
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	h, store, _ := testHandler()
	store.hooks["wh1"] = &Webhook{
		ID:     "wh1",
		URL:    srv.URL,
		Secret: "test-secret",
		Events: []string{"create"},
		Tables: []string{},
	}

	w := doHandlerRequest(t, h.Routes(), "POST", "/wh1/test", "")
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, true, resp.Success)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, resp.DurationMs >= 0, "durationMs should be non-negative")

	// Verify the test server received correct payload.
	testutil.Equal(t, "application/json", receivedContentType)
	testutil.True(t, len(receivedBody) > 0, "body should not be empty")
	testutil.Contains(t, string(receivedBody), `"action":"test"`)
	testutil.Contains(t, string(receivedBody), `"_ayb_test"`)

	// Verify HMAC signature was sent.
	testutil.True(t, receivedSig != "", "X-AYB-Signature should be set")
	testutil.Equal(t, Sign("test-secret", receivedBody), receivedSig)
}

func TestTestNoSecret(t *testing.T) {
	t.Parallel()
	var receivedSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-AYB-Signature")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	h, store, _ := testHandler()
	store.hooks["wh1"] = &Webhook{ID: "wh1", URL: srv.URL}

	w := doHandlerRequest(t, h.Routes(), "POST", "/wh1/test", "")
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, true, resp.Success)
	testutil.Equal(t, "", receivedSig)
}

func TestTestTargetReturns500(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	h, store, _ := testHandler()
	store.hooks["wh1"] = &Webhook{ID: "wh1", URL: srv.URL}

	w := doHandlerRequest(t, h.Routes(), "POST", "/wh1/test", "")
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, false, resp.Success)
	testutil.Equal(t, 500, resp.StatusCode)
}

func TestTestConnectionRefused(t *testing.T) {
	// Start and immediately close a server to get a refused connection.
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	h, store, _ := testHandler()
	store.hooks["wh1"] = &Webhook{ID: "wh1", URL: url}

	w := doHandlerRequest(t, h.Routes(), "POST", "/wh1/test", "")
	testutil.Equal(t, http.StatusOK, w.Code)

	var resp testResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, false, resp.Success)
	testutil.True(t, resp.Error != "", "error message should be present")
	testutil.Contains(t, resp.Error, "connect")
	testutil.True(t, resp.DurationMs >= 0, "durationMs should be non-negative")
}
