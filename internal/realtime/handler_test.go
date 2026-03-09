package realtime_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

// parseSSEData extracts and parses the JSON from an SSE data line ("data: {...}").
func parseSSEData(t *testing.T, line string) map[string]any {
	t.Helper()
	prefix := "data: "
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("expected SSE data line starting with %q, got: %s", prefix, line)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line[len(prefix):]), &m); err != nil {
		t.Fatalf("parsing SSE data JSON: %v (line: %s)", err, line)
	}
	return m
}

const testJWTSecret = "test-secret-that-is-at-least-32-characters!!"

func testSchemaCache(tables ...string) *schema.CacheHolder {
	sc := &schema.SchemaCache{
		Tables: make(map[string]*schema.Table),
	}
	for _, name := range tables {
		sc.Tables["public."+name] = &schema.Table{
			Schema: "public",
			Name:   name,
			Kind:   "table",
		}
	}
	ch := schema.NewCacheHolder(nil, testutil.DiscardLogger())
	ch.SetForTesting(sc)
	return ch
}

func testAuthService() *auth.Service {
	return auth.NewService(nil, testJWTSecret, time.Hour, 7*24*time.Hour, 8, testutil.DiscardLogger())
}

func validToken() string {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   "user-123",
		"email": "test@example.com",
		"iat":   jwt.NewNumericDate(now),
		"exp":   jwt.NewNumericDate(now.Add(time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(testJWTSecret))
	return signed
}

func expiredToken() string {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   "user-123",
		"email": "test@example.com",
		"iat":   jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		"exp":   jwt.NewNumericDate(now.Add(-time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(testJWTSecret))
	return signed
}

// TestSSEMissingTablesParam tests that the handler returns 400 when tables param is missing.
func TestSSEMissingTablesParam(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/realtime", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "tables parameter is required")
}

// TestSSEUnknownTable tests that the handler returns 400 for unknown table names.
func TestSSEUnknownTable(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/realtime?tables=nonexistent", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "unknown table")
}

// TestSSEAuthRequired tests that auth is enforced when authSvc is non-nil.
func TestSSEAuthRequired(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, testAuthService(), testSchemaCache("posts"), testutil.DiscardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/realtime?tables=posts", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "authentication required")
}

// TestSSEExpiredToken tests that expired tokens are rejected.
func TestSSEExpiredToken(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, testAuthService(), testSchemaCache("posts"), testutil.DiscardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/realtime?tables=posts&token="+expiredToken(), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid or expired token")
}

// TestSSENoAuthWhenDisabled tests that no auth is required when authSvc is nil.
func TestSSENoAuthWhenDisabled(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	// Use a real HTTP server to get proper flushing/streaming.
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=posts")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	testutil.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))

	// Read the connected event.
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == "" && len(lines) > 1 {
			break // End of first event (empty line separator).
		}
	}
	testutil.True(t, len(lines) >= 2, "should have at least event + data lines")
	testutil.Equal(t, "event: connected", lines[0])
	data := parseSSEData(t, lines[1])
	testutil.NotNil(t, data["clientId"])
}

// TestSSETokenInHeader tests auth via Authorization header.
func TestSSETokenInHeader(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	authSvc := testAuthService()
	h := realtime.NewHandler(hub, nil, authSvc, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"?tables=posts", nil)
	req.Header.Set("Authorization", "Bearer "+validToken())

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// Verify the stream delivers the connected event.
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == "" && len(lines) > 1 {
			break
		}
	}
	testutil.True(t, len(lines) >= 2, "should have at least event + data lines")
	testutil.Equal(t, "event: connected", lines[0])
	data := parseSSEData(t, lines[1])
	testutil.NotNil(t, data["clientId"])
}

// TestSSETokenInQueryParam tests auth via token query parameter.
func TestSSETokenInQueryParam(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	authSvc := testAuthService()
	h := realtime.NewHandler(hub, nil, authSvc, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=posts&token=" + validToken())
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// Verify the stream delivers the connected event.
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == "" && len(lines) > 1 {
			break
		}
	}
	testutil.True(t, len(lines) >= 2, "should have at least event + data lines")
	testutil.Equal(t, "event: connected", lines[0])
	data := parseSSEData(t, lines[1])
	testutil.NotNil(t, data["clientId"])
}

// TestSSEReceivesPublishedEvents tests that events published to the hub
// are delivered to connected SSE clients.
func TestSSEReceivesPublishedEvents(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=posts")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, http.StatusOK, resp.StatusCode)

	scanner := bufio.NewScanner(resp.Body)

	// Skip the connected event.
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// Publish an event.
	hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "posts",
		Record: map[string]any{"id": 1, "title": "Hello"},
	})

	// Read the published event.
	var eventLines []string
	for scanner.Scan() {
		line := scanner.Text()
		eventLines = append(eventLines, line)
		if line == "" && len(eventLines) > 1 {
			break
		}
	}

	testutil.True(t, len(eventLines) >= 1, "should have event lines")
	evData := parseSSEData(t, eventLines[0])
	testutil.Equal(t, "posts", evData["table"])
	testutil.Equal(t, "create", evData["action"])
	record, ok := evData["record"].(map[string]any)
	testutil.True(t, ok, "event should contain a record object")
	testutil.Equal(t, "Hello", record["title"])
}

