// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/admin_replicas.go.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/replica"
	"github.com/go-chi/chi/v5"
)

// replicaLifecycle is the server-local interface for lifecycle operations.
// Handlers depend on this rather than a concrete *replica.LifecycleService.
type replicaLifecycle interface {
	AddReplica(ctx context.Context, record replica.TopologyNodeRecord) (replica.TopologyNodeRecord, error)
	RemoveReplica(ctx context.Context, name string, force bool) error
	PromoteReplica(ctx context.Context, name string) (replica.TopologyNodeRecord, error)
	InitiateFailover(ctx context.Context, target string, force bool) error
}

// --- Request / response types ---

type addReplicaRequest struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	SSLMode     string `json:"ssl_mode"`
	Weight      int    `json:"weight"`
	MaxLagBytes int64  `json:"max_lag_bytes"`
}

type failoverRequest struct {
	Target string `json:"target"`
	Force  bool   `json:"force"`
}

type replicaConnectionsResponse struct {
	Total int32 `json:"total"`
	Idle  int32 `json:"idle"`
	InUse int32 `json:"in_use"`
}

type replicaStatusResponse struct {
	Name          string                     `json:"name"`
	URL           string                     `json:"url"`
	State         string                     `json:"state"`
	LagBytes      int64                      `json:"lag_bytes"`
	Weight        int                        `json:"weight"`
	Connections   replicaConnectionsResponse `json:"connections"`
	LastCheckedAt string                     `json:"last_checked_at"`
	LastError     *string                    `json:"last_error"`
}

type replicasResponse struct {
	Replicas []replicaStatusResponse `json:"replicas"`
}

type topologyNodeResponse struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	SSLMode     string `json:"ssl_mode"`
	Weight      int    `json:"weight"`
	MaxLagBytes int64  `json:"max_lag_bytes"`
	Role        string `json:"role"`
	State       string `json:"state"`
}

type addReplicaResponse struct {
	Status   string                  `json:"status"`
	Record   topologyNodeResponse    `json:"record"`
	Replicas []replicaStatusResponse `json:"replicas"`
}

type promoteReplicaResponse struct {
	Status   string                  `json:"status"`
	Primary  topologyNodeResponse    `json:"primary"`
	Replicas []replicaStatusResponse `json:"replicas"`
}

// --- Read-only handlers (existing) ---

func (s *Server) handleListReplicas(w http.ResponseWriter, _ *http.Request) {
	if s.healthChecker == nil {
		httputil.WriteJSON(w, http.StatusOK, replicasResponse{Replicas: []replicaStatusResponse{}})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, replicasResponse{
		Replicas: buildReplicaStatusResponses(s.healthChecker.Statuses()),
	})
}

func (s *Server) handleCheckReplicas(w http.ResponseWriter, r *http.Request) {
	if s.healthChecker != nil {
		s.healthChecker.RunCheck(r.Context())
	}
	s.handleListReplicas(w, r)
}

// --- Lifecycle handlers ---

// TODO: Document Server.handleAddReplica.
func (s *Server) handleAddReplica(w http.ResponseWriter, r *http.Request) {
	if s.lifecycleService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "replica lifecycle not available")
		return
	}
	var req addReplicaRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	record := replica.TopologyNodeRecord{
		Name:        req.Name,
		Host:        req.Host,
		Port:        req.Port,
		Database:    req.Database,
		SSLMode:     req.SSLMode,
		Weight:      req.Weight,
		MaxLagBytes: req.MaxLagBytes,
	}
	createdRecord, err := s.lifecycleService.AddReplica(r.Context(), record)
	if err != nil {
		writeLifecycleError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, addReplicaResponse{
		Status:   "added",
		Record:   topologyNodeResponseFromRecord(createdRecord),
		Replicas: s.currentReplicaStatusResponses(),
	})
}

func (s *Server) handleRemoveReplica(w http.ResponseWriter, r *http.Request) {
	if s.lifecycleService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "replica lifecycle not available")
		return
	}
	name := chi.URLParam(r, "name")
	force := r.URL.Query().Get("force") == "true"
	if err := s.lifecycleService.RemoveReplica(r.Context(), name, force); err != nil {
		writeLifecycleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TODO: Document Server.handlePromoteReplica.
func (s *Server) handlePromoteReplica(w http.ResponseWriter, r *http.Request) {
	if s.lifecycleService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "replica lifecycle not available")
		return
	}
	name := chi.URLParam(r, "name")
	primaryRecord, err := s.lifecycleService.PromoteReplica(r.Context(), name)
	if err != nil {
		writeLifecycleError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, promoteReplicaResponse{
		Status:   "promoted",
		Primary:  topologyNodeResponseFromRecord(primaryRecord),
		Replicas: s.currentReplicaStatusResponses(),
	})
}

