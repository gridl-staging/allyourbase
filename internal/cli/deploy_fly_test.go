package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

type mockFlyClient struct {
	calls []string

	createAppReqs []FlyCreateAppRequest
	createAppResp FlyApp
	createAppErr  error

	getAppNames []string
	getAppResp  FlyApp
	getAppErr   error

	createVolumeAppNames []string
	createVolumeReqs     []FlyCreateVolumeRequest
	createVolumeResp     FlyVolume
	createVolumeErr      error

	setSecretsAppNames []string
	setSecretsReqs     []FlySetSecretsRequest
	setSecretsErr      error

	createMachineAppNames []string
	createMachineReqs     []FlyCreateMachineRequest
	createMachineResp     FlyMachine
	createMachineErr      error

	waitReqs []flyWaitRequest
	waitErr  error
}

type flyWaitRequest struct {
	AppName   string
	MachineID string
	State     string
	Timeout   time.Duration
}

func (m *mockFlyClient) CreateApp(_ context.Context, req FlyCreateAppRequest) (FlyApp, error) {
	m.calls = append(m.calls, "CreateApp")
	m.createAppReqs = append(m.createAppReqs, req)
	if m.createAppErr != nil {
		return FlyApp{}, m.createAppErr
	}
	if m.createAppResp.Name == "" {
		m.createAppResp.Name = req.Name
	}
	return m.createAppResp, nil
}

func (m *mockFlyClient) GetApp(_ context.Context, appName string) (FlyApp, error) {
	m.calls = append(m.calls, "GetApp")
	m.getAppNames = append(m.getAppNames, appName)
	if m.getAppErr != nil {
		return FlyApp{}, m.getAppErr
	}
	if m.getAppResp.Name == "" {
		m.getAppResp.Name = appName
	}
	return m.getAppResp, nil
}

func (m *mockFlyClient) CreateVolume(_ context.Context, appName string, req FlyCreateVolumeRequest) (FlyVolume, error) {
	m.calls = append(m.calls, "CreateVolume")
	m.createVolumeAppNames = append(m.createVolumeAppNames, appName)
	m.createVolumeReqs = append(m.createVolumeReqs, req)
	if m.createVolumeErr != nil {
		return FlyVolume{}, m.createVolumeErr
	}
	return m.createVolumeResp, nil
}

func (m *mockFlyClient) SetSecrets(_ context.Context, appName string, req FlySetSecretsRequest) error {
	m.calls = append(m.calls, "SetSecrets")
	m.setSecretsAppNames = append(m.setSecretsAppNames, appName)
	m.setSecretsReqs = append(m.setSecretsReqs, req)
	return m.setSecretsErr
}

func (m *mockFlyClient) CreateMachine(_ context.Context, appName string, req FlyCreateMachineRequest) (FlyMachine, error) {
	m.calls = append(m.calls, "CreateMachine")
	m.createMachineAppNames = append(m.createMachineAppNames, appName)
	m.createMachineReqs = append(m.createMachineReqs, req)
	if m.createMachineErr != nil {
		return FlyMachine{}, m.createMachineErr
	}
	if m.createMachineResp.ID == "" {
		m.createMachineResp.ID = "machine-1"
	}
	return m.createMachineResp, nil
}

func (m *mockFlyClient) WaitForMachineState(_ context.Context, appName, machineID, state string, timeout time.Duration) error {
	m.calls = append(m.calls, "WaitForMachineState")
	m.waitReqs = append(m.waitReqs, flyWaitRequest{AppName: appName, MachineID: machineID, State: state, Timeout: timeout})
	return m.waitErr
}

func TestGenerateFlyToml(t *testing.T) {
	got := generateFlyToml("my-ayb-app", "iad")
	testutil.True(t, strings.Contains(got, "app = \"my-ayb-app\""))
	testutil.True(t, strings.Contains(got, "primary_region = \"iad\""))
	testutil.True(t, strings.Contains(got, "internal_port = 8090"))
	testutil.True(t, strings.Contains(got, "force_https = true"))
	testutil.True(t, strings.Contains(got, "path = \"/health\""))
	testutil.True(t, strings.Contains(got, "destination = \"/data\""))
}