// TestSSEMultipleTables tests subscribing to multiple tables.
func TestSSEMultipleTables(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts", "comments"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=posts,comments")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Skip connected event.
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	// Publish to posts.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})

	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == "" && len(lines) > 1 {
			break
		}
	}
	testutil.True(t, len(lines) >= 1, "should have event lines")
	evData := parseSSEData(t, lines[0])
	testutil.Equal(t, "posts", evData["table"])

	// Publish to comments.
	hub.Publish(&realtime.Event{Action: "create", Table: "comments", Record: map[string]any{"id": 2}})

	lines = nil
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == "" && len(lines) > 1 {
			break
		}
	}
	testutil.True(t, len(lines) >= 1, "should have event lines")
	evData2 := parseSSEData(t, lines[0])
	testutil.Equal(t, "comments", evData2["table"])
}

// --- OAuth SSE Handler Tests ---

// TestOAuthSSEConnected tests that oauth=true creates a client and returns clientId.
func TestOAuthSSEConnected(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?oauth=true")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == "" && len(lines) > 1 {
			break
		}
	}
	testutil.True(t, len(lines) >= 2, "should have event + data lines")
	testutil.Equal(t, "event: connected", lines[0])
	data := parseSSEData(t, lines[1])
	testutil.NotNil(t, data["clientId"])
}

// TestOAuthSSENoAuthRequired tests that oauth=true bypasses JWT auth even when auth is enabled.
func TestOAuthSSENoAuthRequired(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	// Auth service is non-nil (auth enabled), but OAuth SSE should still work without a token.
	h := realtime.NewHandler(hub, nil, testAuthService(), testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?oauth=true")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// Verify the stream delivers the connected event.
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == "" && len(lines) > 1 {
			break
		}
	}
	testutil.True(t, len(lines) >= 2, "should have at least event + data lines")
	testutil.Equal(t, "event: connected", lines[0])
	data := parseSSEData(t, lines[1])
	testutil.NotNil(t, data["clientId"])
}

// TestOAuthSSEReceivesOAuthEvent tests that publishing an OAuth event delivers it to the SSE client.
func TestOAuthSSEReceivesOAuthEvent(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?oauth=true")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Read connected event and extract clientId.
	var connectedDataLine string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			connectedDataLine = line
		}
		if line == "" && connectedDataLine != "" {
			break
		}
	}
	connData := parseSSEData(t, connectedDataLine)
	clientID, ok := connData["clientId"].(string)
	testutil.True(t, ok && clientID != "", "connected event should contain non-empty clientId string")

	// Publish an OAuth event.
	hub.PublishOAuth(clientID, &auth.OAuthEvent{
		Token:        "access-tok",
		RefreshToken: "refresh-tok",
	})

	// Read the oauth event.
	var eventType, eventDataLine string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			eventDataLine = line
		}
		if line == "" && eventDataLine != "" {
			break
		}
	}

	testutil.Equal(t, "oauth", eventType)
	oauthData := parseSSEData(t, eventDataLine)
	testutil.Equal(t, "access-tok", oauthData["token"])
	testutil.Equal(t, "refresh-tok", oauthData["refreshToken"])
}

// TestOAuthSSEClientCleanupOnDisconnect tests that OAuth SSE clients are cleaned up.
func TestOAuthSSEClientCleanupOnDisconnect(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?oauth=true")
	testutil.NoError(t, err)

	// Wait for client registration.
	deadline := time.Now().Add(time.Second)
	for hub.ClientCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	testutil.Equal(t, 1, hub.ClientCount())

	resp.Body.Close()

	// Wait for cleanup.
	deadline = time.Now().Add(time.Second)
	for hub.ClientCount() > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	testutil.Equal(t, 0, hub.ClientCount())
}

// TestSSEClientCleanupOnDisconnect tests that disconnecting cleans up the client.
func TestSSEClientCleanupOnDisconnect(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=posts")
	testutil.NoError(t, err)

	// Wait for the client to be registered.
	deadline := time.Now().Add(time.Second)
	for hub.ClientCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	testutil.Equal(t, 1, hub.ClientCount())

	// Disconnect.
	resp.Body.Close()

	// Wait for cleanup.
	deadline = time.Now().Add(time.Second)
	for hub.ClientCount() > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	testutil.Equal(t, 0, hub.ClientCount())
}
