package cli

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

type mockDigitalOceanClient struct {
	calls []string

	createDropletReqs  []DigitalOceanDropletCreateRequest
	createDropletResp  DigitalOceanDroplet
	createDropletErr   error
	waitActiveReqs     []int
	waitActiveResp     DigitalOceanDroplet
	waitActiveErr      error
	waitActiveFunc     func(ctx context.Context, id int) (DigitalOceanDroplet, error)
	waitPublicIPReqs   []int
	waitPublicIPResp   string
	waitPublicIPErr    error
	waitPublicIPFunc   func(ctx context.Context, id int) (string, error)
	createFirewallReqs []DigitalOceanFirewallCreateRequest
	createFirewallResp DigitalOceanFirewall
	createFirewallErr  error
}

func (m *mockDigitalOceanClient) CreateDroplet(_ context.Context, req DigitalOceanDropletCreateRequest) (DigitalOceanDroplet, error) {
	m.calls = append(m.calls, "CreateDroplet")
	m.createDropletReqs = append(m.createDropletReqs, req)
	if m.createDropletErr != nil {
		return DigitalOceanDroplet{}, m.createDropletErr
	}
	if m.createDropletResp.ID == 0 {
		m.createDropletResp = DigitalOceanDroplet{ID: 101, Name: req.Name, Status: "new"}
	}
	return m.createDropletResp, nil
}

func (m *mockDigitalOceanClient) GetDroplet(_ context.Context, id int) (DigitalOceanDroplet, error) {
	return DigitalOceanDroplet{ID: id, Status: "active"}, nil
}

func (m *mockDigitalOceanClient) CreateFirewall(_ context.Context, req DigitalOceanFirewallCreateRequest) (DigitalOceanFirewall, error) {
	m.calls = append(m.calls, "CreateFirewall")
	m.createFirewallReqs = append(m.createFirewallReqs, req)
	if m.createFirewallErr != nil {
		return DigitalOceanFirewall{}, m.createFirewallErr
	}
	if m.createFirewallResp.ID == "" {
		m.createFirewallResp = DigitalOceanFirewall{ID: "fw-1", Name: req.Name}
	}
	return m.createFirewallResp, nil
}

func (m *mockDigitalOceanClient) WaitForDropletActive(ctx context.Context, id int) (DigitalOceanDroplet, error) {
	m.calls = append(m.calls, "WaitForDropletActive")
	m.waitActiveReqs = append(m.waitActiveReqs, id)
	if m.waitActiveFunc != nil {
		return m.waitActiveFunc(ctx, id)
	}
	if m.waitActiveErr != nil {
		return DigitalOceanDroplet{}, m.waitActiveErr
	}
	if m.waitActiveResp.ID == 0 {
		m.waitActiveResp = DigitalOceanDroplet{ID: id, Status: "active"}
	}
	return m.waitActiveResp, nil
}

func (m *mockDigitalOceanClient) WaitForDropletPublicIP(ctx context.Context, id int) (string, error) {
	m.calls = append(m.calls, "WaitForDropletPublicIP")
	m.waitPublicIPReqs = append(m.waitPublicIPReqs, id)
	if m.waitPublicIPFunc != nil {
		return m.waitPublicIPFunc(ctx, id)
	}
	if m.waitPublicIPErr != nil {
		return "", m.waitPublicIPErr
	}
	if m.waitPublicIPResp == "" {
		m.waitPublicIPResp = "203.0.113.20"
	}
	return m.waitPublicIPResp, nil
}

func TestGenerateAYBSystemdUnit(t *testing.T) {
	unit := generateAYBSystemdUnit()
	testutil.Contains(t, unit, "[Unit]")
	testutil.Contains(t, unit, "ExecStart=/usr/local/bin/ayb start")
	testutil.Contains(t, unit, "EnvironmentFile=/etc/ayb/.env")
	testutil.Contains(t, unit, "WantedBy=multi-user.target")
}