func TestGenerateDockerfile(t *testing.T) {
	got := generateDockerfile()
	testutil.True(t, strings.Contains(got, "FROM alpine:3.20"))
	testutil.True(t, strings.Contains(got, "RUN apk add --no-cache ca-certificates tzdata"))
	testutil.True(t, strings.Contains(got, "COPY ayb /usr/local/bin/ayb"))
	testutil.True(t, strings.Contains(got, "EXPOSE 8090"))
	testutil.True(t, strings.Contains(got, "ENTRYPOINT [\"ayb\"]"))
	testutil.True(t, strings.Contains(got, "CMD [\"start\"]"))
}

func TestFlyProviderDeployHappyPath(t *testing.T) {
	mockClient := &mockFlyClient{}
	provider := flyProvider{
		client:        mockClient,
		jwtSecretFunc: func() (string, error) { return "generated-jwt-secret", nil },
	}

	cfg := DeployConfig{
		Provider:    deployProviderFly,
		Domain:      "api.example.com",
		Region:      "ord",
		Env:         map[string]string{"APP_ENV": "production"},
		PostgresURL: "postgres://db.example.com:5432/app",
		ProviderOptions: map[string]string{
			flyOptionAppName:    "ayb-test-app",
			flyOptionImage:      "ghcr.io/gridlhq/ayb:latest",
			flyOptionVMSize:     "shared-cpu-1x",
			flyOptionVolumeSize: "2",
		},
	}

	testutil.NoError(t, provider.Validate(cfg))
	result, err := provider.Deploy(context.Background(), cfg)
	testutil.NoError(t, err)

	if !reflect.DeepEqual([]string{"CreateApp", "CreateVolume", "SetSecrets", "CreateMachine", "WaitForMachineState"}, mockClient.calls) {
		t.Fatalf("unexpected call sequence: %v", mockClient.calls)
	}

	testutil.Equal(t, "ord", mockClient.createVolumeReqs[0].Region)
	testutil.Equal(t, 2, mockClient.createVolumeReqs[0].SizeGB)
	testutil.Equal(t, "/data", mockClient.createMachineReqs[0].Config.Mounts[0].Path)
	testutil.Equal(t, "ghcr.io/gridlhq/ayb:latest", mockClient.createMachineReqs[0].Config.Image)
	testutil.Equal(t, "shared-cpu-1x", mockClient.createMachineReqs[0].Config.Guest.Size)
	testutil.Equal(t, "/health", mockClient.createMachineReqs[0].Config.Checks["http"].HTTP.Path)
	testutil.Equal(t, 8090, mockClient.createMachineReqs[0].Config.Checks["http"].HTTP.Port)

	secrets := mockClient.setSecretsReqs[0].Secrets
	testutil.Equal(t, "production", secrets["APP_ENV"])
	testutil.Equal(t, "postgres://db.example.com:5432/app", secrets["AYB_DATABASE_URL"])
	testutil.Equal(t, "generated-jwt-secret", secrets["AYB_AUTH_JWT_SECRET"])

	testutil.Equal(t, deployProviderFly, result.Provider)
	testutil.Equal(t, "https://ayb-test-app.fly.dev", result.AppURL)
	testutil.Equal(t, "https://fly.io/apps/ayb-test-app", result.DashboardURL)
	testutil.True(t, len(result.NextSteps) >= 3)
	testutil.True(t, strings.Contains(fmt.Sprint(result.Metadata["fly_toml"]), "app = \"ayb-test-app\""))
}

func TestFlyProviderDeployAppAlreadyExists(t *testing.T) {
	mockClient := &mockFlyClient{
		createAppErr: &DeployAPIError{Provider: "fly", StatusCode: http.StatusConflict, Message: "already exists", Body: `{"error":"already exists"}`},
		getAppResp:   FlyApp{Name: "existing-app"},
	}
	provider := flyProvider{
		client:        mockClient,
		jwtSecretFunc: func() (string, error) { return "generated-jwt-secret", nil },
	}

	cfg := DeployConfig{
		Provider: deployProviderFly,
		Env:      map[string]string{"AYB_DATABASE_URL": "postgres://db"},
		ProviderOptions: map[string]string{
			flyOptionAppName: "existing-app",
			flyOptionImage:   "ghcr.io/gridlhq/ayb:latest",
		},
	}
	result, err := provider.Deploy(context.Background(), cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, "existing-app", mockClient.getAppNames[0])
	if !reflect.DeepEqual([]string{"CreateApp", "GetApp", "CreateVolume", "SetSecrets", "CreateMachine", "WaitForMachineState"}, mockClient.calls) {
		t.Fatalf("unexpected call sequence: %v", mockClient.calls)
	}
	testutil.Equal(t, "https://existing-app.fly.dev", result.AppURL)
}

