package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/ws"
)

func dialRealtimeWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/realtime/ws"
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	testutil.NoError(t, err)
	return c
}

func wsWrite(t *testing.T, c *websocket.Conn, msg ws.ClientMessage) {
	t.Helper()
	testutil.NoError(t, c.WriteJSON(msg))
}

func wsRead(t *testing.T, c *websocket.Conn) ws.ServerMessage {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	var m ws.ServerMessage
	testutil.NoError(t, c.ReadJSON(&m))
	return m
}

func assertWSCloseCode(t *testing.T, c *websocket.Conn, expectedCode int) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))

	for i := 0; i < 2; i++ {
		_, payload, err := c.ReadMessage()
		if err != nil {
			closeErr, ok := err.(*websocket.CloseError)
			testutil.True(t, ok, "expected websocket close error, got: %v", err)
			testutil.Equal(t, expectedCode, closeErr.Code)
			return
		}

		// A connected frame may arrive before the close frame under some timing windows.
		var msg ws.ServerMessage
		if json.Unmarshal(payload, &msg) == nil && msg.Type == ws.MsgTypeConnected {
			continue
		}
	}

	t.Fatalf("expected websocket close code %d", expectedCode)
}

func TestAdminRealtimeForceDisconnectRequiresAdminAuth(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "testpass")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/realtime/connections/c1/disconnect", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminRealtimeForceDisconnectUnknownIDReturns404(t *testing.T) {
	t.Parallel()
	app := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, app)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/realtime/connections/nonexistent/disconnect", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	app.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminRealtimeForceDisconnect(t *testing.T) {
	t.Parallel()
	app := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, app)
	httpSrv := httptest.NewServer(app.Router())
	defer httpSrv.Close()

	// Establish a WS connection.
	wsConn := dialRealtimeWS(t, httpSrv)
	defer wsConn.Close()
	connected := wsRead(t, wsConn) // read "connected" message
	testutil.Equal(t, ws.MsgTypeConnected, connected.Type)

	clientID := connected.ClientID
	testutil.True(t, clientID != "", "connected message should carry client ID")

	// Force-disconnect via admin endpoint.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/realtime/connections/"+clientID+"/disconnect", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	app.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNoContent, w.Code)
	assertWSCloseCode(t, wsConn, 1001)

	// A second disconnect of the same ID should be 404.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/admin/realtime/connections/"+clientID+"/disconnect", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	app.Router().ServeHTTP(w2, req2)
	testutil.Equal(t, http.StatusNotFound, w2.Code)
}

func TestRealtimeWSConnectionLimitExceededClosesWith4008(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	cfg.Realtime.MaxConnectionsPerUser = 1
	app := newTestServerWithConfig(t, cfg)
	httpSrv := httptest.NewServer(app.Router())
	defer httpSrv.Close()

	ws1 := dialRealtimeWS(t, httpSrv)
	defer ws1.Close()
	connected := wsRead(t, ws1)
	testutil.Equal(t, ws.MsgTypeConnected, connected.Type)

	ws2 := dialRealtimeWS(t, httpSrv)
	defer ws2.Close()
	assertWSCloseCode(t, ws2, 4008)
}

func TestAdminRealtimeStatsEndpointsRequireAdminAuth(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithPassword(t, "testpass")

	for _, path := range []string{
		"/api/admin/realtime/stats",
		"/api/admin/realtime/connections",
		"/api/admin/realtime/subscriptions",
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusUnauthorized, w.Code)
	}
}

func TestAdminRealtimeStatsEndpointsReturnInspectorPayloads(t *testing.T) {
	t.Parallel()
	app := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, app)
	httpSrv := httptest.NewServer(app.Router())
	defer httpSrv.Close()

	c1 := dialRealtimeWS(t, httpSrv)
	defer c1.Close()
	c2 := dialRealtimeWS(t, httpSrv)
	defer c2.Close()
	_ = wsRead(t, c1) // connected
	_ = wsRead(t, c2) // connected

	wsWrite(t, c1, ws.ClientMessage{Type: ws.MsgTypeSubscribe, Tables: []string{"posts"}, Ref: "s1"})
	testutil.Equal(t, ws.MsgTypeReply, wsRead(t, c1).Type)
	wsWrite(t, c2, ws.ClientMessage{Type: ws.MsgTypeSubscribe, Tables: []string{"posts", "comments"}, Ref: "s2"})
	testutil.Equal(t, ws.MsgTypeReply, wsRead(t, c2).Type)
	wsWrite(t, c1, ws.ClientMessage{Type: ws.MsgTypeChannelSubscribe, Channel: "room1", Ref: "c1"})
	testutil.Equal(t, ws.MsgTypeReply, wsRead(t, c1).Type)
	testutil.Equal(t, ws.MsgTypePresence, wsRead(t, c1).Type)
	wsWrite(t, c2, ws.ClientMessage{Type: ws.MsgTypeChannelSubscribe, Channel: "room1", Ref: "c2"})
	testutil.Equal(t, ws.MsgTypeReply, wsRead(t, c2).Type)
	testutil.Equal(t, ws.MsgTypePresence, wsRead(t, c2).Type)
	wsWrite(t, c1, ws.ClientMessage{Type: ws.MsgTypePresenceTrack, Channel: "room1", Presence: map[string]any{"user": "alice"}, Ref: "p1"})
	testutil.Equal(t, ws.MsgTypePresence, wsRead(t, c1).Type) // diff
	testutil.Equal(t, ws.MsgTypePresence, wsRead(t, c2).Type) // diff
	testutil.Equal(t, ws.MsgTypeReply, wsRead(t, c1).Type)    // reply

	type statsResp struct {
		Version     string `json:"version"`
		Timestamp   string `json:"timestamp"`
		Connections struct {
			SSE   int `json:"sse"`
			WS    int `json:"ws"`
			Total int `json:"total"`
		} `json:"connections"`
		Subscriptions struct {
			Tables   map[string]int `json:"tables"`
			Channels struct {
				Broadcast map[string]int `json:"broadcast"`
				Presence  map[string]int `json:"presence"`
			} `json:"channels"`
		} `json:"subscriptions"`
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/realtime/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	app.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	var stats statsResp
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	testutil.True(t, stats.Version != "", "version should be set")
	testutil.True(t, stats.Timestamp != "", "timestamp should be set")
	testutil.Equal(t, 2, stats.Connections.WS)
	testutil.Equal(t, 2, stats.Subscriptions.Tables["posts"])
	testutil.Equal(t, 1, stats.Subscriptions.Tables["comments"])
	testutil.Equal(t, 2, stats.Subscriptions.Channels.Broadcast["room1"])
	testutil.Equal(t, 1, stats.Subscriptions.Channels.Presence["room1"])

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/admin/realtime/connections", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	app.Router().ServeHTTP(w2, req2)
	testutil.Equal(t, http.StatusOK, w2.Code)
	testutil.Contains(t, w2.Body.String(), `"connections"`)

	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/admin/realtime/subscriptions", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	app.Router().ServeHTTP(w3, req3)
	testutil.Equal(t, http.StatusOK, w3.Code)
	testutil.Contains(t, w3.Body.String(), `"tables"`)
	testutil.Contains(t, w3.Body.String(), `"channels"`)
}
