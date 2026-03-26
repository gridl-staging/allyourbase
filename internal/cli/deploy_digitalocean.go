// Package cli provides DigitalOcean cloud deployment functionality, including API client implementation, droplet and firewall provisioning, and cloud-init configuration for AYB binary installation.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"text/template"
	"time"
)

const (
	doDefaultBaseURL      = "https://api.digitalocean.com/v2"
	doDefaultRegion       = "nyc1"
	doDefaultDropletSize  = "s-1vcpu-1gb"
	doDefaultDropletImage = "ubuntu-22-04-x64"
	doDefaultWaitTimeout  = 2 * time.Minute
	doPollInterval        = 2 * time.Second

	doOptionDropletSize  = "droplet_size"
	doOptionDropletImage = "droplet_image"
	doOptionSSHKeyID     = "ssh_key_id"
	doOptionFirewallName = "firewall_name"
	doOptionBinaryURL    = "binary_url"
)

// DigitalOceanClient defines the interface for interacting with the DigitalOcean API.
type DigitalOceanClient interface {
	CreateDroplet(ctx context.Context, req DigitalOceanDropletCreateRequest) (DigitalOceanDroplet, error)
	GetDroplet(ctx context.Context, id int) (DigitalOceanDroplet, error)
	CreateFirewall(ctx context.Context, req DigitalOceanFirewallCreateRequest) (DigitalOceanFirewall, error)
	WaitForDropletActive(ctx context.Context, id int) (DigitalOceanDroplet, error)
	WaitForDropletPublicIP(ctx context.Context, id int) (string, error)
}

// digitalOceanHTTPClient implements DigitalOceanClient with HTTP API calls.
type digitalOceanHTTPClient struct {
	client  *http.Client
	token   string
	baseURL string
}

func newDigitalOceanHTTPClient(token string) *digitalOceanHTTPClient {
	return &digitalOceanHTTPClient{
		client:  &http.Client{Timeout: 30 * time.Second},
		token:   strings.TrimSpace(token),
		baseURL: doDefaultBaseURL,
	}
}

// DigitalOceanDroplet represents a droplet response from DigitalOcean API.
type DigitalOceanDroplet struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	PublicIP string `json:"public_ip_address,omitempty"`
	Networks struct {
		V4 []struct {
			IPAddress string `json:"ip_address"`
			Type      string `json:"type"`
		} `json:"v4"`
	} `json:"networks,omitempty"`
}

// DigitalOceanFirewall represents a firewall response from DigitalOcean API.
type DigitalOceanFirewall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DigitalOceanFirewallTargets struct {
	Addresses []string `json:"addresses,omitempty"`
}

// DigitalOceanInboundFirewallRule represents an inbound firewall rule for DigitalOcean.
type DigitalOceanInboundFirewallRule struct {
	Protocol  string                      `json:"protocol"`
	PortRange string                      `json:"ports"`
	Sources   DigitalOceanFirewallTargets `json:"sources"`
}

// DigitalOceanOutboundFirewallRule represents an outbound firewall rule for DigitalOcean.
type DigitalOceanOutboundFirewallRule struct {
	Protocol     string                      `json:"protocol"`
	PortRange    string                      `json:"ports"`
	Destinations DigitalOceanFirewallTargets `json:"destinations"`
}

// DigitalOceanDropletCreateRequest represents a request to create a droplet.
type DigitalOceanDropletCreateRequest struct {
	Name     string   `json:"name"`
	Region   string   `json:"region"`
	Size     string   `json:"size"`
	Image    string   `json:"image"`
	SSHKeys  []string `json:"ssh_keys,omitempty"`
	UserData string   `json:"user_data,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// DigitalOceanFirewallCreateRequest represents a request to create a firewall.
type DigitalOceanFirewallCreateRequest struct {
	Name          string                             `json:"name"`
	InboundRules  []DigitalOceanInboundFirewallRule  `json:"inbound_rules"`
	OutboundRules []DigitalOceanOutboundFirewallRule `json:"outbound_rules"`
	DropletIDs    []int                              `json:"droplet_ids"`
}

// doJSON performs an authenticated JSON HTTP request to the DigitalOcean API, marshaling the request body, reading the response, and unmarshaling it into the out parameter. It returns DeployAPIError for non-2xx status codes.
func (c *digitalOceanHTTPClient) doJSON(ctx context.Context, method, path string, reqBody, out any) error {
	var body io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal digitalocean payload: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create digitalocean request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform digitalocean request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read digitalocean response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return buildDigitalOceanAPIError(resp.StatusCode, respBody)
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode digitalocean response: %w", err)
		}
	}
	return nil
}

// buildDigitalOceanAPIError constructs a DeployAPIError from an HTTP status code and response body, attempting to extract an error message from the JSON message or error fields, falling back to the raw body or HTTP status text if parsing fails.
func buildDigitalOceanAPIError(statusCode int, body []byte) *DeployAPIError {
	return buildDeployAPIError("digitalocean", statusCode, body, "message", "error")
}

func (c *digitalOceanHTTPClient) CreateDroplet(ctx context.Context, req DigitalOceanDropletCreateRequest) (DigitalOceanDroplet, error) {
	var resp struct {
		Droplet DigitalOceanDroplet `json:"droplet"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/droplets", req, &resp); err != nil {
		return DigitalOceanDroplet{}, err
	}
	return resp.Droplet, nil
}

