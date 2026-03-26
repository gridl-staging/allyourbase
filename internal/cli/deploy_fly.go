// Package cli implements deployment to Fly.io via HTTP API, including app and machine creation, secrets management, and configuration generation.
package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	flyDefaultBaseURL    = "https://api.machines.dev"
	flyDefaultRegion     = "iad"
	flyDefaultVMSize     = "shared-cpu-1x"
	flyDefaultVolumeSize = 1
	flyDefaultWaitTime   = 30 * time.Second
)

type FlyClient interface {
	CreateApp(ctx context.Context, req FlyCreateAppRequest) (FlyApp, error)
	GetApp(ctx context.Context, appName string) (FlyApp, error)
	CreateVolume(ctx context.Context, appName string, req FlyCreateVolumeRequest) (FlyVolume, error)
	SetSecrets(ctx context.Context, appName string, req FlySetSecretsRequest) error
	CreateMachine(ctx context.Context, appName string, req FlyCreateMachineRequest) (FlyMachine, error)
	WaitForMachineState(ctx context.Context, appName, machineID, state string, timeout time.Duration) error
}

type FlyCreateAppRequest struct {
	Name string
}

type FlyApp struct {
	Name string
}

type FlyCreateVolumeRequest struct {
	Name   string `json:"name,omitempty"`
	Region string `json:"region"`
	SizeGB int    `json:"size_gb"`
}

type FlyVolume struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FlySetSecretsRequest struct {
	Secrets map[string]string
}

type FlySecretEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type FlyCreateMachineRequest struct {
	Name   string           `json:"name,omitempty"`
	Config FlyMachineConfig `json:"config"`
}

type FlyMachineConfig struct {
	Image    string                   `json:"image"`
	Mounts   []FlyMachineMount        `json:"mounts,omitempty"`
	Services []FlyMachineService      `json:"services,omitempty"`
	Checks   map[string]FlyCheck      `json:"checks,omitempty"`
	Guest    FlyMachineGuest          `json:"guest"`
	Env      map[string]string        `json:"env,omitempty"`
	Metadata map[string]string        `json:"metadata,omitempty"`
	Restart  *FlyMachineRestartPolicy `json:"restart,omitempty"`
}

type FlyMachineRestartPolicy struct {
	Policy string `json:"policy"`
}

type FlyMachineMount struct {
	Volume string `json:"volume"`
	Path   string `json:"path"`
}

type FlyMachineService struct {
	Protocol     string           `json:"protocol"`
	InternalPort int              `json:"internal_port"`
	Ports        []FlyServicePort `json:"ports,omitempty"`
}

type FlyServicePort struct {
	Port     int      `json:"port"`
	Handlers []string `json:"handlers,omitempty"`
}

type FlyCheck struct {
	Type        string       `json:"type"`
	GracePeriod string       `json:"grace_period,omitempty"`
	Interval    string       `json:"interval,omitempty"`
	Timeout     string       `json:"timeout,omitempty"`
	HTTP        FlyHTTPCheck `json:"http_service"`
}

type FlyHTTPCheck struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Port   int    `json:"port"`
}

type FlyMachineGuest struct {
	Size string `json:"size"`
}

type FlyMachine struct {
	ID    string `json:"id"`
	State string `json:"state,omitempty"`
}

type flyHTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func newFlyHTTPClient(token, baseURL string) *flyHTTPClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = flyDefaultBaseURL
	}
	return &flyHTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// CreateApp sends an HTTP POST request to create an application with the given name, returning the created app or an error.
