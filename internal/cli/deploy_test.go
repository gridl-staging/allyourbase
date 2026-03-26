package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func newDeployConfigCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("domain", "", "")
	cmd.Flags().String("region", "", "")
	cmd.Flags().StringArray("env", nil, "")
	cmd.Flags().String("postgres-url", "", "")
	cmd.Flags().String("token", "", "")
	testutil.NoError(t, cmd.ParseFlags(args))
	return cmd
}

func TestParseDeployEnvValidPairs(t *testing.T) {
	env, err := parseDeployEnv([]string{"APP_ENV=production", "DEBUG=true", "RAW=value=with=equals"})
	testutil.NoError(t, err)
	testutil.Equal(t, "production", env["APP_ENV"])
	testutil.Equal(t, "true", env["DEBUG"])
	testutil.Equal(t, "value=with=equals", env["RAW"])
}

func TestParseDeployEnvInvalidPair(t *testing.T) {
	_, err := parseDeployEnv([]string{"MISSING_EQUALS"})
	testutil.ErrorContains(t, err, "expected KEY=VALUE")
}

func TestParseDeployEnvEmptyKey(t *testing.T) {
	_, err := parseDeployEnv([]string{"=VALUE"})
	testutil.ErrorContains(t, err, "key cannot be empty")
}

func TestParseDeployEnvDuplicateKeys(t *testing.T) {
	_, err := parseDeployEnv([]string{"KEY=one", "KEY=two"})
	testutil.ErrorContains(t, err, "duplicate env key")
}

func TestBuildDeployConfigConstructsFields(t *testing.T) {
	cmd := newDeployConfigCommand(t,
		"--domain", "example.com",
		"--region", "sfo",
		"--env", "APP_ENV=production",
		"--env", "API_PREFIX=v1",
		"--postgres-url", "postgres://source:5432",
		"--token", "flag-token",
	)

	cfg, err := buildDeployConfig(cmd, deployProviderFly)
	testutil.NoError(t, err)
	testutil.Equal(t, "example.com", cfg.Domain)
	testutil.Equal(t, "sfo", cfg.Region)
	testutil.Equal(t, "postgres://source:5432", cfg.PostgresURL)
	testutil.Equal(t, "flag-token", cfg.Token)
	testutil.Equal(t, deployProviderFly, cfg.Provider)
	testutil.Equal(t, "flag", cfg.TokenSource)
	testutil.Equal(t, 2, len(cfg.Env))
	testutil.Equal(t, "production", cfg.Env["APP_ENV"])
	testutil.Equal(t, "v1", cfg.Env["API_PREFIX"])
}

func TestBuildDeployConfigTokenPrecedenceFlagOverEnv(t *testing.T) {
	cmd := newDeployConfigCommand(t,
		"--token", "flag-token",
	)
	t.Setenv("FLY_API_TOKEN", "env-token")

	cfg, err := buildDeployConfig(cmd, deployProviderFly)
	testutil.NoError(t, err)
	testutil.Equal(t, "flag-token", cfg.Token)
	testutil.Equal(t, "flag", cfg.TokenSource)
}

func TestBuildDeployConfigUsesTokenFromEnv(t *testing.T) {
	cmd := newDeployConfigCommand(t)
	t.Setenv("RAILWAY_API_TOKEN", "env-token")

	cfg, err := buildDeployConfig(cmd, deployProviderRailway)
	testutil.NoError(t, err)
	testutil.Equal(t, "env-token", cfg.Token)
	testutil.Equal(t, "RAILWAY_API_TOKEN", cfg.TokenSource)
	testutil.Equal(t, deployProviderRailway, cfg.Provider)
}

func TestBuildDeployConfigInvalidEnvPair(t *testing.T) {
	cmd := newDeployConfigCommand(t,
		"--env", "BADPAIR",
		"--token", "token",
	)

	_, err := buildDeployConfig(cmd, deployProviderFly)
	testutil.ErrorContains(t, err, "expected KEY=VALUE")
}

func TestBuildDeployConfigMissingTokenIncludesGuidance(t *testing.T) {
	cmd := newDeployConfigCommand(t)
	t.Setenv("FLY_API_TOKEN", "")

	_, err := buildDeployConfig(cmd, deployProviderFly)
	testutil.ErrorContains(t, err, "FLY_API_TOKEN")
	testutil.ErrorContains(t, err, "Set --token")
}

func TestResolveDeployProviderKnownAndUnknown(t *testing.T) {
	p, err := resolveDeployProvider(deployProviderRailway)
	testutil.NoError(t, err)
	testutil.Equal(t, deployProviderRailway, p.Name())

	_, err = resolveDeployProvider("unknown-provider")
	testutil.ErrorContains(t, err, "unknown deploy provider")
}

func TestRegisterDeployProviderRejectsNilAndDuplicate(t *testing.T) {
	old := make(map[string]DeployProvider, len(deployProviderRegistry))
	for k, v := range deployProviderRegistry {
		old[k] = v
	}
	t.Cleanup(func() {
		deployProviderRegistry = old
	})

	err := registerDeployProvider("", flyProvider{})
	testutil.ErrorContains(t, err, "name is required")
	err = registerDeployProvider("broken", nil)
	testutil.ErrorContains(t, err, "cannot be nil")
	err = registerDeployProvider(deployProviderFly, flyProvider{})
	testutil.ErrorContains(t, err, "already registered")
}