func TestGenerateDigitalOceanCloudInit(t *testing.T) {
	cfg := DeployConfig{}
	opts := digitalOceanProviderOption{BinaryURL: "https://example.com/ayb.tar.gz"}
	env := map[string]string{"APP_ENV": "production", "AYB_DATABASE_URL": "postgres://db"}

	out, err := generateDigitalOceanCloudInit(cfg, opts, env)
	testutil.NoError(t, err)
	testutil.Contains(t, out, "#cloud-config")
	testutil.Contains(t, out, "curl -fsSL 'https://example.com/ayb.tar.gz'")
	testutil.Contains(t, out, "APP_ENV='production'")
	testutil.Contains(t, out, "AYB_DATABASE_URL='postgres://db'")
	testutil.Contains(t, out, "systemctl enable ayb.service")
	testutil.Contains(t, out, "/etc/systemd/system/ayb.service")
	testutil.Contains(t, out, "Description=Allyourbase Server")
	testutil.Contains(t, out, "ExecStart=/usr/local/bin/ayb start")
	testutil.Contains(t, out, "find /tmp/ayb-extract -type f -name 'ayb'")
	if strings.Contains(out, "-name 'ayb*'") {
		t.Fatalf("cloud-init should not use broad ayb* glob for binary selection: %s", out)
	}
}

func TestDigitalOceanProviderValidate(t *testing.T) {
	provider := digitalOceanProvider{}

	err := provider.Validate(DeployConfig{ProviderOptions: map[string]string{}})
	testutil.ErrorContains(t, err, "--binary-url")

	stderr := captureStderr(t, func() {
		err = provider.Validate(DeployConfig{ProviderOptions: map[string]string{doOptionBinaryURL: "https://example.com/ayb.tar.gz"}})
		testutil.NoError(t, err)
	})
	testutil.Contains(t, stderr, "warning")
	testutil.Contains(t, stderr, "--postgres-url")
}

func TestDigitalOceanProviderDeployHappyPath(t *testing.T) {
	mock := &mockDigitalOceanClient{}
	provider := digitalOceanProvider{client: mock}
	cfg := DeployConfig{
		Provider:    deployProviderDigitalOcean,
		Domain:      "api.example.com",
		Region:      "lon1",
		PostgresURL: "postgres://db",
		Env:         map[string]string{"APP_ENV": "production"},
		ProviderOptions: map[string]string{
			doOptionBinaryURL:    "https://example.com/ayb.tar.gz",
			doOptionDropletSize:  "s-1vcpu-1gb",
			doOptionDropletImage: "ubuntu-22-04-x64",
			doOptionSSHKeyID:     "123,456",
			doOptionFirewallName: "custom-fw",
		},
	}

	result, err := provider.Deploy(context.Background(), cfg)
	testutil.NoError(t, err)
	if !reflect.DeepEqual([]string{"CreateDroplet", "WaitForDropletActive", "WaitForDropletPublicIP", "CreateFirewall"}, mock.calls) {
		t.Fatalf("unexpected call order: %v", mock.calls)
	}
	testutil.Equal(t, deployProviderDigitalOcean, result.Provider)
	testutil.Equal(t, "https://api.example.com", result.AppURL)
	testutil.Contains(t, result.DashboardURL, "cloud.digitalocean.com/droplets/")

	req := mock.createDropletReqs[0]
	testutil.Equal(t, "lon1", req.Region)
	testutil.Equal(t, "s-1vcpu-1gb", req.Size)
	testutil.Equal(t, "ubuntu-22-04-x64", req.Image)
	if !reflect.DeepEqual([]string{"123", "456"}, req.SSHKeys) {
		t.Fatalf("unexpected ssh keys: %v", req.SSHKeys)
	}
	testutil.Contains(t, req.UserData, "AYB_DATABASE_URL='postgres://db'")
	testutil.Contains(t, req.UserData, "AYB_AUTH_JWT_SECRET=")

	fwReq := mock.createFirewallReqs[0]
	testutil.Equal(t, "custom-fw", fwReq.Name)
	testutil.Equal(t, "22", fwReq.InboundRules[0].PortRange)
	testutil.Equal(t, "8090", fwReq.InboundRules[1].PortRange)
	if !reflect.DeepEqual([]string{"0.0.0.0/0", "::/0"}, fwReq.InboundRules[0].Sources.Addresses) {
		t.Fatalf("unexpected inbound sources: %v", fwReq.InboundRules[0].Sources.Addresses)
	}
	if !reflect.DeepEqual([]string{"0.0.0.0/0", "::/0"}, fwReq.OutboundRules[0].Destinations.Addresses) {
		t.Fatalf("unexpected outbound destinations: %v", fwReq.OutboundRules[0].Destinations.Addresses)
	}
}

