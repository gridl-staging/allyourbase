package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

type mockRailwayClient struct {
	calls []string

	projectNames           []string
	projectResp            RailwayProject
	projectErr             error
	serviceReqs            []railwayCreateServiceReq
	serviceResp            RailwayService
	serviceErr             error
	setConfigProjectIDs    []string
	setConfigVariablesReqs []map[string]string
	setConfigErr           error
	deployReqs             []railwayDeployReq
	deployResp             RailwayDeployment
	deployErr              error
	waitReqs               []railwayWaitReq
	waitResp               RailwayDeployment
	waitErr                error
	waitFunc               func(ctx context.Context, projectID, deploymentID string) (RailwayDeployment, error)
}

type railwayCreateServiceReq struct {
	ProjectID string
	Name      string
}

type railwayDeployReq struct {
	ProjectID string
	ServiceID string
	Image     string
}

type railwayWaitReq struct {
	ProjectID    string
	DeploymentID string
}

func (m *mockRailwayClient) CreateProject(_ context.Context, name string) (RailwayProject, error) {
	return RailwayProject{}, fmt.Errorf("not used")
}

func (m *mockRailwayClient) GetOrCreateProject(_ context.Context, name string) (RailwayProject, error) {
	m.calls = append(m.calls, "GetOrCreateProject")
	m.projectNames = append(m.projectNames, name)
	if m.projectErr != nil {
		return RailwayProject{}, m.projectErr
	}
	if m.projectResp.ID == "" {
		m.projectResp = RailwayProject{ID: "proj-1", Name: name}
	}
	return m.projectResp, nil
}

func (m *mockRailwayClient) GetOrCreateService(_ context.Context, projectID, name string) (RailwayService, error) {
	m.calls = append(m.calls, "GetOrCreateService")
	m.serviceReqs = append(m.serviceReqs, railwayCreateServiceReq{ProjectID: projectID, Name: name})
	if m.serviceErr != nil {
		return RailwayService{}, m.serviceErr
	}
	if m.serviceResp.ID == "" {
		m.serviceResp = RailwayService{ID: "svc-1", Name: name}
	}
	return m.serviceResp, nil
}

func (m *mockRailwayClient) CreateService(_ context.Context, projectID, name string) (RailwayService, error) {
	return RailwayService{}, fmt.Errorf("not used")
}

func (m *mockRailwayClient) SetConfigVariables(_ context.Context, projectID string, variables map[string]string) error {
	m.calls = append(m.calls, "SetConfigVariables")
	m.setConfigProjectIDs = append(m.setConfigProjectIDs, projectID)
	copied := make(map[string]string, len(variables))
	for k, v := range variables {
		copied[k] = v
	}
	m.setConfigVariablesReqs = append(m.setConfigVariablesReqs, copied)
	return m.setConfigErr
}

func (m *mockRailwayClient) TriggerDeployment(_ context.Context, projectID, serviceID, image string) (RailwayDeployment, error) {
	m.calls = append(m.calls, "TriggerDeployment")
	m.deployReqs = append(m.deployReqs, railwayDeployReq{ProjectID: projectID, ServiceID: serviceID, Image: image})
	if m.deployErr != nil {
		return RailwayDeployment{}, m.deployErr
	}
	if m.deployResp.ID == "" {
		m.deployResp = RailwayDeployment{ID: "dep-1", Status: "QUEUED", URL: "https://proj-1.railway.app"}
	}
	return m.deployResp, nil
}

func (m *mockRailwayClient) WaitForDeploymentSuccess(ctx context.Context, projectID, deploymentID string) (RailwayDeployment, error) {
	m.calls = append(m.calls, "WaitForDeploymentSuccess")
	m.waitReqs = append(m.waitReqs, railwayWaitReq{ProjectID: projectID, DeploymentID: deploymentID})
	if m.waitFunc != nil {
		return m.waitFunc(ctx, projectID, deploymentID)
	}
	if m.waitErr != nil {
		return RailwayDeployment{}, m.waitErr
	}
	if m.waitResp.ID == "" {
		m.waitResp = RailwayDeployment{ID: deploymentID, Status: "SUCCESS", URL: "https://proj-1.railway.app"}
	}
	return m.waitResp, nil
}