func TestFlyProviderDeployAPIErrorPropagation(t *testing.T) {
	mockClient := &mockFlyClient{
		createMachineErr: &DeployAPIError{Provider: "fly", StatusCode: http.StatusUnprocessableEntity, Message: "invalid image reference", Body: `{"error":"invalid image reference"}`},
	}
	provider := flyProvider{
		client:        mockClient,
		jwtSecretFunc: func() (string, error) { return "generated-jwt-secret", nil },
	}

	cfg := DeployConfig{
		Provider: deployProviderFly,
		Env:      map[string]string{"AYB_DATABASE_URL": "postgres://db"},
		ProviderOptions: map[string]string{
			flyOptionAppName: "failing-app",
			flyOptionImage:   "bad-image",
		},
	}
	_, err := provider.Deploy(context.Background(), cfg)
	testutil.ErrorContains(t, err, "422")
	testutil.ErrorContains(t, err, "invalid image reference")
}

func TestFlyProviderValidate(t *testing.T) {
	provider := flyProvider{}
	err := provider.Validate(DeployConfig{Provider: deployProviderFly, Env: map[string]string{}, ProviderOptions: map[string]string{}})
	testutil.ErrorContains(t, err, "--image")
	testutil.ErrorContains(t, err, "Build and push")

	provider = flyProvider{}
	stderr := captureStderr(t, func() {
		err = provider.Validate(DeployConfig{
			Provider:        deployProviderFly,
			Env:             map[string]string{},
			ProviderOptions: map[string]string{flyOptionImage: "ghcr.io/gridlhq/ayb:latest"},
		})
		testutil.NoError(t, err)
	})
	testutil.Contains(t, stderr, "warning")
	testutil.Contains(t, stderr, "AYB_DATABASE_URL")
}

func TestFlyProviderDeployDefaultsRegionToIAD(t *testing.T) {
	mockClient := &mockFlyClient{}
	provider := flyProvider{
		client:        mockClient,
		jwtSecretFunc: func() (string, error) { return "generated-jwt-secret", nil },
	}

	cfg := DeployConfig{
		Provider: deployProviderFly,
		Env:      map[string]string{"AYB_DATABASE_URL": "postgres://db"},
		ProviderOptions: map[string]string{
			flyOptionAppName: "default-region-app",
			flyOptionImage:   "ghcr.io/gridlhq/ayb:latest",
		},
	}
	_, err := provider.Deploy(context.Background(), cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, "iad", mockClient.createVolumeReqs[0].Region)
}

func TestFlyHTTPClientErrorIncludesStatusAndBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"image pull failed"}`))
	}))
	defer ts.Close()

	client := newFlyHTTPClient("test-token", ts.URL)
	_, err := client.CreateMachine(context.Background(), "my-app", FlyCreateMachineRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *DeployAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected DeployAPIError, got %T: %v", err, err)
	}
	testutil.Equal(t, http.StatusUnprocessableEntity, apiErr.StatusCode)
	testutil.Contains(t, apiErr.Error(), "422")
	testutil.Contains(t, apiErr.Error(), "image pull failed")
}

func TestBuildFlyAPIErrorReturnsDeployAPIError(t *testing.T) {
	t.Parallel()

	apiErr := buildFlyAPIError(http.StatusUnprocessableEntity, []byte(`{"error":"image pull failed"}`))
	testutil.Equal(t, "fly", apiErr.Provider)
	testutil.Equal(t, http.StatusUnprocessableEntity, apiErr.StatusCode)
	testutil.Equal(t, "image pull failed", apiErr.Message)
}
