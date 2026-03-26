package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

type stubGQLWSAuthValidator struct {
	validateToken  func(token string) (*auth.Claims, error)
	validateAPIKey func(ctx context.Context, token string) (*auth.Claims, error)
}

func (s *stubGQLWSAuthValidator) ValidateToken(token string) (*auth.Claims, error) {
	if s.validateToken != nil {
		return s.validateToken(token)
	}
	return nil, errors.New("invalid token")
}

func (s *stubGQLWSAuthValidator) ValidateAPIKey(ctx context.Context, token string) (*auth.Claims, error) {
	if s.validateAPIKey != nil {
		return s.validateAPIKey(ctx, token)
	}
	return nil, errors.New("invalid api key")
}

func newTestGQLWSHandler(t *testing.T) *GQLWSHandler {
	t.Helper()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", TypeName: "text"},
		},
	}})

	h := NewGQLWSHandler(nil, func() *schema.SchemaCache { return cache }, testutil.DiscardLogger())
	h.InitTimeout = 120 * time.Millisecond
	h.PingInterval = 80 * time.Millisecond
	return h
}

func dialGQLWS(t *testing.T, srv *httptest.Server, withSubprotocol bool) (*websocket.Conn, *http.Response) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialer := websocket.Dialer{}
	if withSubprotocol {
		dialer.Subprotocols = []string{"graphql-transport-ws"}
	}
	conn, resp, err := dialer.Dial(wsURL, nil)
	testutil.NoError(t, err)
	return conn, resp
}

func writeGQLWSMessage(t *testing.T, conn *websocket.Conn, msg gqlwsMessage) {
	t.Helper()
	testutil.NoError(t, conn.WriteJSON(msg))
}

func readGQLWSMessage(t *testing.T, conn *websocket.Conn) gqlwsMessage {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg gqlwsMessage
	testutil.NoError(t, conn.ReadJSON(&msg))
	return msg
}

func expectCloseCode(t *testing.T, conn *websocket.Conn, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn.SetReadDeadline(deadline)
		_, _, err := conn.ReadMessage()
		if err == nil {
			continue
		}
		closeErr := &websocket.CloseError{}
		testutil.True(t, errors.As(err, &closeErr), "expected close error, got: %v", err)
		testutil.Equal(t, want, closeErr.Code)
		return
	}
}

func waitForWSSubRegistered(t *testing.T, h *Handler, connID, subID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.subMu.Lock()
		subs := h.wsSubs[connID]
		_, ok := subs[subID]
		h.subMu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("subscription %s on conn %s was not registered", subID, connID)
}

func TestGQLWSSubprotocolNegotiation(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	testutil.Equal(t, "graphql-transport-ws", conn.Subprotocol())
}

func TestGQLWSMissingSubprotocolClosesWithProtocolError(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, false)
	defer conn.Close()
	expectCloseCode(t, conn, closeInternalError)
}

func TestGQLWSConnectionInitAck(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	msg := readGQLWSMessage(t, conn)
	testutil.Equal(t, gqlwsConnectionAck, msg.Type)
}

func TestGQLWSConnectionInitWithAuthSetsClaims(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	h.authValidator = &stubGQLWSAuthValidator{
		validateToken: func(token string) (*auth.Claims, error) {
			if token == "valid-token" {
				return &auth.Claims{Email: "user@example.com"}, nil
			}
			return nil, errors.New("bad token")
		},
	}

	claimsSeen := make(chan *auth.Claims, 1)
	h.OnSubscribe = func(ctx context.Context, conn *GQLWSConn, id string, payload gqlwsSubscribePayload) {
		claimsSeen <- conn.Claims()
	}

	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	initPayload, err := json.Marshal(gqlwsConnectionInitPayload{Authorization: "Bearer valid-token"})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit, Payload: initPayload})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	subPayload, err := json.Marshal(gqlwsSubscribePayload{Query: `subscription { posts { id } }`})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsSubscribe, Payload: subPayload})

	select {
	case claims := <-claimsSeen:
		testutil.NotNil(t, claims)
		testutil.Equal(t, "user@example.com", claims.Email)
	case <-time.After(2 * time.Second):
		t.Fatal("expected OnSubscribe callback with claims")
	}
}