func TestRailwayProviderValidate(t *testing.T) {
	provider := railwayProvider{}

	err := provider.Validate(DeployConfig{ProviderOptions: map[string]string{}})
	testutil.ErrorContains(t, err, "--image")

	stderr := captureStderr(t, func() {
		err = provider.Validate(DeployConfig{ProviderOptions: map[string]string{railOptionImage: "ghcr.io/gridlhq/ayb:latest"}})
		testutil.NoError(t, err)
	})
	testutil.Contains(t, stderr, "warning")
	testutil.Contains(t, stderr, "--postgres-url")
}

func TestRailwayProviderDeployHappyPath(t *testing.T) {
	mock := &mockRailwayClient{}
	provider := railwayProvider{client: mock}
	cfg := DeployConfig{
		Provider:    deployProviderRailway,
		PostgresURL: "postgres://db",
		Env:         map[string]string{"APP_ENV": "production"},
		ProviderOptions: map[string]string{
			railOptionProjectName: "my-proj",
			railOptionServiceName: "api",
			railOptionImage:       "ghcr.io/gridlhq/ayb:latest",
		},
	}

	result, err := provider.Deploy(context.Background(), cfg)
	testutil.NoError(t, err)
	if !reflect.DeepEqual([]string{"GetOrCreateProject", "GetOrCreateService", "SetConfigVariables", "TriggerDeployment", "WaitForDeploymentSuccess"}, mock.calls) {
		t.Fatalf("unexpected call order: %v", mock.calls)
	}
	if !reflect.DeepEqual([]string{"my-proj"}, mock.projectNames) {
		t.Fatalf("unexpected project names: %v", mock.projectNames)
	}
	if !reflect.DeepEqual([]railwayCreateServiceReq{{ProjectID: "proj-1", Name: "api"}}, mock.serviceReqs) {
		t.Fatalf("unexpected service reqs: %v", mock.serviceReqs)
	}
	if !reflect.DeepEqual([]railwayDeployReq{{ProjectID: "proj-1", ServiceID: "svc-1", Image: "ghcr.io/gridlhq/ayb:latest"}}, mock.deployReqs) {
		t.Fatalf("unexpected deploy reqs: %v", mock.deployReqs)
	}

	setVars := mock.setConfigVariablesReqs[0]
	testutil.Equal(t, "production", setVars["APP_ENV"])
	testutil.Equal(t, "postgres://db", setVars["AYB_DATABASE_URL"])
	testutil.True(t, setVars["AYB_AUTH_JWT_SECRET"] != "")

	testutil.Equal(t, deployProviderRailway, result.Provider)
	testutil.Equal(t, "https://proj-1.railway.app", result.AppURL)
	testutil.Equal(t, "https://railway.app/project/proj-1", result.DashboardURL)
	testutil.True(t, len(result.NextSteps) > 0)
}

func TestRailwayProviderDeployDefaults(t *testing.T) {
	mock := &mockRailwayClient{}
	provider := railwayProvider{client: mock}
	cfg := DeployConfig{
		Provider: deployProviderRailway,
		Domain:   "api.example.com",
		Env:      map[string]string{"AYB_DATABASE_URL": "postgres://db"},
		ProviderOptions: map[string]string{
			railOptionImage: "ghcr.io/gridlhq/ayb:latest",
		},
	}

	_, err := provider.Deploy(context.Background(), cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, "ayb", mock.serviceReqs[0].Name)
	testutil.True(t, mock.projectNames[0] != "")
}

func TestRailwayProviderDeployErrorPropagation(t *testing.T) {
	mock := &mockRailwayClient{deployErr: fmt.Errorf("GraphQL error: invalid image reference")}
	provider := railwayProvider{client: mock}
	cfg := DeployConfig{
		Provider: deployProviderRailway,
		Env:      map[string]string{"AYB_DATABASE_URL": "postgres://db"},
		ProviderOptions: map[string]string{
			railOptionImage: "bad-image",
		},
	}

	_, err := provider.Deploy(context.Background(), cfg)
	testutil.ErrorContains(t, err, "triggering deployment")
	testutil.ErrorContains(t, err, "invalid image reference")
}

