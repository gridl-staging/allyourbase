package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/replica"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

// fakeLifecycleService implements replicaLifecycle for handler tests.
type fakeLifecycleService struct {
	addErr        error
	removeErr     error
	promoteErr    error
	failoverErr   error
	addResult     replica.TopologyNodeRecord
	promoteResult replica.TopologyNodeRecord

	addCalled      bool
	removeCalled   bool
	promoteCalled  bool
	failoverCalled bool

	lastAddRecord     replica.TopologyNodeRecord
	lastRemoveName    string
	lastRemoveForce   bool
	lastPromoteName   string
	lastFailoverName  string
	lastFailoverForce bool
}

func (f *fakeLifecycleService) AddReplica(_ context.Context, record replica.TopologyNodeRecord) (replica.TopologyNodeRecord, error) {
	f.addCalled = true
	f.lastAddRecord = record
	if f.addErr != nil {
		return replica.TopologyNodeRecord{}, f.addErr
	}
	if f.addResult.Name == "" {
		return record, nil
	}
	return f.addResult, nil
}

func (f *fakeLifecycleService) RemoveReplica(_ context.Context, name string, force bool) error {
	f.removeCalled = true
	f.lastRemoveName = name
	f.lastRemoveForce = force
	return f.removeErr
}

func (f *fakeLifecycleService) PromoteReplica(_ context.Context, name string) (replica.TopologyNodeRecord, error) {
	f.promoteCalled = true
	f.lastPromoteName = name
	if f.promoteErr != nil {
		return replica.TopologyNodeRecord{}, f.promoteErr
	}
	return f.promoteResult, nil
}

func (f *fakeLifecycleService) InitiateFailover(_ context.Context, target string, force bool) error {
	f.failoverCalled = true
	f.lastFailoverName = target
	f.lastFailoverForce = force
	return f.failoverErr
}

// --- Existing tests ---

func TestAdminReplicasRouteRequiresAuthAndReturnsEmptyWhenDisabled(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "replica-admin"
	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	s := newServer(cfg, logger, ch, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/replicas", nil)
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/admin/replicas", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminAuth.token())
	w = httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var body replicasResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.SliceLen(t, body.Replicas, 0)
}

func TestAdminReplicasCheckRunsOnDemandAndReturnsStatus(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "replica-admin"
	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	s := newServer(cfg, logger, ch, nil, nil, nil, nil)

	primary := newReplicaTestPool(t)
	t.Cleanup(primary.Close)
	replicaPool := newReplicaTestPool(t)
	t.Cleanup(replicaPool.Close)

	router := replica.NewPoolRouter(primary, []replica.ReplicaPool{
		{
			Name: "replica-1",
			Pool: replicaPool,
			Config: config.ReplicaConfig{
				URL:         "postgres://replica-1.local:5432/appdb?application_name=replica-1",
				Weight:      3,
				MaxLagBytes: 1024,
			},
		},
	}, logger)
	s.poolRouter = router
	s.healthChecker = replica.NewHealthChecker(router, time.Second, logger)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/replicas/check", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminAuth.token())
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var body replicasResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.SliceLen(t, body.Replicas, 1)

	entry := body.Replicas[0]
	testutil.Equal(t, "replica-1", entry.Name)
	testutil.Equal(t, "postgres://replica-1.local:5432/appdb?application_name=replica-1", entry.URL)
	testutil.Equal(t, "suspect", entry.State)
	testutil.Equal(t, int64(0), entry.LagBytes)
	testutil.Equal(t, 3, entry.Weight)
	testutil.True(t, entry.LastCheckedAt != "")
	testutil.NotNil(t, entry.LastError)
}

func TestBuildReplicaStatusResponsesSanitizesSensitiveReplicaURLData(t *testing.T) {
	t.Parallel()

	statuses := []replica.ReplicaStatus{
		{
			Name: "replica-1",
			Config: replica.ReplicaConfig{
				URL: "postgres://reader:secret@replica-1.local:5432/appdb?application_name=replica-1&password=leak&sslpassword=leak2&user=leaky",
			},
		},
	}

	resp := buildReplicaStatusResponses(statuses)
	testutil.SliceLen(t, resp, 1)
	testutil.Equal(t, "replica-1", resp[0].Name)
	testutil.Equal(t, "postgres://replica-1.local:5432/appdb?application_name=replica-1", resp[0].URL)
}

// --- Lifecycle handler tests ---

func newTestServerWithLifecycle(t *testing.T, lifecycle replicaLifecycle) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.Admin.Password = "test-admin"
	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	s := newServer(cfg, logger, ch, nil, nil, nil, nil)
	s.lifecycleService = lifecycle
	return s
}

func adminReq(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		testutil.NoError(t, err)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Authorization", "Bearer "+s.adminAuth.token())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	return w
}

func TestHandleAddReplicaSuccess(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{
		addResult: replica.TopologyNodeRecord{
			Name:        "replica-1",
			Host:        "replica-1.local",
			Port:        5432,
			Database:    "appdb",
			SSLMode:     "disable",
			Weight:      2,
			MaxLagBytes: 2048,
			Role:        replica.TopologyRoleReplica,
			State:       replica.TopologyStateActive,
		},
	}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas", addReplicaRequest{
		Name:        "replica-1",
		Host:        "replica-1.local",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Weight:      2,
		MaxLagBytes: 2048,
	})
	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.True(t, fake.addCalled)
	testutil.Equal(t, "replica-1", fake.lastAddRecord.Name)
	testutil.Equal(t, "replica-1.local", fake.lastAddRecord.Host)
	testutil.Equal(t, 5432, fake.lastAddRecord.Port)
	testutil.Equal(t, "appdb", fake.lastAddRecord.Database)
	testutil.Equal(t, 2, fake.lastAddRecord.Weight)
	testutil.Equal(t, int64(2048), fake.lastAddRecord.MaxLagBytes)

	var body addReplicaResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.Equal(t, "added", body.Status)
	testutil.Equal(t, "replica-1", body.Record.Name)
	testutil.Equal(t, replica.TopologyRoleReplica, body.Record.Role)
	testutil.Equal(t, replica.TopologyStateActive, body.Record.State)
}