func TestGQLWSConnectionInitInvalidAuthClosesUnauthorized(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	h.authValidator = &stubGQLWSAuthValidator{
		validateToken: func(token string) (*auth.Claims, error) {
			return nil, errors.New("invalid")
		},
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	initPayload, err := json.Marshal(gqlwsConnectionInitPayload{Authorization: "Bearer no"})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit, Payload: initPayload})
	expectCloseCode(t, conn, closeUnauthorized)
}

func TestGQLWSConnectionInitTimeout(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	h.InitTimeout = 50 * time.Millisecond
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()
	expectCloseCode(t, conn, closeInitTimeout)
}

func TestGQLWSDoubleConnectionInitCloses(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)
	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	expectCloseCode(t, conn, closeTooManyInitRequests)
}

func TestGQLWSSubscribeBeforeInitClosesUnauthorized(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	subPayload, err := json.Marshal(gqlwsSubscribePayload{Query: `subscription { posts { id } }`})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsSubscribe, Payload: subPayload})
	expectCloseCode(t, conn, closeUnauthorized)
}

func TestGQLWSSubscribeValidCallsOnSubscribe(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	type subCall struct {
		id      string
		payload gqlwsSubscribePayload
	}
	calls := make(chan subCall, 1)
	h.OnSubscribe = func(ctx context.Context, conn *GQLWSConn, id string, payload gqlwsSubscribePayload) {
		calls <- subCall{id: id, payload: payload}
	}

	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	subPayload, err := json.Marshal(gqlwsSubscribePayload{Query: `subscription { posts { id } }`})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsSubscribe, Payload: subPayload})

	select {
	case call := <-calls:
		testutil.Equal(t, "sub-1", call.id)
		testutil.Equal(t, `subscription { posts { id } }`, call.payload.Query)
	case <-time.After(2 * time.Second):
		t.Fatal("expected OnSubscribe callback")
	}
}

func TestGQLWSDuplicateSubscribeIDCloses(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	h.OnSubscribe = func(ctx context.Context, conn *GQLWSConn, id string, payload gqlwsSubscribePayload) {}

	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	subPayload, err := json.Marshal(gqlwsSubscribePayload{Query: `subscription { posts { id } }`})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsSubscribe, Payload: subPayload})
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsSubscribe, Payload: subPayload})
	expectCloseCode(t, conn, closeSubscriberExists)
}

func TestGQLWSCompleteCallsOnComplete(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	h.OnSubscribe = func(ctx context.Context, conn *GQLWSConn, id string, payload gqlwsSubscribePayload) {}
	completed := make(chan string, 1)
	h.OnComplete = func(conn *GQLWSConn, id string) {
		completed <- id
	}

	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	subPayload, err := json.Marshal(gqlwsSubscribePayload{Query: `subscription { posts { id } }`})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsSubscribe, Payload: subPayload})
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsComplete})

	select {
	case id := <-completed:
		testutil.Equal(t, "sub-1", id)
	case <-time.After(2 * time.Second):
		t.Fatal("expected OnComplete callback")
	}
}

func TestGQLWSPingPong(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	payload := json.RawMessage(`{"x":1}`)
	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsPing, Payload: payload})
	resp := readGQLWSMessage(t, conn)
	testutil.Equal(t, gqlwsPong, resp.Type)
	testutil.Equal(t, string(payload), string(resp.Payload))
}

func TestGQLWSServerKeepalivePing(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	h.PingInterval = 30 * time.Millisecond
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	deadline := time.Now().Add(2 * time.Second)
	for {
		conn.SetReadDeadline(deadline)
		msg := readGQLWSMessage(t, conn)
		if msg.Type == gqlwsPing {
			return
		}
	}
}

