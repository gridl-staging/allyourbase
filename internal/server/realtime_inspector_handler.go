package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/realtime"
)

type realtimeConnectionsResponse struct {
	Version     string                       `json:"version"`
	Timestamp   string                       `json:"timestamp"`
	Connections realtime.ConnectionsSnapshot `json:"connections"`
}

type realtimeSubscriptionsResponse struct {
	Version       string                        `json:"version"`
	Timestamp     string                        `json:"timestamp"`
	Subscriptions realtime.SubscriptionSnapshot `json:"subscriptions"`
}

func (s *Server) realtimeSnapshot() realtime.Snapshot {
	if s.realtimeInspector == nil {
		return realtime.NewInspector(nil, nil).Snapshot()
	}
	return s.realtimeInspector.Snapshot()
}

func (s *Server) handleAdminRealtimeStats(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, s.realtimeSnapshot())
}

func (s *Server) handleAdminRealtimeConnections(w http.ResponseWriter, _ *http.Request) {
	snapshot := s.realtimeSnapshot()
	resp := realtimeConnectionsResponse{
		Version:     snapshot.Version,
		Timestamp:   snapshot.Timestamp.Format(time.RFC3339Nano),
		Connections: snapshot.Connections,
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminRealtimeSubscriptions(w http.ResponseWriter, _ *http.Request) {
	snapshot := s.realtimeSnapshot()
	resp := realtimeSubscriptionsResponse{
		Version:       snapshot.Version,
		Timestamp:     snapshot.Timestamp.Format(time.RFC3339Nano),
		Subscriptions: snapshot.Subscriptions,
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// handleAdminRealtimeForceDisconnect handles POST /admin/realtime/connections/{id}/disconnect.
// It looks up the connection in the ConnectionManager, calls its CloseFunc, deregisters it,
// and returns 204. Returns 404 if the connection ID is not found.
func (s *Server) handleAdminRealtimeForceDisconnect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.connManager == nil || !s.connManager.ForceDisconnect(id) {
		httputil.WriteError(w, http.StatusNotFound, "connection not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
