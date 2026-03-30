package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ensureDemoServer returns the configured server URL and starts an auth-enabled
// local AYB server when one is not already running.
func ensureDemoServer() (string, bool, error) {
	base := serverURL()
	client := &http.Client{Timeout: 2 * time.Second}

	// Check if already running.
	resp, err := client.Get(base + "/health")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return base, false, nil
		}
	}

	// Not running — auto-start via `ayb start`.
	// cmd.Run() blocks until the parent `ayb start` exits (after readiness).
	aybBin, err := os.Executable()
	if err != nil {
		aybBin = os.Args[0]
	}
	jwtSecret, err := resolveDemoJWTSecret()
	if err != nil {
		return "", false, fmt.Errorf("generating demo auth secret: %w", err)
	}

	startCmd := exec.Command(aybBin, "start")
	startCmd.Env = append(os.Environ(), "AYB_AUTH_ENABLED=true", "AYB_AUTH_JWT_SECRET="+jwtSecret)
	startCmd.Stdout = io.Discard
	var startErr strings.Builder
	startCmd.Stderr = &startErr

	if err := startCmd.Run(); err != nil {
		detail := strings.TrimSpace(startErr.String())
		if detail != "" {
			return "", false, fmt.Errorf("failed to start AYB server:\n  %s", detail)
		}
		return "", false, fmt.Errorf("failed to start AYB server: %w", err)
	}
	return base, true, nil
}

func resolveDemoJWTSecret() (string, error) {
	if secret := strings.TrimSpace(os.Getenv("AYB_AUTH_JWT_SECRET")); secret != "" {
		return secret, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// demoServerPort extracts the port from a server URL, falling back to scheme defaults or the default demo port.
func demoServerPort(baseURL string) string {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return demoDefaultServerPort
	}
	if port := strings.TrimSpace(parsedURL.Port()); port != "" {
		return port
	}
	switch strings.ToLower(strings.TrimSpace(parsedURL.Scheme)) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return demoDefaultServerPort
	}
}

func demoRestartCommand() string {
	return "ayb stop && ayb demo <name>"
}

func demoKillCommand() string {
	return "lsof -ti :8090 | xargs kill && ayb demo <name>"
}

func demoCustomPortNote(baseURL, command string) string {
	port := demoServerPort(baseURL)
	if port == demoDefaultServerPort {
		return ""
	}
	return fmt.Sprintf("\n\n  If the server is using port %s, use instead:\n    %s", port, command)
}

// requireDemoAuthEnabled checks that the running server has auth enabled, returning an actionable error with restart instructions if it does not.
func requireDemoAuthEnabled(baseURL string, useColor bool) error {
	enabled, err := demoAuthEnabled(baseURL)
	if err != nil {
		return fmt.Errorf("checking auth status: %w", err)
	}
	if enabled {
		return nil
	}
	return fmt.Errorf("%s %s\n\n  %s\n    [auth]\n    enabled = true\n\n  %s\n\n    %s%s\n\n  %s",
		yellow("⚠", useColor),
		yellow("The running AYB server has auth disabled. Demos require auth for registration and login.", useColor),
		dim("Enable auth in ayb.toml:", useColor),
		dim("Or stop the running server and let the demo start its own auth-enabled server:", useColor),
		demoRestartCommand(),
		demoCustomPortNote(baseURL, fmt.Sprintf("ayb stop --port %s && ayb demo <name>", demoServerPort(baseURL))),
		dim("Then restart your usual server config after the demo if needed.", useColor),
	)
}

// demoAuthEnabled probes the server to determine whether the public auth
// routes are available. /api/auth/me returns 404 when auth is disabled and
// 401/200 when the route exists.
func demoAuthEnabled(baseURL string) (bool, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/api/auth/me")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusUnauthorized, http.StatusForbidden:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if readErr != nil {
			return false, fmt.Errorf("auth probe returned %d and the response body could not be read: %w", resp.StatusCode, readErr)
		}
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			return false, fmt.Errorf("auth probe returned unexpected status %d", resp.StatusCode)
		}
		return false, fmt.Errorf("auth probe returned unexpected status %d: %s", resp.StatusCode, detail)
	}
}