func TestGQLWSSubscribeNonSubscriptionQueryReturnsError(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	h.OnSubscribe = func(ctx context.Context, conn *GQLWSConn, id string, payload gqlwsSubscribePayload) {}

	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	subPayload, err := json.Marshal(gqlwsSubscribePayload{Query: `{ posts { id } }`})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsSubscribe, Payload: subPayload})

	msg := readGQLWSMessage(t, conn)
	testutil.Equal(t, gqlwsError, msg.Type)
	testutil.Equal(t, "sub-1", msg.ID)
}

func TestHandlerWebSocketContentNegotiation(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "title", TypeName: "text"},
		},
	}})

	h := &Handler{
		cache:         cache,
		logger:        testutil.DiscardLogger(),
		wsHandler:     NewGQLWSHandler(nil, func() *schema.SchemaCache { return cache }, testutil.DiscardLogger()),
		maxDepth:      0,
		maxComplexity: 0,
	}
	h.wsHandler.InitTimeout = 200 * time.Millisecond

	srv := httptest.NewServer(h)
	defer srv.Close()

	rrBody := []byte(`{"query":"{ posts { id title } }"}`)
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(rrBody))
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.Equal(t, http.StatusOK, resp.StatusCode)

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()
	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)
}

func TestGQLWSRoundTripInitSubscribeRegisters(t *testing.T) {
	t.Parallel()
	h := newTestGQLWSHandler(t)
	registered := make(chan string, 1)
	h.OnSubscribe = func(ctx context.Context, conn *GQLWSConn, id string, payload gqlwsSubscribePayload) {
		registered <- id
	}

	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	subPayload, err := json.Marshal(gqlwsSubscribePayload{Query: `subscription { posts { id } }`})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "s-1", Type: gqlwsSubscribe, Payload: subPayload})

	select {
	case id := <-registered:
		testutil.Equal(t, "s-1", id)
	case <-time.After(2 * time.Second):
		t.Fatal("expected subscription registration callback")
	}
}

func TestHandlerGQLWSHubDeliveryWithWhereFilter(t *testing.T) {
	t.Parallel()

	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", TypeName: "text"},
		},
		PrimaryKey: []string{"id"},
	}})
	cacheHolder := &schema.CacheHolder{}
	cacheHolder.SetForTesting(cache)

	h := NewHandler(nil, cacheHolder, testutil.DiscardLogger())
	h.SetHub(realtime.NewHub(testutil.DiscardLogger()))
	h.wsHandler.InitTimeout = 200 * time.Millisecond

	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	subPayload, err := json.Marshal(gqlwsSubscribePayload{
		Query: `subscription { posts(where: { id: { _eq: 1 } }) { id title } }`,
	})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsSubscribe, Payload: subPayload})
	waitForWSSubRegistered(t, h, "gqlws-1", "sub-1")

	// Non-matching event should be filtered out.
	h.hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "posts",
		Record: map[string]any{"id": 2, "title": "skip"},
	})

	// Matching event should be delivered.
	h.hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "posts",
		Record: map[string]any{"id": 1, "title": "deliver"},
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		conn.SetReadDeadline(deadline)
		msg := readGQLWSMessage(t, conn)
		if msg.Type != gqlwsNext {
			continue
		}
		testutil.Equal(t, "sub-1", msg.ID)
		var payload map[string]map[string]any
		testutil.NoError(t, json.Unmarshal(msg.Payload, &payload))
		row := payload["data"]["posts"].(map[string]any)
		switch id := row["id"].(type) {
		case float64:
			testutil.Equal(t, float64(1), id)
		case int64:
			testutil.Equal(t, int64(1), id)
		case int:
			testutil.Equal(t, 1, id)
		default:
			t.Fatalf("unexpected id type: %T", row["id"])
		}
		testutil.Equal(t, "deliver", row["title"])
		return
	}
}