// GetDroplet retrieves a droplet by ID from the DigitalOcean API, extracting the public IP address from the networks field if not already populated in the response.
func (c *digitalOceanHTTPClient) GetDroplet(ctx context.Context, id int) (DigitalOceanDroplet, error) {
	path := fmt.Sprintf("/droplets/%d", id)
	var resp struct {
		Droplet DigitalOceanDroplet `json:"droplet"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return DigitalOceanDroplet{}, err
	}

	if resp.Droplet.PublicIP == "" {
		for _, n := range resp.Droplet.Networks.V4 {
			if n.Type == "public" {
				resp.Droplet.PublicIP = strings.TrimSpace(n.IPAddress)
				break
			}
		}
	}
	return resp.Droplet, nil
}

func (c *digitalOceanHTTPClient) CreateFirewall(ctx context.Context, req DigitalOceanFirewallCreateRequest) (DigitalOceanFirewall, error) {
	var resp struct {
		Firewall DigitalOceanFirewall `json:"firewall"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/firewalls", req, &resp); err != nil {
		return DigitalOceanFirewall{}, err
	}
	return resp.Firewall, nil
}

func (c *digitalOceanHTTPClient) WaitForDropletActive(ctx context.Context, id int) (DigitalOceanDroplet, error) {
	for {
		droplet, err := c.GetDroplet(ctx, id)
		if err != nil {
			return DigitalOceanDroplet{}, err
		}
		if droplet.Status == "active" {
			return droplet, nil
		}
		if err := sleepWithContext(ctx, doPollInterval); err != nil {
			return DigitalOceanDroplet{}, err
		}
	}
}

// WaitForDropletPublicIP polls the DigitalOcean API at regular intervals until the droplet has a public IP address assigned, returning the IP or an error if the context is cancelled or times out.
func (c *digitalOceanHTTPClient) WaitForDropletPublicIP(ctx context.Context, id int) (string, error) {
	for {
		droplet, err := c.GetDroplet(ctx, id)
		if err != nil {
			return "", err
		}
		if ip := strings.TrimSpace(droplet.PublicIP); ip != "" {
			return ip, nil
		}
		for _, n := range droplet.Networks.V4 {
			if n.Type == "public" && strings.TrimSpace(n.IPAddress) != "" {
				return strings.TrimSpace(n.IPAddress), nil
			}
		}
		if err := sleepWithContext(ctx, doPollInterval); err != nil {
			return "", err
		}
	}
}

// digitalOceanProviderOption holds DigitalOcean-specific provider options.
type digitalOceanProviderOption struct {
	DropletSize  string
	DropletImage string
	SSHKeyIDs    []string
	FirewallName string
	BinaryURL    string
}

// resolveDigitalOceanOptions extracts DigitalOcean-specific deployment options from the configuration, applying default values for droplet size and image if not specified.
func resolveDigitalOceanOptions(cfg DeployConfig) digitalOceanProviderOption {
	opts := digitalOceanProviderOption{
		DropletSize:  strings.TrimSpace(cfg.ProviderOptions[doOptionDropletSize]),
		DropletImage: strings.TrimSpace(cfg.ProviderOptions[doOptionDropletImage]),
		FirewallName: strings.TrimSpace(cfg.ProviderOptions[doOptionFirewallName]),
		BinaryURL:    strings.TrimSpace(cfg.ProviderOptions[doOptionBinaryURL]),
		SSHKeyIDs:    splitCommaOption(strings.TrimSpace(cfg.ProviderOptions[doOptionSSHKeyID])),
	}
	if opts.DropletSize == "" {
		opts.DropletSize = doDefaultDropletSize
	}
	if opts.DropletImage == "" {
		opts.DropletImage = doDefaultDropletImage
	}
	return opts
}