func TestHandleAddReplicaValidationError(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{addErr: errors.New(`normalize replica topology record "bad": host is required`)}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas", addReplicaRequest{
		Name: "bad",
	})
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAddReplicaConnectivityError(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{addErr: errors.New(`add replica "r1": connectivity check failed: connection refused`)}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas", addReplicaRequest{
		Name: "r1",
		Host: "unreachable.local",
	})
	testutil.Equal(t, http.StatusBadGateway, w.Code)
}

func TestHandleAddReplicaRejectsPrimaryTarget(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{addErr: errors.New(`add replica "r1": target is not a standby replica`)}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas", addReplicaRequest{
		Name: "r1",
		Host: "primary.local",
	})
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleRemoveReplicaSuccess(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodDelete, "/api/admin/replicas/my-replica", nil)
	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.True(t, fake.removeCalled)
	testutil.Equal(t, "my-replica", fake.lastRemoveName)
	testutil.False(t, fake.lastRemoveForce)
}

func TestHandleRemoveReplicaWithForce(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodDelete, "/api/admin/replicas/my-replica?force=true", nil)
	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.True(t, fake.lastRemoveForce)
}

func TestHandleRemoveReplicaNotFound(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{removeErr: replica.ErrTopologyNodeNotFound}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodDelete, "/api/admin/replicas/missing", nil)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleRemoveReplicaLastActiveConflict(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{removeErr: errors.New(`remove replica "r1": refusing to remove last active replica without force`)}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodDelete, "/api/admin/replicas/r1", nil)
	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandlePromoteReplicaSuccess(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{
		promoteResult: replica.TopologyNodeRecord{
			Name:     "my-replica",
			Host:     "my-replica.local",
			Port:     5432,
			Database: "appdb",
			SSLMode:  "disable",
			Role:     replica.TopologyRolePrimary,
			State:    replica.TopologyStateActive,
		},
	}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas/my-replica/promote", nil)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.True(t, fake.promoteCalled)
	testutil.Equal(t, "my-replica", fake.lastPromoteName)

	var body promoteReplicaResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.Equal(t, "promoted", body.Status)
	testutil.Equal(t, "my-replica", body.Primary.Name)
	testutil.Equal(t, replica.TopologyRolePrimary, body.Primary.Role)
}

func TestHandlePromoteReplicaNotFound(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{promoteErr: replica.ErrTopologyNodeNotFound}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas/missing/promote", nil)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleFailoverSuccess(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas/failover", failoverRequest{
		Target: "replica-2",
		Force:  true,
	})
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.True(t, fake.failoverCalled)
	testutil.Equal(t, "replica-2", fake.lastFailoverName)
	testutil.True(t, fake.lastFailoverForce)
}

func TestHandleFailoverSucceedsWithEmptyBody(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas/failover", nil)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.True(t, fake.failoverCalled)
	testutil.Equal(t, "", fake.lastFailoverName)
	testutil.False(t, fake.lastFailoverForce)
}

func TestHandleFailoverPrimaryHealthyConflict(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{failoverErr: errors.New("initiate failover: primary is healthy; pass force=true to override")}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas/failover", failoverRequest{})
	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandlePromoteReplicaUnhealthyTargetConflict(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{promoteErr: errors.New(`promote replica "r1": target replica is not healthy`)}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas/r1/promote", nil)
	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestHandleFailoverNoHealthyCandidateConflict(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{failoverErr: errors.New("initiate failover: no healthy replica candidates available")}
	s := newTestServerWithLifecycle(t, fake)

	w := adminReq(t, s, http.MethodPost, "/api/admin/replicas/failover", nil)
	testutil.Equal(t, http.StatusConflict, w.Code)
}

func TestLifecycleHandlersReturnServiceUnavailableWhenNil(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Password = "test-admin"
	logger := testReplicaLogger()
	ch := schema.NewCacheHolder(nil, logger)
	s := newServer(cfg, logger, ch, nil, nil, nil, nil)
	// lifecycleService is nil by default

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/replicas"},
		{http.MethodDelete, "/api/admin/replicas/some-replica"},
		{http.MethodPost, "/api/admin/replicas/some-replica/promote"},
		{http.MethodPost, "/api/admin/replicas/failover"},
	}

	for _, ep := range endpoints {
		var reqBody *bytes.Buffer
		if ep.method == http.MethodPost {
			reqBody = bytes.NewBufferString("{}")
		} else {
			reqBody = &bytes.Buffer{}
		}
		req := httptest.NewRequest(ep.method, ep.path, reqBody)
		req.Header.Set("Authorization", "Bearer "+s.adminAuth.token())
		if ep.method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	}
}

func TestLifecycleRoutesRequireAdminAuth(t *testing.T) {
	t.Parallel()
	fake := &fakeLifecycleService{}
	s := newTestServerWithLifecycle(t, fake)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/replicas"},
		{http.MethodDelete, "/api/admin/replicas/some-replica"},
		{http.MethodPost, "/api/admin/replicas/some-replica/promote"},
		{http.MethodPost, "/api/admin/replicas/failover"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		// No Authorization header
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusUnauthorized, w.Code)
	}
}