func TestHandlerGQLWSCompleteStopsHubDelivery(t *testing.T) {
	t.Parallel()

	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", TypeName: "text"},
		},
		PrimaryKey: []string{"id"},
	}})
	cacheHolder := &schema.CacheHolder{}
	cacheHolder.SetForTesting(cache)

	h := NewHandler(nil, cacheHolder, testutil.DiscardLogger())
	h.SetHub(realtime.NewHub(testutil.DiscardLogger()))
	h.wsHandler.InitTimeout = 200 * time.Millisecond

	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	subPayload, err := json.Marshal(gqlwsSubscribePayload{
		Query: `subscription { posts { id title } }`,
	})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsSubscribe, Payload: subPayload})
	waitForWSSubRegistered(t, h, "gqlws-1", "sub-1")

	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsComplete})
	time.Sleep(20 * time.Millisecond)
	h.hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "posts",
		Record: map[string]any{"id": 1, "title": "should-not-deliver"},
	})

	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	var msg gqlwsMessage
	err = conn.ReadJSON(&msg)
	netErr, ok := err.(net.Error)
	testutil.True(t, ok && netErr.Timeout(), "expected read timeout after complete, got: %v", err)
}

func TestHandlerGQLWSHubDeliverySupportsAliasedFields(t *testing.T) {
	t.Parallel()

	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", TypeName: "text"},
		},
		PrimaryKey: []string{"id"},
	}})
	cacheHolder := &schema.CacheHolder{}
	cacheHolder.SetForTesting(cache)

	h := NewHandler(nil, cacheHolder, testutil.DiscardLogger())
	h.SetHub(realtime.NewHub(testutil.DiscardLogger()))
	h.wsHandler.InitTimeout = 200 * time.Millisecond

	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	subPayload, err := json.Marshal(gqlwsSubscribePayload{
		Query: `subscription { posts { postId: id title } }`,
	})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-1", Type: gqlwsSubscribe, Payload: subPayload})
	waitForWSSubRegistered(t, h, "gqlws-1", "sub-1")

	h.hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "posts",
		Record: map[string]any{"id": 11, "title": "aliased"},
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		conn.SetReadDeadline(deadline)
		msg := readGQLWSMessage(t, conn)
		if msg.Type != gqlwsNext {
			continue
		}
		var payload map[string]map[string]any
		testutil.NoError(t, json.Unmarshal(msg.Payload, &payload))
		row := payload["data"]["posts"].(map[string]any)
		_, hasRawID := row["id"]
		testutil.True(t, !hasRawID, "raw field name should not be present when alias is used")
		testutil.Equal(t, float64(11), row["postId"].(float64))
		testutil.Equal(t, "aliased", row["title"])
		return
	}
}

func TestHandlerGQLWSRejectsSubscriptionToSkippedTableKinds(t *testing.T) {
	t.Parallel()

	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "posts",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			},
			PrimaryKey: []string{"id"},
		},
		{
			Schema: "public",
			Name:   "post_view",
			Kind:   "view",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
			},
		},
	})
	cacheHolder := &schema.CacheHolder{}
	cacheHolder.SetForTesting(cache)

	h := NewHandler(nil, cacheHolder, testutil.DiscardLogger())
	h.SetHub(realtime.NewHub(testutil.DiscardLogger()))
	h.wsHandler.InitTimeout = 200 * time.Millisecond

	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, _ := dialGQLWS(t, srv, true)
	defer conn.Close()

	writeGQLWSMessage(t, conn, gqlwsMessage{Type: gqlwsConnectionInit})
	testutil.Equal(t, gqlwsConnectionAck, readGQLWSMessage(t, conn).Type)

	subPayload, err := json.Marshal(gqlwsSubscribePayload{
		Query: `subscription { post_view { id } }`,
	})
	testutil.NoError(t, err)
	writeGQLWSMessage(t, conn, gqlwsMessage{ID: "sub-view", Type: gqlwsSubscribe, Payload: subPayload})

	msg := readGQLWSMessage(t, conn)
	testutil.Equal(t, gqlwsError, msg.Type)
	testutil.Equal(t, "sub-view", msg.ID)
}