func TestRailwayProviderDeployTimeout(t *testing.T) {
	mock := &mockRailwayClient{
		waitFunc: func(ctx context.Context, projectID, deploymentID string) (RailwayDeployment, error) {
			<-ctx.Done()
			return RailwayDeployment{}, ctx.Err()
		},
	}
	provider := railwayProvider{client: mock, waitTimeout: 15 * time.Millisecond}
	cfg := DeployConfig{
		Provider: deployProviderRailway,
		Env:      map[string]string{"AYB_DATABASE_URL": "postgres://db"},
		ProviderOptions: map[string]string{
			railOptionImage: "ghcr.io/gridlhq/ayb:latest",
		},
	}

	_, err := provider.Deploy(context.Background(), cfg)
	testutil.ErrorContains(t, err, "waiting for deployment success")
	testutil.ErrorContains(t, err, "context deadline exceeded")
}

func TestMergeDeployEnv(t *testing.T) {
	cfg := DeployConfig{
		Env:         map[string]string{"EXISTING_VAR": "hello", "PORT": "3000"},
		PostgresURL: "postgresql://user:pass@host:5432/db",
	}

	merged, err := mergeDeployEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, "postgresql://user:pass@host:5432/db", merged["AYB_DATABASE_URL"])
	testutil.Equal(t, "hello", merged["EXISTING_VAR"])
	testutil.Equal(t, "3000", merged["PORT"])
	testutil.True(t, merged["AYB_AUTH_JWT_SECRET"] != "")
}

func TestMergeDeployEnvExistingJWTSecret(t *testing.T) {
	cfg := DeployConfig{Env: map[string]string{"AYB_AUTH_JWT_SECRET": "provided-secret-123"}}
	merged, err := mergeDeployEnv(cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, "provided-secret-123", merged["AYB_AUTH_JWT_SECRET"])
}

func TestRailwayMockCallOrderDeterministic(t *testing.T) {
	mock := &mockRailwayClient{}
	provider := railwayProvider{client: mock}
	cfg := DeployConfig{
		Provider: deployProviderRailway,
		Env:      map[string]string{"AYB_DATABASE_URL": "postgres://db"},
		ProviderOptions: map[string]string{
			railOptionImage: "ghcr.io/gridlhq/ayb:latest",
		},
	}
	_, err := provider.Deploy(context.Background(), cfg)
	testutil.NoError(t, err)

	want := []string{"GetOrCreateProject", "GetOrCreateService", "SetConfigVariables", "TriggerDeployment", "WaitForDeploymentSuccess"}
	if !reflect.DeepEqual(want, mock.calls) {
		t.Fatalf("unexpected call order: got %v want %v", mock.calls, want)
	}
}

func TestRailwayHTTPClientGetOrCreateServiceUsesExisting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var op railwayGraphQLOperation
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&op))
		testutil.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		if strings.Contains(op.Query, "services") {
			_, _ = w.Write([]byte(`{"data":{"project":{"services":{"nodes":[{"id":"svc-existing","name":"ayb"}]}}}}`))
			return
		}
		t.Fatalf("unexpected query: %s", op.Query)
	}))
	defer server.Close()

	client := newRailwayHTTPClient("test-token")
	client.baseURL = server.URL
	client.client = server.Client()

	svc, err := client.GetOrCreateService(context.Background(), "proj-1", "ayb")
	testutil.NoError(t, err)
	testutil.Equal(t, "svc-existing", svc.ID)
	testutil.Equal(t, "ayb", svc.Name)
}

func TestRailwayHTTPClientGetOrCreateServiceCreatesWhenMissing(t *testing.T) {
	var callCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		callCount++

		var op railwayGraphQLOperation
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&op))

		if strings.Contains(op.Query, "services") {
			_, _ = w.Write([]byte(`{"data":{"project":{"services":{"nodes":[]}}}}`))
			return
		}
		if strings.Contains(op.Query, "serviceCreate") {
			_, _ = w.Write([]byte(`{"data":{"serviceCreate":{"id":"svc-created","name":"ayb"}}}`))
			return
		}
		t.Fatalf("unexpected query: %s", op.Query)
	}))
	defer server.Close()

	client := newRailwayHTTPClient("test-token")
	client.baseURL = server.URL
	client.client = server.Client()

	svc, err := client.GetOrCreateService(context.Background(), "proj-1", "ayb")
	testutil.NoError(t, err)
	testutil.Equal(t, "svc-created", svc.ID)
	testutil.Equal(t, "ayb", svc.Name)
	testutil.Equal(t, 2, callCount)
}
