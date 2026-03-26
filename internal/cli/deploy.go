// Package cli provides cloud deployment commands and abstractions for deploying AYB to managed providers including Fly.io, DigitalOcean, and Railway.
package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	deployProviderFly          = "fly"
	deployProviderDigitalOcean = "digitalocean"
	deployProviderRailway      = "railway"
	deployProviderTokenUnknown = "unknown"
)

const (
	// Options for Fly.io provider
	flyOptionAppName    = "app_name"
	flyOptionImage      = "image"
	flyOptionVMSize     = "vm_size"
	flyOptionVolumeSize = "volume_size"
)

type DeployProvider interface {
	Name() string
	Validate(cfg DeployConfig) error
	Deploy(ctx context.Context, cfg DeployConfig) (DeployResult, error)
}

type DeployConfig struct {
	Provider        string
	Token           string
	TokenSource     string
	Domain          string
	Region          string
	Env             map[string]string
	PostgresURL     string
	ProviderOptions map[string]string
}

type DeployResult struct {
	Provider     string         `json:"provider"`
	AppURL       string         `json:"app_url"`
	DashboardURL string         `json:"dashboard_url"`
	NextSteps    []string       `json:"next_steps"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type deployProviderInfo struct {
	DisplayName string
	EnvVar      string
	BaseAPI     string
	Scopes      string
	Flags       string
}

type deployJSONError struct {
	Provider string `json:"provider"`
	Error    string `json:"error"`
}

var deployProviderRegistry = map[string]DeployProvider{}

var deployProviderInfos = map[string]deployProviderInfo{
	deployProviderFly: {
		DisplayName: "Fly.io",
		EnvVar:      "FLY_API_TOKEN",
		BaseAPI:     "https://api.machines.dev",
		Scopes:      "app:write, machine:write, token",
		Flags:       "--token",
	},
	deployProviderDigitalOcean: {
		DisplayName: "DigitalOcean",
		EnvVar:      "DIGITALOCEAN_ACCESS_TOKEN",
		BaseAPI:     "https://api.digitalocean.com/v2",
		Scopes:      "droplet:create, droplet:read, droplet:delete, firewall:create, block_storage:create",
		Flags:       "--token",
	},
	deployProviderRailway: {
		DisplayName: "Railway",
		EnvVar:      "RAILWAY_API_TOKEN",
		BaseAPI:     "https://backboard.railway.app/graphql/v2",
		Scopes:      "full access to project/team resources",
		Flags:       "--token",
	},
}

func init() {
	if err := registerDeployProvider(deployProviderFly, flyProvider{}); err != nil {
		panic(err)
	}
	if err := registerDeployProvider(deployProviderDigitalOcean, digitalOceanProvider{}); err != nil {
		panic(err)
	}
	if err := registerDeployProvider(deployProviderRailway, railwayProvider{}); err != nil {
		panic(err)
	}

	deployCmd.PersistentFlags().String("domain", "", "Public domain for the deployment")
	deployCmd.PersistentFlags().String("region", "", "Cloud region for the deployment")
	deployCmd.PersistentFlags().StringArray("env", nil, "Environment variable key=value pairs")
	deployCmd.PersistentFlags().String("postgres-url", "", "PostgreSQL URL to connect to")

	deployFlyCmd.Flags().String("token", "", "Fly API token (or set FLY_API_TOKEN)")
	deployFlyCmd.Flags().String("image", "", "OCI image to deploy (required)")
	deployFlyCmd.Flags().String("app-name", "", "Fly app name (defaults to derived domain name)")
	deployFlyCmd.Flags().String("vm-size", "", "Fly VM size (e.g. shared-cpu-1x, performance-1x)")
	deployFlyCmd.Flags().Int("volume-size", flyDefaultVolumeSize, "Size of app volume in GB")

	deployDigitalOceanCmd.Flags().String("token", "", "DigitalOcean API token (or set DIGITALOCEAN_ACCESS_TOKEN)")
	deployDigitalOceanCmd.Flags().String("binary-url", "", "URL to AYB binary tarball (required)")
	deployDigitalOceanCmd.Flags().String("droplet-size", "s-1vcpu-1gb", "DigitalOcean droplet size")
	deployDigitalOceanCmd.Flags().String("droplet-image", "ubuntu-22-04-x64", "DigitalOcean droplet image")
	deployDigitalOceanCmd.Flags().StringSlice("ssh-key-id", []string{}, "IDs of SSH keys to be added to the droplet (repeatable)")
	deployDigitalOceanCmd.Flags().String("firewall-name", "", "Name of firewall rule (defaults to derived name)")

	deployRailwayCmd.Flags().String("token", "", "Railway API token (or set RAILWAY_API_TOKEN)")
	deployRailwayCmd.Flags().String("image", "", "OCI image to deploy (required)")
	deployRailwayCmd.Flags().String("project-name", "", "Railway project name (defaults to derived name)")
	deployRailwayCmd.Flags().String("service-name", "ayb", "Railway service name")

	deployCmd.AddCommand(deployFlyCmd)
	deployCmd.AddCommand(deployDigitalOceanCmd)
	deployCmd.AddCommand(deployRailwayCmd)
}

var deployCmd = &cobra.Command{
	Use:     "deploy",
	Short:   "Deploy AYB to a cloud provider",
	GroupID: groupCore,
	// Grouping assigned in initHelp via command registration map.
	Long: `Deploy AYB to a managed deployment target.

Examples:
  ayb deploy fly --domain app.example.com --env APP_ENV=production
  ayb deploy digitalocean --region nyc1 --postgres-url postgresql://... \
    --env APP_ENV=production
  ayb deploy railway --region global --env APP_ENV=production`,
}

var deployFlyCmd = &cobra.Command{
	Use:   deployProviderFly,
	Short: "Deploy AYB using Fly.io",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDeployWithProvider(cmd, args, deployProviderFly)
	},
}

var deployDigitalOceanCmd = &cobra.Command{
	Use:   deployProviderDigitalOcean,
	Short: "Deploy AYB using DigitalOcean",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDeployWithProvider(cmd, args, deployProviderDigitalOcean)
	},
}

var deployRailwayCmd = &cobra.Command{
	Use:   deployProviderRailway,
	Short: "Deploy AYB using Railway",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDeployWithProvider(cmd, args, deployProviderRailway)
	},
}

func registerDeployProvider(name string, provider DeployProvider) error {
	providerName := normalizeDeployProviderName(name)
	if providerName == "" {
		return fmt.Errorf("deploy provider name is required")
	}
	if provider == nil {
		return fmt.Errorf("deploy provider %q cannot be nil", providerName)
	}
	if _, exists := deployProviderRegistry[providerName]; exists {
		return fmt.Errorf("deploy provider %q already registered", providerName)
	}
	deployProviderRegistry[providerName] = provider
	return nil
}

func resolveDeployProvider(name string) (DeployProvider, error) {
	providerName := normalizeDeployProviderName(name)
	if providerName == "" {
		return nil, errors.New("deploy provider is required")
	}
	provider, ok := deployProviderRegistry[providerName]
	if !ok {
		supported := []string{deployProviderFly, deployProviderDigitalOcean, deployProviderRailway}
		return nil, fmt.Errorf("unknown deploy provider %q; supported providers: %s", providerName, strings.Join(supported, ", "))
	}
	if provider == nil {
		return nil, fmt.Errorf("deploy provider %q is not configured", providerName)
	}
	return provider, nil
}

func normalizeDeployProviderName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// parseDeployEnv parses KEY=VALUE environment variable pairs from command-line arguments, validating that keys are non-empty and contain no spaces, and rejecting duplicates.
func parseDeployEnv(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return map[string]string{}, nil
	}
	parsed := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --env value %q; expected KEY=VALUE", pair)
		}
		key := strings.TrimSpace(parts[0])
		value := parts[1]
		if key == "" {
			return nil, fmt.Errorf("invalid --env value %q; key cannot be empty", pair)
		}
		if strings.Contains(key, " ") {
			return nil, fmt.Errorf("invalid --env key %q; spaces are not allowed", key)
		}
		if _, exists := parsed[key]; exists {
			return nil, fmt.Errorf("duplicate env key %q", key)
		}
		parsed[key] = value
	}
	return parsed, nil
}

// resolveDeployToken obtains a deployment API token from the provided flag or environment variable, returning both the token and its source (flag name or environment variable name).
func resolveDeployToken(cmd *cobra.Command, provider string) (string, string, error) {
	providerName := normalizeDeployProviderName(provider)
	info, ok := deployProviderInfos[providerName]
	if !ok {
		return "", "", fmt.Errorf("unknown deploy provider %q", provider)
	}

	token := strings.TrimSpace(getFlagValue(cmd, "token"))
	if token != "" {
		return token, "flag", nil
	}
	envToken := strings.TrimSpace(os.Getenv(info.EnvVar))
	if envToken != "" {
		return envToken, info.EnvVar, nil
	}

	return "", "", fmt.Errorf("%s deployment requires a token. Set --token or set %s", info.DisplayName, info.EnvVar)
}

func getFlagValue(cmd *cobra.Command, name string) string {
	value, _ := cmd.Flags().GetString(name)
	if value != "" {
		return value
	}
	value, _ = cmd.InheritedFlags().GetString(name)
	return value
}

func getOptionalIntFlag(cmd *cobra.Command, name string) int {
	value, err := cmd.Flags().GetInt(name)
	if err == nil {
		return value
	}
	value, _ = cmd.InheritedFlags().GetInt(name)
	return value
}

// buildDeployConfig constructs a complete DeployConfig by extracting and validating all relevant command flags, environment variables, and provider-specific options.
func buildDeployConfig(cmd *cobra.Command, provider string) (DeployConfig, error) {
	providerName := normalizeDeployProviderName(provider)
	if providerName == "" {
		return DeployConfig{}, errors.New("provider is required")
	}

	domain, _ := cmd.Flags().GetString("domain")
	region, _ := cmd.Flags().GetString("region")
	postgresURL, _ := cmd.Flags().GetString("postgres-url")
	rawEnv, _ := cmd.Flags().GetStringArray("env")
	env, err := parseDeployEnv(rawEnv)
	if err != nil {
		return DeployConfig{}, err
	}
	token, source, err := resolveDeployToken(cmd, providerName)
	if err != nil {
		return DeployConfig{}, err
	}
	providerOptions, err := buildDeployProviderOptions(cmd, providerName)
	if err != nil {
		return DeployConfig{}, err
	}

	return DeployConfig{
		Provider:        providerName,
		Token:           token,
		TokenSource:     source,
		Domain:          strings.TrimSpace(domain),
		Region:          strings.TrimSpace(region),
		PostgresURL:     strings.TrimSpace(postgresURL),
		Env:             env,
		ProviderOptions: providerOptions,
	}, nil
}

// buildDeployProviderOptions extracts provider-specific deployment options from command flags and returns them as a string map keyed by option name.
func buildDeployProviderOptions(cmd *cobra.Command, provider string) (map[string]string, error) {
	providerName := normalizeDeployProviderName(provider)
	switch providerName {
	case deployProviderFly:
		volumeSize := getOptionalIntFlag(cmd, "volume-size")
		return map[string]string{
			flyOptionAppName:    strings.TrimSpace(getFlagValue(cmd, "app-name")),
			flyOptionImage:      strings.TrimSpace(getFlagValue(cmd, "image")),
			flyOptionVMSize:     strings.TrimSpace(getFlagValue(cmd, "vm-size")),
			flyOptionVolumeSize: strconv.Itoa(volumeSize),
		}, nil
	case deployProviderDigitalOcean:
		return map[string]string{
			doOptionDropletSize:  strings.TrimSpace(getFlagValue(cmd, "droplet-size")),
			doOptionDropletImage: strings.TrimSpace(getFlagValue(cmd, "droplet-image")),
			doOptionSSHKeyID:     strings.Join(getStringSliceFlag(cmd, "ssh-key-id"), ","),
			doOptionFirewallName: strings.TrimSpace(getFlagValue(cmd, "firewall-name")),
			doOptionBinaryURL:    strings.TrimSpace(getFlagValue(cmd, "binary-url")),
		}, nil
	case deployProviderRailway:
		return map[string]string{
			railOptionProjectName: strings.TrimSpace(getFlagValue(cmd, "project-name")),
			railOptionServiceName: strings.TrimSpace(getFlagValue(cmd, "service-name")),
			railOptionImage:       strings.TrimSpace(getFlagValue(cmd, "image")),
		}, nil
	default:
		return map[string]string{}, nil
	}
}

func getStringSliceFlag(cmd *cobra.Command, name string) []string {
	values, err := cmd.Flags().GetStringSlice(name)
	if err != nil {
		// If there's an error getting the flag from the specific command flags,
		// try to get it from inherited flags
		val, err2 := cmd.InheritedFlags().GetStringSlice(name)
		if err2 != nil {
			return []string{} // Return empty slice on both errors
		}
		return val
	}
	return values
}

// runDeployWithProvider orchestrates the complete deployment workflow by building configuration, validating it with the provider, executing deployment, and outputting the result or error.
func runDeployWithProvider(cmd *cobra.Command, _ []string, provider string) error {
	cfg, err := buildDeployConfig(cmd, provider)
	if err != nil {
		return outputDeployError(cmd, provider, err)
	}

	p, err := resolveDeployProvider(provider)
	if err != nil {
		return outputDeployError(cmd, provider, err)
	}

	if err := p.Validate(cfg); err != nil {
		return outputDeployError(cmd, provider, err)
	}

	result, err := p.Deploy(context.Background(), cfg)
	if err != nil {
		return outputDeployError(cmd, provider, err)
	}

	return outputDeployResult(cmd, result)
}

// outputDeployError formats and outputs a deployment error as JSON or plain text, returning the error or any encoding error that occurred.
func outputDeployError(cmd *cobra.Command, provider string, err error) error {
	if err == nil {
		return nil
	}
	if outputFormat(cmd) == "json" {
		providerLabel := deployProviderTokenUnknown
		if provider != "" {
			providerLabel = normalizeDeployProviderName(provider)
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		if encodeErr := enc.Encode(deployJSONError{Provider: providerLabel, Error: err.Error()}); encodeErr != nil {
			return encodeErr
		}
		return nil
	}
	return err
}

// outputDeployResult formats and outputs a successful deployment result as JSON or as human-readable text with provider details, URLs, and next steps.
func outputDeployResult(cmd *cobra.Command, result DeployResult) error {
	if outputFormat(cmd) == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		return enc.Encode(result)
	}

	if result.Provider != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Provider: %s\n", result.Provider)
	}
	if result.AppURL != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "App URL: %s\n", result.AppURL)
	}
	if result.DashboardURL != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Dashboard URL: %s\n", result.DashboardURL)
	}
	if len(result.NextSteps) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
		for _, step := range result.NextSteps {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", step)
		}
	}
	return nil
}

// deriveAppName derives a provider-safe app name from a domain or creates a fallback name.
func deriveAppName(domain, defaultSuffix string) string {
	if strings.TrimSpace(domain) != "" {
		name := sanitizeFlyAppName(normalizeDomainForAppName(domain))
		if name != "" {
			return name
		}
	}
	if strings.TrimSpace(defaultSuffix) == "" {
		defaultSuffix = "ayb"
	}
	if clean := sanitizeFlyAppName(defaultSuffix); clean != "" {
		defaultSuffix = clean
	}
	return defaultSuffix + randomAlphaNum(4)
}

// randomAlphaNum generates a random alphanumeric string of specified length
func randomAlphaNum(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	if length <= 0 {
		return ""
	}
	b := make([]byte, length)
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return strings.Repeat("0", length)
	}
	for i, v := range raw {
		b[i] = charset[int(v)%len(charset)]
	}
	return string(b)
}

// mergeDeployEnv combines deploy config with additional environment mappings
func mergeDeployEnv(cfg DeployConfig) (map[string]string, error) {
	// Start with the user-provided env vars
	mergedEnv := make(map[string]string)
	for k, v := range cfg.Env {
		mergedEnv[k] = v
	}

	// Add the PostgresURL if provided
	if cfg.PostgresURL != "" {
		mergedEnv["AYB_DATABASE_URL"] = cfg.PostgresURL
	}

	// Generate AYB_AUTH_JWT_SECRET only if not already provided
	if _, hasJWTSecret := mergedEnv["AYB_AUTH_JWT_SECRET"]; !hasJWTSecret {
		jwtSecret, err := generateJWTSecret()
		if err != nil {
			return nil, err
		}
		mergedEnv["AYB_AUTH_JWT_SECRET"] = jwtSecret
	}

	return mergedEnv, nil
}

// generateJWTSecret creates a JWT secret for the application
func generateJWTSecret() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func hasDeployDatabaseConfig(cfg DeployConfig) bool {
	if strings.TrimSpace(cfg.PostgresURL) != "" {
		return true
	}
	return strings.TrimSpace(cfg.Env["AYB_DATABASE_URL"]) != ""
}

func warnMissingDatabaseConfig(cfg DeployConfig) {
	if hasDeployDatabaseConfig(cfg) {
		return
	}
	fmt.Fprintln(os.Stderr, "warning: no --postgres-url provided and AYB_DATABASE_URL not found in --env; AYB requires a database to function")
}

func resolveProviderTimeout(override, fallback time.Duration) time.Duration {
	if override > 0 {
		return override
	}
	return fallback
}