// digitalOceanProvider implements the DeployProvider interface for DigitalOcean.
type digitalOceanProvider struct {
	client      DigitalOceanClient
	waitTimeout time.Duration
}

func (p digitalOceanProvider) Name() string {
	return deployProviderDigitalOcean
}

func (p digitalOceanProvider) Validate(cfg DeployConfig) error {
	opts := resolveDigitalOceanOptions(cfg)
	if opts.BinaryURL == "" {
		return fmt.Errorf("DigitalOcean deployment requires --binary-url (URL to pre-built AYB binary tarball)\n  Example: --binary-url https://github.com/allyourbase/ayb/releases/download/v0.1.0/ayb_0.1.0_linux_amd64.tar.gz")
	}
	warnMissingDatabaseConfig(cfg)
	return nil
}

// Deploy creates a DigitalOcean droplet with the specified configuration, waits for it to become active, retrieves its public IP, creates and attaches a firewall, and returns deployment details including the app URL and dashboard link.
func (p digitalOceanProvider) Deploy(ctx context.Context, cfg DeployConfig) (DeployResult, error) {
	waitCtx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()

	opts := resolveDigitalOceanOptions(cfg)
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = doDefaultRegion
	}

	mergedEnv, err := mergeDeployEnv(cfg)
	if err != nil {
		return DeployResult{}, fmt.Errorf("merge deploy env: %w", err)
	}

	appName := deriveAppName(cfg.Domain, "ayb-do-")
	userData, err := generateDigitalOceanCloudInit(cfg, opts, mergedEnv)
	if err != nil {
		return DeployResult{}, fmt.Errorf("generate cloud-init: %w", err)
	}

	client := p.client
	if client == nil {
		client = newDigitalOceanHTTPClient(cfg.Token)
	}

	droplet, err := client.CreateDroplet(waitCtx, DigitalOceanDropletCreateRequest{
		Name:     appName,
		Region:   region,
		Size:     opts.DropletSize,
		Image:    opts.DropletImage,
		SSHKeys:  opts.SSHKeyIDs,
		UserData: userData,
		Tags:     []string{"ayb-deployment", appName},
	})
	if err != nil {
		return DeployResult{}, fmt.Errorf("creating droplet: %w", err)
	}

	if _, err := client.WaitForDropletActive(waitCtx, droplet.ID); err != nil {
		return DeployResult{}, fmt.Errorf("waiting for droplet active state: %w", err)
	}

	publicIP, err := client.WaitForDropletPublicIP(waitCtx, droplet.ID)
	if err != nil {
		return DeployResult{}, fmt.Errorf("waiting for droplet public IP: %w", err)
	}

	firewallName := opts.FirewallName
	if firewallName == "" {
		firewallName = fmt.Sprintf("%s-fw", appName)
	}

	firewall, err := client.CreateFirewall(waitCtx, DigitalOceanFirewallCreateRequest{
		Name: firewallName,
		InboundRules: []DigitalOceanInboundFirewallRule{
			{Protocol: "tcp", PortRange: "22", Sources: DigitalOceanFirewallTargets{Addresses: []string{"0.0.0.0/0", "::/0"}}},
			{Protocol: "tcp", PortRange: "8090", Sources: DigitalOceanFirewallTargets{Addresses: []string{"0.0.0.0/0", "::/0"}}},
		},
		OutboundRules: []DigitalOceanOutboundFirewallRule{
			{Protocol: "tcp", PortRange: "all", Destinations: DigitalOceanFirewallTargets{Addresses: []string{"0.0.0.0/0", "::/0"}}},
			{Protocol: "udp", PortRange: "all", Destinations: DigitalOceanFirewallTargets{Addresses: []string{"0.0.0.0/0", "::/0"}}},
			{Protocol: "icmp", PortRange: "all", Destinations: DigitalOceanFirewallTargets{Addresses: []string{"0.0.0.0/0", "::/0"}}},
		},
		DropletIDs: []int{droplet.ID},
	})
	if err != nil {
		return DeployResult{}, fmt.Errorf("creating firewall: %w", err)
	}

	appURL := fmt.Sprintf("http://%s:8090", publicIP)
	if strings.TrimSpace(cfg.Domain) != "" {
		appURL = "https://" + strings.TrimSpace(cfg.Domain)
	}

	nextSteps := []string{
		fmt.Sprintf("SSH into the host: ssh root@%s", publicIP),
		"Inspect service logs: sudo journalctl -u ayb.service -f",
		fmt.Sprintf("View droplet in dashboard: https://cloud.digitalocean.com/droplets/%d", droplet.ID),
	}
	if strings.TrimSpace(cfg.Domain) != "" {
		nextSteps = append(nextSteps, fmt.Sprintf("Create DNS A record: %s -> %s", strings.TrimSpace(cfg.Domain), publicIP))
	}

	return DeployResult{
		Provider:     deployProviderDigitalOcean,
		AppURL:       appURL,
		DashboardURL: fmt.Sprintf("https://cloud.digitalocean.com/droplets/%d", droplet.ID),
		NextSteps:    nextSteps,
		Metadata: map[string]any{
			"droplet_id":  droplet.ID,
			"firewall_id": firewall.ID,
			"public_ip":   publicIP,
		},
	}, nil
}

