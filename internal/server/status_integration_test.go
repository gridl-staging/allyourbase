//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/server"
	statuspkg "github.com/allyourbase/ayb/internal/status"
	"github.com/allyourbase/ayb/internal/testutil"
)

type statusAPIResponse struct {
	Status    statuspkg.ServiceStatus `json:"status"`
	Services  []statuspkg.ProbeResult `json:"services"`
	Incidents []statuspkg.Incident    `json:"incidents"`
	CheckedAt time.Time               `json:"checkedAt"`
}

func statusAdminLogin(t *testing.T, ts *httptest.Server, password string) string {
	t.Helper()
	resp, err := http.Post(ts.URL+"/api/admin/auth", "application/json", strings.NewReader(`{"password":"`+password+`"}`))
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var out map[string]string
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	token := out["token"]
	if token == "" {
		t.Fatal("admin auth returned empty token")
	}
	return token
}

func TestStatusIntegration_IncidentLifecycleReflectedInPublicStatus(t *testing.T) {
	ctx := context.Background()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	ensureIntegrationMigrations(t, ctx)

	cfg := config.Default()
	cfg.Status.Enabled = true
	cfg.Status.PublicEndpointEnabled = true
	cfg.Admin.Password = "status-admin-pass"

	srv := server.New(cfg, testutil.DiscardLogger(), nil, sharedPG.Pool, nil, nil)
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)

	adminToken := statusAdminLogin(t, ts, cfg.Admin.Password)

	createReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/incidents", strings.NewReader(`{"title":"DB outage","status":"investigating","affected_services":["database"]}`))
	testutil.NoError(t, err)
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	testutil.NoError(t, err)
	defer createResp.Body.Close()
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)

	var created statuspkg.Incident
	testutil.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	testutil.True(t, created.ID != "")

	publicResp, err := http.Get(ts.URL + "/api/status")
	testutil.NoError(t, err)
	defer publicResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, publicResp.StatusCode)

	var public statusAPIResponse
	testutil.NoError(t, json.NewDecoder(publicResp.Body).Decode(&public))
	testutil.Equal(t, 1, len(public.Incidents))
	testutil.Equal(t, created.ID, public.Incidents[0].ID)
	testutil.Equal(t, statuspkg.IncidentInvestigating, public.Incidents[0].Status)

	progressReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/incidents/"+created.ID+"/updates", strings.NewReader(`{"message":"Mitigation deployed","status":"monitoring"}`))
	testutil.NoError(t, err)
	progressReq.Header.Set("Authorization", "Bearer "+adminToken)
	progressReq.Header.Set("Content-Type", "application/json")
	progressResp, err := http.DefaultClient.Do(progressReq)
	testutil.NoError(t, err)
	defer progressResp.Body.Close()
	testutil.StatusCode(t, http.StatusCreated, progressResp.StatusCode)

	publicResp, err = http.Get(ts.URL + "/api/status")
	testutil.NoError(t, err)
	defer publicResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, publicResp.StatusCode)
	testutil.NoError(t, json.NewDecoder(publicResp.Body).Decode(&public))
	testutil.Equal(t, 1, len(public.Incidents))
	testutil.Equal(t, statuspkg.IncidentMonitoring, public.Incidents[0].Status)

	resolveReq, err := http.NewRequest(http.MethodPut, ts.URL+"/api/admin/incidents/"+created.ID, strings.NewReader(`{"status":"resolved"}`))
	testutil.NoError(t, err)
	resolveReq.Header.Set("Authorization", "Bearer "+adminToken)
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveResp, err := http.DefaultClient.Do(resolveReq)
	testutil.NoError(t, err)
	defer resolveResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resolveResp.StatusCode)

	publicResp, err = http.Get(ts.URL + "/api/status")
	testutil.NoError(t, err)
	defer publicResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, publicResp.StatusCode)
	testutil.NoError(t, json.NewDecoder(publicResp.Body).Decode(&public))
	testutil.Equal(t, 0, len(public.Incidents))
}

func TestStatusIntegration_ProbeOrchestrationWithDatabaseProbe(t *testing.T) {
	ctx := context.Background()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	ensureIntegrationMigrations(t, ctx)

	cfg := config.Default()
	cfg.Status.Enabled = true
	cfg.Status.PublicEndpointEnabled = true
	cfg.Status.CheckIntervalSeconds = 1
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = findFreePort(t)

	srv := server.New(cfg, testutil.DiscardLogger(), nil, sharedPG.Pool, nil, nil)
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StartWithReady(ready)
	}()
	<-ready

	t.Cleanup(func() {
		shutdownErr := srv.Shutdown(context.Background())
		testutil.NoError(t, shutdownErr)
		select {
		case runErr := <-errCh:
			if runErr != nil && !errors.Is(runErr, http.ErrServerClosed) {
				t.Fatalf("server run error: %v", runErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for server goroutine to exit")
		}
	})

	baseURL := "http://" + net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	var got statusAPIResponse
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, reqErr := http.Get(baseURL + "/api/status")
		if reqErr == nil {
			func() {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					_ = json.NewDecoder(resp.Body).Decode(&got)
				}
			}()
		}
		if got.Status == statuspkg.Operational && len(got.Services) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status checker did not publish expected snapshot in time; got status=%q services=%d", got.Status, len(got.Services))
		}
		time.Sleep(50 * time.Millisecond)
	}

	foundDB := false
	for _, svc := range got.Services {
		if svc.Service == statuspkg.Database {
			foundDB = true
			testutil.True(t, svc.Healthy, "database probe should be healthy")
		}
	}
	testutil.True(t, foundDB, "database service probe must be present")
}

func findFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	testutil.NoError(t, err)
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr type %T", ln.Addr())
	}
	if addr.Port <= 0 {
		t.Fatalf("invalid free port %d", addr.Port)
	}
	return addr.Port
}