func TestDigitalOceanProviderDeployDefaultsRegionAndIPURL(t *testing.T) {
	mock := &mockDigitalOceanClient{waitPublicIPResp: "198.51.100.44"}
	provider := digitalOceanProvider{client: mock}
	cfg := DeployConfig{
		Provider: deployProviderDigitalOcean,
		ProviderOptions: map[string]string{
			doOptionBinaryURL: "https://example.com/ayb.tar.gz",
		},
		Env: map[string]string{"AYB_DATABASE_URL": "postgres://db"},
	}

	result, err := provider.Deploy(context.Background(), cfg)
	testutil.NoError(t, err)
	testutil.Equal(t, doDefaultRegion, mock.createDropletReqs[0].Region)
	testutil.Equal(t, "http://198.51.100.44:8090", result.AppURL)
}

func TestDigitalOceanProviderDeployErrorPropagation(t *testing.T) {
	mock := &mockDigitalOceanClient{createFirewallErr: fmt.Errorf("API error (status 422): invalid firewall rule")}
	provider := digitalOceanProvider{client: mock}
	cfg := DeployConfig{
		Provider: deployProviderDigitalOcean,
		Env:      map[string]string{"AYB_DATABASE_URL": "postgres://db"},
		ProviderOptions: map[string]string{
			doOptionBinaryURL: "https://example.com/ayb.tar.gz",
		},
	}

	_, err := provider.Deploy(context.Background(), cfg)
	testutil.ErrorContains(t, err, "creating firewall")
	testutil.ErrorContains(t, err, "422")
	testutil.ErrorContains(t, err, "invalid firewall rule")
}

func TestDigitalOceanProviderDeployTimeout(t *testing.T) {
	mock := &mockDigitalOceanClient{
		waitActiveFunc: func(ctx context.Context, id int) (DigitalOceanDroplet, error) {
			<-ctx.Done()
			return DigitalOceanDroplet{}, ctx.Err()
		},
	}
	provider := digitalOceanProvider{client: mock, waitTimeout: 15 * time.Millisecond}
	cfg := DeployConfig{
		Provider: deployProviderDigitalOcean,
		Env:      map[string]string{"AYB_DATABASE_URL": "postgres://db"},
		ProviderOptions: map[string]string{
			doOptionBinaryURL: "https://example.com/ayb.tar.gz",
		},
	}

	_, err := provider.Deploy(context.Background(), cfg)
	testutil.ErrorContains(t, err, "waiting for droplet active state")
	testutil.ErrorContains(t, err, "context deadline exceeded")
}

func TestDigitalOceanParseSSHKeyIDs(t *testing.T) {
	opts := resolveDigitalOceanOptions(DeployConfig{ProviderOptions: map[string]string{doOptionSSHKeyID: "a, b ,c"}})
	if !reflect.DeepEqual([]string{"a", "b", "c"}, opts.SSHKeyIDs) {
		t.Fatalf("unexpected ssh keys parse: %v", opts.SSHKeyIDs)
	}
}

func TestGenerateDigitalOceanCloudInitEscapesEnvValues(t *testing.T) {
	out, err := generateDigitalOceanCloudInit(
		DeployConfig{},
		digitalOceanProviderOption{BinaryURL: "https://example.com/ayb.tar.gz"},
		map[string]string{"TOKEN": "ab'cd", "SPACE": "hello world"},
	)
	testutil.NoError(t, err)
	if strings.Contains(out, "TOKEN=ab'cd") {
		t.Fatalf("expected single quote in env value to be escaped in cloud-init output: %s", out)
	}
	testutil.Contains(t, out, "TOKEN='ab'\"'\"'cd'")
	testutil.Contains(t, out, "SPACE='hello world'")
}

func TestGenerateDigitalOceanCloudInitQuotesBinaryURL(t *testing.T) {
	out, err := generateDigitalOceanCloudInit(
		DeployConfig{},
		digitalOceanProviderOption{BinaryURL: "https://example.com/ayb.tar.gz?token=a&download=1"},
		map[string]string{"AYB_DATABASE_URL": "postgres://db"},
	)
	testutil.NoError(t, err)
	testutil.Contains(t, out, "curl -fsSL 'https://example.com/ayb.tar.gz?token=a&download=1' -o /tmp/ayb.tgz")
}

func TestBuildDigitalOceanAPIErrorReturnsDeployAPIError(t *testing.T) {
	t.Parallel()

	apiErr := buildDigitalOceanAPIError(http.StatusUnprocessableEntity, []byte(`{"message":"firewall invalid"}`))
	testutil.Equal(t, "digitalocean", apiErr.Provider)
	testutil.Equal(t, http.StatusUnprocessableEntity, apiErr.StatusCode)
	testutil.Equal(t, "firewall invalid", apiErr.Message)
}