func (p digitalOceanProvider) timeout() time.Duration {
	return resolveProviderTimeout(p.waitTimeout, doDefaultWaitTimeout)
}

// generateDigitalOceanCloudInit generates cloud-init user data for AYB binary installation.
func generateDigitalOceanCloudInit(_ DeployConfig, opts digitalOceanProviderOption, mergedEnv map[string]string) (string, error) {
	if strings.TrimSpace(opts.BinaryURL) == "" {
		return "", errors.New("binary URL is required for cloud-init generation")
	}

	envKeys := make([]string, 0, len(mergedEnv))
	for k := range mergedEnv {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	envLines := make([]string, 0, len(envKeys))
	for _, k := range envKeys {
		envLines = append(envLines, fmt.Sprintf("%s=%s", k, shellSingleQuote(mergedEnv[k])))
	}

	data := struct {
		BinaryURLQuoted string
		EnvLines        []string
		SystemdUnit     string
	}{
		BinaryURLQuoted: shellSingleQuote(strings.TrimSpace(opts.BinaryURL)),
		EnvLines:        envLines,
		SystemdUnit:     generateAYBSystemdUnit(),
	}

	const cloudInitTemplate = `#cloud-config
package_update: true
package_upgrade: true
packages:
  - curl
  - ca-certificates

write_files:
  - path: /etc/ayb/.env
    owner: root:root
    permissions: '0644'
    content: |
{{- range .EnvLines }}
      {{ . }}
{{- end }}
  - path: /etc/systemd/system/ayb.service
    owner: root:root
    permissions: '0644'
    content: |
{{ indent .SystemdUnit 6 }}

runcmd:
  - mkdir -p /etc/ayb /usr/local/bin
  - |
      if [ ! -x /usr/local/bin/ayb ]; then
        curl -fsSL {{ .BinaryURLQuoted }} -o /tmp/ayb.tgz
        rm -rf /tmp/ayb-extract
        mkdir -p /tmp/ayb-extract
        tar -xzf /tmp/ayb.tgz -C /tmp/ayb-extract
        AYB_BIN="$(find /tmp/ayb-extract -type f -name 'ayb' | head -n1)"
        if [ -z "$AYB_BIN" ]; then
          echo "ayb binary not found in archive"
          exit 1
        fi
        install -m 0755 "$AYB_BIN" /usr/local/bin/ayb
      fi
  - systemctl daemon-reload
  - systemctl enable ayb.service
  - systemctl restart ayb.service
`

	tmpl, err := template.New("do-cloud-init").Funcs(template.FuncMap{
		"indent": func(s string, spaces int) string {
			pad := strings.Repeat(" ", spaces)
			lines := strings.Split(s, "\n")
			for i := range lines {
				lines[i] = pad + lines[i]
			}
			return strings.Join(lines, "\n")
		},
	}).Parse(cloudInitTemplate)
	if err != nil {
		return "", fmt.Errorf("parse cloud-init template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render cloud-init template: %w", err)
	}
	return buf.String(), nil
}

// generateAYBSystemdUnit returns the AYB service unit content.
func generateAYBSystemdUnit() string {
	return `[Unit]
Description=Allyourbase Server
After=network.target

[Service]
Type=simple
User=root
Group=root
EnvironmentFile=/etc/ayb/.env
ExecStart=/usr/local/bin/ayb start
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target`
}

func splitCommaOption(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func shellSingleQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		d = time.Second
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