// TODO: Document Server.handleFailover.
func (s *Server) handleFailover(w http.ResponseWriter, r *http.Request) {
	if s.lifecycleService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "replica lifecycle not available")
		return
	}
	var req failoverRequest
	if !decodeOptionalJSON(w, r, &req) {
		return
	}
	if err := s.lifecycleService.InitiateFailover(r.Context(), req.Target, req.Force); err != nil {
		writeLifecycleError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "failover_complete",
	})
}

// --- Shared error mapping ---

func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}

	r.Body = http.MaxBytesReader(w, r.Body, httputil.MaxBodySize)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// TODO: Document writeLifecycleError.
func writeLifecycleError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case errors.Is(err, replica.ErrTopologyNodeNotFound):
		httputil.WriteError(w, http.StatusNotFound, msg)
	case strings.Contains(msg, "connectivity check failed") ||
		strings.Contains(msg, "dial connectivity pool") ||
		strings.Contains(msg, "dial runtime pool") ||
		strings.Contains(msg, "dial promotion pool"):
		httputil.WriteError(w, http.StatusBadGateway, msg)
	case strings.Contains(msg, "refusing to remove last active replica without force") ||
		strings.Contains(msg, "primary is healthy; pass force=true to override") ||
		strings.Contains(msg, "target replica is not healthy") ||
		strings.Contains(msg, "no healthy replica candidates available"):
		httputil.WriteError(w, http.StatusConflict, msg)
	case strings.Contains(msg, "normalize") ||
		strings.Contains(msg, "host is required") ||
		strings.Contains(msg, "target is not a standby replica") ||
		strings.Contains(msg, "target is not a replica") ||
		strings.Contains(msg, "target is not active") ||
		strings.Contains(msg, "already exists"):
		httputil.WriteError(w, http.StatusBadRequest, msg)
	default:
		httputil.WriteError(w, http.StatusInternalServerError, msg)
	}
}

// --- Response helpers ---

// TODO: Document buildReplicaStatusResponses.
func buildReplicaStatusResponses(statuses []replica.ReplicaStatus) []replicaStatusResponse {
	resp := make([]replicaStatusResponse, 0, len(statuses))
	for _, status := range statuses {
		lastCheckedAt := ""
		if !status.LastCheckedAt.IsZero() {
			lastCheckedAt = status.LastCheckedAt.UTC().Format(time.RFC3339)
		}

		var lastError *string
		if status.LastError != nil {
			errMsg := status.LastError.Error()
			lastError = &errMsg
		}

		connections := replicaConnectionsResponse{}
		if status.Pool != nil {
			stats := status.Pool.Stat()
			connections = replicaConnectionsResponse{
				Total: stats.TotalConns(),
				Idle:  stats.IdleConns(),
				InUse: stats.AcquiredConns(),
			}
		}

		resp = append(resp, replicaStatusResponse{
			Name:          status.Name,
			URL:           replica.SanitizeReplicaURL(status.Config.URL),
			State:         status.State.String(),
			LagBytes:      status.LagBytes,
			Weight:        status.Config.Weight,
			Connections:   connections,
			LastCheckedAt: lastCheckedAt,
			LastError:     lastError,
		})
	}
	return resp
}

func topologyNodeResponseFromRecord(record replica.TopologyNodeRecord) topologyNodeResponse {
	return topologyNodeResponse{
		Name:        record.Name,
		Host:        record.Host,
		Port:        record.Port,
		Database:    record.Database,
		SSLMode:     record.SSLMode,
		Weight:      record.Weight,
		MaxLagBytes: record.MaxLagBytes,
		Role:        record.Role,
		State:       record.State,
	}
}

func (s *Server) currentReplicaStatusResponses() []replicaStatusResponse {
	if s.healthChecker == nil {
		return []replicaStatusResponse{}
	}
	return buildReplicaStatusResponses(s.healthChecker.Statuses())
}