func TestDeployCommandHelp(t *testing.T) {
	resetJSONFlag()
	rootHelp := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"--help"})
		err := rootCmd.Execute()
		testutil.NoError(t, err)
	})
	testutil.Contains(t, rootHelp, "deploy")

	deployHelp := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"deploy", "--help"})
		err := rootCmd.Execute()
		testutil.NoError(t, err)
	})
	testutil.Contains(t, deployHelp, "fly")
	testutil.Contains(t, deployHelp, "digitalocean")
	testutil.Contains(t, deployHelp, "railway")
}

func TestDeployFlySubcommandRequiresImage(t *testing.T) {
	resetJSONFlag()
	t.Setenv("FLY_API_TOKEN", "token")
	err := func() error {
		rootCmd.SetArgs([]string{"deploy", "fly", "--domain", "app.example.com", "--env", "APP_ENV=production"})
		return rootCmd.Execute()
	}()
	testutil.ErrorContains(t, err, "--image")
	testutil.ErrorContains(t, err, "Build and push")
}

func TestDeployDigitalOceanMissingTokenJSON(t *testing.T) {
	resetJSONFlag()
	t.Setenv("DIGITALOCEAN_ACCESS_TOKEN", "")

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"deploy", "digitalocean", "--json"})
		err := rootCmd.Execute()
		testutil.NoError(t, err)
	})

	var got map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(output), &got))
	testutil.Equal(t, "digitalocean", fmt.Sprint(got["provider"]))
	if _, ok := got["error"]; !ok {
		t.Fatalf("expected JSON error payload, got: %v", got)
	}
}

func TestDeployDigitalOceanSubcommandRequiresBinaryURL(t *testing.T) {
	resetJSONFlag()
	t.Setenv("DIGITALOCEAN_ACCESS_TOKEN", "token")
	err := func() error {
		rootCmd.SetArgs([]string{"deploy", "digitalocean"})
		return rootCmd.Execute()
	}()
	testutil.ErrorContains(t, err, "--binary-url")
}

func TestDeployRailwaySubcommandRequiresImage(t *testing.T) {
	resetJSONFlag()
	t.Setenv("RAILWAY_API_TOKEN", "token")
	err := func() error {
		rootCmd.SetArgs([]string{"deploy", "railway"})
		return rootCmd.Execute()
	}()
	testutil.ErrorContains(t, err, "--image")
}

func TestOutputDeployErrorUsesCommandWriter(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("output", "table", "")
	testutil.NoError(t, cmd.Flags().Set("json", "true"))

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := outputDeployError(cmd, deployProviderFly, fmt.Errorf("boom"))
	testutil.NoError(t, err)

	var got map[string]any
	testutil.NoError(t, json.Unmarshal(out.Bytes(), &got))
	testutil.Equal(t, deployProviderFly, fmt.Sprint(got["provider"]))
	testutil.Equal(t, "boom", fmt.Sprint(got["error"]))
}

func TestOutputDeployResultUsesCommandWriter(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("output", "table", "")

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := outputDeployResult(cmd, DeployResult{
		Provider:     deployProviderFly,
		AppURL:       "https://app.example.com",
		DashboardURL: "https://fly.io/apps/example",
		NextSteps:    []string{"Check logs"},
	})
	testutil.NoError(t, err)

	rendered := out.String()
	testutil.True(t, strings.Contains(rendered, "Provider: fly"))
	testutil.True(t, strings.Contains(rendered, "App URL: https://app.example.com"))
	testutil.True(t, strings.Contains(rendered, "Dashboard URL: https://fly.io/apps/example"))
	testutil.True(t, strings.Contains(rendered, "Next steps:"))
	testutil.True(t, strings.Contains(rendered, "- Check logs"))
}

func TestOutputDeployResultJSONUsesCommandWriter(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("output", "table", "")
	testutil.NoError(t, cmd.Flags().Set("output", "json"))

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := outputDeployResult(cmd, DeployResult{
		Provider: deployProviderRailway,
		AppURL:   "https://railway.app/project/example",
	})
	testutil.NoError(t, err)

	var got map[string]any
	testutil.NoError(t, json.Unmarshal(out.Bytes(), &got))
	testutil.Equal(t, deployProviderRailway, fmt.Sprint(got["provider"]))
	testutil.Equal(t, "https://railway.app/project/example", fmt.Sprint(got["app_url"]))
}

func TestDeployFlyHelpIncludesFlyFlags(t *testing.T) {
	resetJSONFlag()

	help := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"deploy", "fly", "--help"})
		err := rootCmd.Execute()
		testutil.NoError(t, err)
	})

	testutil.Contains(t, help, "--app-name")
	testutil.Contains(t, help, "--image")
	testutil.Contains(t, help, "--vm-size")
	testutil.Contains(t, help, "--volume-size")
}