func (c *flyHTTPClient) CreateApp(ctx context.Context, req FlyCreateAppRequest) (FlyApp, error) {
	payload := struct {
		AppName string `json:"app_name"`
	}{
		AppName: req.Name,
	}
	var resp struct {
		Name string `json:"name"`
		App  struct {
			Name string `json:"name"`
		} `json:"app"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/apps", payload, &resp); err != nil {
		return FlyApp{}, err
	}
	if resp.Name == "" {
		resp.Name = resp.App.Name
	}
	return FlyApp{Name: resp.Name}, nil
}

// GetApp retrieves application details from the Fly API by name, returning the app or an error if not found.
func (c *flyHTTPClient) GetApp(ctx context.Context, appName string) (FlyApp, error) {
	var resp struct {
		Name string `json:"name"`
		App  struct {
			Name string `json:"name"`
		} `json:"app"`
	}
	path := "/v1/apps/" + url.PathEscape(appName)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return FlyApp{}, err
	}
	if resp.Name == "" {
		resp.Name = resp.App.Name
	}
	return FlyApp{Name: resp.Name}, nil
}

func (c *flyHTTPClient) CreateVolume(ctx context.Context, appName string, req FlyCreateVolumeRequest) (FlyVolume, error) {
	var resp FlyVolume
	path := "/v1/apps/" + url.PathEscape(appName) + "/volumes"
	if err := c.doJSON(ctx, http.MethodPost, path, req, &resp); err != nil {
		return FlyVolume{}, err
	}
	return resp, nil
}

// SetSecrets sends environment variables to the Fly API for the given application, sorting secret keys for deterministic ordering.
func (c *flyHTTPClient) SetSecrets(ctx context.Context, appName string, req FlySetSecretsRequest) error {
	entries := make([]FlySecretEntry, 0, len(req.Secrets))
	keys := make([]string, 0, len(req.Secrets))
	for k := range req.Secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entries = append(entries, FlySecretEntry{Key: key, Value: req.Secrets[key]})
	}
	payload := struct {
		Secrets []FlySecretEntry `json:"secrets"`
	}{Secrets: entries}
	path := "/v1/apps/" + url.PathEscape(appName) + "/secrets"
	return c.doJSON(ctx, http.MethodPost, path, payload, nil)
}

func (c *flyHTTPClient) CreateMachine(ctx context.Context, appName string, req FlyCreateMachineRequest) (FlyMachine, error) {
	var resp FlyMachine
	path := "/v1/apps/" + url.PathEscape(appName) + "/machines"
	if err := c.doJSON(ctx, http.MethodPost, path, req, &resp); err != nil {
		return FlyMachine{}, err
	}
	return resp, nil
}

func (c *flyHTTPClient) WaitForMachineState(ctx context.Context, appName, machineID, state string, timeout time.Duration) error {
	query := url.Values{}
	if strings.TrimSpace(state) != "" {
		query.Set("state", state)
	}
	if timeout > 0 {
		query.Set("timeout", strconv.Itoa(int(timeout.Seconds())))
	}
	path := "/v1/apps/" + url.PathEscape(appName) + "/machines/" + url.PathEscape(machineID) + "/wait"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return c.doJSON(ctx, http.MethodGet, path, nil, nil)
}

// doJSON performs an HTTP request to the Fly API with JSON serialization and deserialization, setting authorization headers and handling error responses.
func (c *flyHTTPClient) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal fly payload: %w", err)
		}
		body = strings.NewReader(string(raw))
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create fly request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform fly request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read fly response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return buildFlyAPIError(resp.StatusCode, respBody)
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode fly response: %w", err)
		}
	}
	return nil
}

// buildFlyAPIError parses an HTTP error response from the Fly API and constructs a DeployAPIError, extracting the error message from JSON fields or falling back to the HTTP status text.
func buildFlyAPIError(statusCode int, body []byte) *DeployAPIError {
	return buildDeployAPIError("fly", statusCode, body, "error", "message")
}

type flyProvider struct {
	client        FlyClient
	jwtSecretFunc func() (string, error)
	nowFunc       func() time.Time
	waitTimeout   time.Duration
}

func (p flyProvider) Name() string {
	return deployProviderFly
}

func (p flyProvider) Validate(cfg DeployConfig) error {
	image := strings.TrimSpace(cfg.ProviderOptions[flyOptionImage])
	if image == "" {
		return errors.New("--image is required. Build and push an image first, then pass --image <registry>/<repo>:<tag>")
	}
	warnMissingDatabaseConfig(cfg)
	return nil
}

// Deploy creates and configures a complete application deployment on Fly.io, provisioning an app, storage volume, setting secrets, and launching a virtual machine. It waits for the machine to reach the started state and returns deployment metadata and next-step instructions.
func (p flyProvider) Deploy(ctx context.Context, cfg DeployConfig) (DeployResult, error) {
	options := resolveFlyOptions(cfg)
	appName := options.AppName
	if appName == "" {
		appName = deriveFlyAppName(cfg.Domain, p.now())
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = flyDefaultRegion
	}

	client := p.client
	if client == nil {
		client = newFlyHTTPClient(cfg.Token, "")
	}

	if _, err := client.CreateApp(ctx, FlyCreateAppRequest{Name: appName}); err != nil {
		if !IsDeployStatusCode(err, http.StatusConflict) {
			return DeployResult{}, err
		}
		if _, getErr := client.GetApp(ctx, appName); getErr != nil {
			return DeployResult{}, getErr
		}
	}

	volumeName := flyVolumeName(appName)
	if _, err := client.CreateVolume(ctx, appName, FlyCreateVolumeRequest{
		Name:   volumeName,
		Region: region,
		SizeGB: options.VolumeSize,
	}); err != nil {
		return DeployResult{}, err
	}

	secrets, err := p.buildFlySecrets(cfg)
	if err != nil {
		return DeployResult{}, err
	}
	if err := client.SetSecrets(ctx, appName, FlySetSecretsRequest{Secrets: secrets}); err != nil {
		return DeployResult{}, err
	}

	machine, err := client.CreateMachine(ctx, appName, FlyCreateMachineRequest{
		Name: appName + "-vm",
		Config: FlyMachineConfig{
			Image: options.Image,
			Mounts: []FlyMachineMount{
				{Volume: volumeName, Path: "/data"},
			},
			Services: []FlyMachineService{
				{
					Protocol:     "tcp",
					InternalPort: 8090,
					Ports: []FlyServicePort{
						{Port: 80, Handlers: []string{"http"}},
						{Port: 443, Handlers: []string{"tls", "http"}},
					},
				},
			},
			Checks: map[string]FlyCheck{
				"http": {
					Type:        "http",
					GracePeriod: "20s",
					Interval:    "15s",
					Timeout:     "10s",
					HTTP: FlyHTTPCheck{
						Method: "GET",
						Path:   "/health",
						Port:   8090,
					},
				},
			},
			Guest: FlyMachineGuest{Size: options.VMSize},
			Restart: &FlyMachineRestartPolicy{
				Policy: "always",
			},
		},
	})
	if err != nil {
		return DeployResult{}, err
	}

	if err := client.WaitForMachineState(ctx, appName, machine.ID, "started", p.timeout()); err != nil {
		return DeployResult{}, err
	}

	appURL := fmt.Sprintf("https://%s.fly.dev", appName)
	nextSteps := []string{
		fmt.Sprintf("Set an admin password: ayb admin reset-password --url %s", appURL),
		fmt.Sprintf("View logs: flyctl logs -a %s", appName),
	}
	if strings.TrimSpace(cfg.Domain) != "" {
		nextSteps = append(nextSteps, fmt.Sprintf("Create CNAME record: %s -> %s.fly.dev", strings.TrimSpace(cfg.Domain), appName))
	}

	result := DeployResult{
		Provider:     deployProviderFly,
		AppURL:       appURL,
		DashboardURL: fmt.Sprintf("https://fly.io/apps/%s", appName),
		NextSteps:    nextSteps,
		Metadata: map[string]any{
			"app_name":    appName,
			"region":      region,
			"volume_name": volumeName,
			"machine_id":  machine.ID,
			"fly_toml":    generateFlyToml(appName, region),
			"dockerfile":  generateDockerfile(),
		},
	}
	return result, nil
}

func (p flyProvider) buildFlySecrets(cfg DeployConfig) (map[string]string, error) {
	secrets, err := mergeDeployEnv(cfg)
	if err != nil {
		return nil, err
	}
	// Allow deterministic secrets in tests while keeping mergeDeployEnv as the shared source.
	if p.jwtSecretFunc != nil {
		jwtSecret, err := p.newJWTSecret()
		if err != nil {
			return nil, err
		}
		secrets["AYB_AUTH_JWT_SECRET"] = jwtSecret
	}
	return secrets, nil
}

func (p flyProvider) newJWTSecret() (string, error) {
	if p.jwtSecretFunc != nil {
		return p.jwtSecretFunc()
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate AYB_AUTH_JWT_SECRET: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func (p flyProvider) now() time.Time {
	if p.nowFunc != nil {
		return p.nowFunc()
	}
	return time.Now().UTC()
}

func (p flyProvider) timeout() time.Duration {
	return resolveProviderTimeout(p.waitTimeout, flyDefaultWaitTime)
}

type flyOptions struct {
	AppName    string
	Image      string
	VMSize     string
	VolumeSize int
}

// resolveFlyOptions extracts and normalizes Fly deployment options from the configuration, applying defaults for VM size and volume size.
func resolveFlyOptions(cfg DeployConfig) flyOptions {
	appName := sanitizeFlyAppName(cfg.ProviderOptions[flyOptionAppName])
	image := strings.TrimSpace(cfg.ProviderOptions[flyOptionImage])
	vmSize := strings.TrimSpace(cfg.ProviderOptions[flyOptionVMSize])
	if vmSize == "" {
		vmSize = flyDefaultVMSize
	}
	volumeSize := flyDefaultVolumeSize
	if raw := strings.TrimSpace(cfg.ProviderOptions[flyOptionVolumeSize]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			volumeSize = parsed
		}
	}
	return flyOptions{
		AppName:    appName,
		Image:      image,
		VMSize:     vmSize,
		VolumeSize: volumeSize,
	}
}

// deriveFlyAppName generates a valid Fly app name by normalizing the provided domain, applying sanitization, and using a timestamp-based fallback if the domain is empty or invalid. The returned name is at most 63 characters.
func deriveFlyAppName(domain string, now time.Time) string {
	candidate := normalizeDomainForAppName(domain)
	if candidate == "" {
		candidate = fmt.Sprintf("ayb-%d", now.Unix())
	}
	candidate = sanitizeFlyAppName(candidate)
	if candidate == "" {
		candidate = fmt.Sprintf("ayb-%d", now.Unix())
	}
	if len(candidate) > 63 {
		candidate = strings.Trim(candidate[:63], "-")
	}
	if candidate == "" {
		candidate = fmt.Sprintf("ayb-%d", now.Unix())
	}
	return candidate
}

func normalizeDomainForAppName(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	if idx := strings.Index(domain, "/"); idx >= 0 {
		domain = domain[:idx]
	}
	if idx := strings.Index(domain, ":"); idx >= 0 {
		domain = domain[:idx]
	}
	return domain
}

// sanitizeFlyAppName converts a string into a valid Fly application name by converting to lowercase, replacing special characters with hyphens, collapsing consecutive hyphens, and truncating to 63 characters.
func sanitizeFlyAppName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(".", "-", "_", "-", " ", "-")
	value = replacer.Replace(value)

	var b strings.Builder
	lastHyphen := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if r == '-' {
			if !lastHyphen {
				b.WriteRune('-')
				lastHyphen = true
			}
		}
	}
	clean := strings.Trim(b.String(), "-")
	if len(clean) > 63 {
		clean = strings.Trim(clean[:63], "-")
	}
	return clean
}

func flyVolumeName(appName string) string {
	return appName + "-data"
}

// generateFlyToml generates fly.toml configuration file content for the given app name and region, including HTTP service, health checks, and volume mount settings.
func generateFlyToml(appName, region string) string {
	if strings.TrimSpace(region) == "" {
		region = flyDefaultRegion
	}
	volumeName := flyVolumeName(appName)
	return fmt.Sprintf(`app = "%s"
primary_region = "%s"

[http_service]
  internal_port = 8090
  force_https = true

[checks]
  [checks.http]
    type = "http"
    path = "/health"
    interval = "15s"
    timeout = "10s"

[[mounts]]
  source = "%s"
  destination = "/data"
`, appName, region, volumeName)
}

func generateDockerfile() string {
	return `FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY ayb /usr/local/bin/ayb
EXPOSE 8090
ENTRYPOINT ["ayb"]
CMD ["start"]
`
}
