// Package cli demo.go implements the ayb demo command, which runs bundled demo applications with built-in server startup, schema application, and user account seeding.
package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/examples"
	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/spf13/cobra"
)

type seedAccount struct {
	Email    string
	Password string
}

// demoSeedUsers are pre-created accounts so users can log in instantly.
var demoSeedUsers = []seedAccount{
	{Email: "alice@demo.test", Password: "password123"},
	{Email: "bob@demo.test", Password: "password123"},
	{Email: "charlie@demo.test", Password: "password123"},
}

type demoInfo struct {
	Name        string
	Title       string
	Description string
	Port        int
	TrySteps    []string
}

var demoRegistry = map[string]demoInfo{
	"kanban": {
		Name:        "kanban",
		Title:       "Kanban Board",
		Description: "Trello-lite with drag-and-drop, auth, and realtime sync",
		Port:        5173,
		TrySteps: []string{
			"Open http://localhost:5173",
			"Sign in with a demo account (shown on the login page)",
			"Create a board and add some cards",
			"Open a second browser tab to see realtime sync",
		},
	},
	"live-polls": {
		Name:        "live-polls",
		Title:       "Live Polls",
		Description: "Slido-lite — real-time polling with voting and bar charts",
		Port:        5175,
		TrySteps: []string{
			"Open http://localhost:5175",
			"Sign in with a demo account (shown on the login page)",
			"Create a poll with a few options",
			"Open a second browser, sign in as another user, and vote — watch results update live",
		},
	},
}

var demoCmd = &cobra.Command{
	Use:   "demo <name>",
	Short: "Run a demo app (one command, batteries included)",
	Long: `Run one of the bundled AYB demo applications.

Available demos:
  kanban        Trello-lite Kanban board with drag-and-drop    (port 5173)
  live-polls    Slido-lite real-time polling app                (port 5175)

The command handles everything:
  - Starts the AYB server if not already running
  - Applies the database schema
  - Serves the pre-built demo app (no Node.js required)

Examples:
  ayb demo kanban
  ayb demo live-polls`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"kanban", "live-polls"},
	RunE:      runDemo,
}

// runDemo runs a bundled demo application. It starts the AYB server if needed, applies the database schema, seeds demo user accounts, and serves the pre-built frontend with API reverse-proxying.
func runDemo(cmd *cobra.Command, args []string) error {
	name := args[0]
	demo, ok := demoRegistry[name]
	if !ok {
		names := make([]string, 0, len(demoRegistry))
		for k := range demoRegistry {
			names = append(names, k)
		}
		return fmt.Errorf("unknown demo %q (available: %s)", name, strings.Join(names, ", "))
	}

	useColor := colorEnabled()
	isTTY := ui.StderrIsTTY()
	sp := ui.NewStepSpinner(os.Stderr, !isTTY)

	// Header
	fmt.Fprintf(os.Stderr, "\n  %s %s\n\n",
		ui.BrandEmoji,
		boldCyan(fmt.Sprintf("Allyourbase Demo: %s", demo.Title), useColor))

	// Step 1: Ensure AYB server is running
	sp.Start("Connecting to AYB server...")
	baseURL, weStarted, err := ensureDemoServer()
	if err != nil {
		sp.Fail()
		return err
	}
	sp.Done()

	// Clean up server on exit if we started it.
	if weStarted {
		aybBin, _ := os.Executable()
		defer exec.Command(aybBin, "stop").Run() //nolint:errcheck
	}

	// Demos depend on the public auth routes for registration and login.
	// If a user already has an auth-disabled server running, fail before we
	// mutate schema or attempt seed-user creation.
	if err := requireDemoAuthEnabled(baseURL, useColor); err != nil {
		return err
	}

	// Step 2: Apply schema
	sp.Start("Applying database schema...")
	schemaResult, err := applyDemoSchema(baseURL, name)
	if err != nil {
		sp.Fail()
		return fmt.Errorf("applying schema: %w", err)
	}
	sp.Done()
	if schemaResult == "exists" {
		fmt.Fprintf(os.Stderr, "  %s\n", dim("Schema already applied (tables exist)", useColor))
	}

	// Step 3: Seed demo users
	sp.Start("Creating demo accounts...")
	if err := seedDemoUsers(baseURL); err != nil {
		sp.Fail()
		return fmt.Errorf("seeding demo users: %w", err)
	}
	sp.Done()

	// Step 4: Print banner
	fmt.Fprintln(os.Stderr)
	padLabel := func(label string, width int) string {
		return bold(fmt.Sprintf("%-*s", width, label), useColor)
	}
	fmt.Fprintf(os.Stderr, "  %s %s\n", padLabel("Demo:", 10), demo.Description)
	fmt.Fprintf(os.Stderr, "  %s %s\n", padLabel("App:", 10), cyan(fmt.Sprintf("http://localhost:%d", demo.Port), useColor))
	fmt.Fprintf(os.Stderr, "  %s %s\n", padLabel("API:", 10), cyan(baseURL+"/api", useColor))
	fmt.Fprintf(os.Stderr, "  %s %s\n", padLabel("Admin:", 10), cyan(baseURL+"/admin", useColor))

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  %s\n", bold("Accounts:", useColor))
	for _, u := range demoSeedUsers {
		fmt.Fprintf(os.Stderr, "    %s  %s %s\n",
			cyan(fmt.Sprintf("%-22s", u.Email), useColor),
			dim("/", useColor),
			green(u.Password, useColor))
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  %s\n", dim("Try:", useColor))
	for i, step := range demo.TrySteps {
		fmt.Fprintf(os.Stderr, "  %s %s\n", dim(fmt.Sprintf("%d.", i+1), useColor), step)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  %s\n\n", dim("Press Ctrl+C to stop.", useColor))

	// Step 5: Serve the pre-built demo app
	return serveDemoApp(name, demo.Port, baseURL)
}

// TODO: Document ensureDemoServer.
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

// requireDemoAuthEnabled ensures the connected server exposes the auth routes
// that demos rely on for registration and login.
func requireDemoAuthEnabled(baseURL string, useColor bool) error {
	enabled, err := demoAuthEnabled(baseURL)
	if err != nil {
		return fmt.Errorf("checking auth status: %w", err)
	}
	if enabled {
		return nil
	}
	return fmt.Errorf("%s %s\n\n  %s\n    [auth]\n    enabled = true\n\n  %s\n\n    ayb stop && ayb demo <name>\n\n  %s",
		yellow("⚠", useColor),
		yellow("The running AYB server has auth disabled. Demos require auth for registration and login.", useColor),
		dim("Enable auth in ayb.toml:", useColor),
		dim("Or stop the running server and let the demo start its own auth-enabled server:", useColor),
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
	resp.Body.Close()
	return resp.StatusCode != http.StatusNotFound, nil
}

// applyDemoSchema reads schema.sql from the embedded FS and sends it to the running server.
// Returns "applied", "exists", or an error.
func applyDemoSchema(baseURL, name string) (string, error) {
	schemaSQL, err := fs.ReadFile(examples.FS, name+"/schema.sql")
	if err != nil {
		return "", fmt.Errorf("reading embedded schema.sql: %w", err)
	}

	token, err := resolveDemoAdminToken(baseURL)
	if err != nil {
		return "", fmt.Errorf("authenticating with server: %w", err)
	}

	body, err := json.Marshal(map[string]string{"query": string(schemaSQL)})
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}
	req, err := http.NewRequest("POST", baseURL+"/api/admin/sql/", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending schema to server: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading server response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		bodyStr := string(respBody)
		// "already exists" is fine — schema was previously applied
		if strings.Contains(bodyStr, "already exists") {
			return "exists", nil
		}
		// Parse error message if possible
		var errResp map[string]any
		if json.Unmarshal(respBody, &errResp) == nil {
			if msg, ok := errResp["message"].(string); ok {
				if strings.Contains(msg, "already exists") {
					return "exists", nil
				}
				return "", fmt.Errorf("SQL error: %s", msg)
			}
		}
		return "", fmt.Errorf("server returned %d: %s", resp.StatusCode, bodyStr)
	}

	return "applied", nil
}

// TODO: Document resolveDemoAdminToken.
func resolveDemoAdminToken(baseURL string) (string, error) {
	if token := resolveCLIAdminToken("", baseURL); token != "" {
		return token, nil
	}

	tokenPath, saved, err := readSavedAdminTokenFile()
	if err != nil {
		if tokenPath == "" {
			return "", fmt.Errorf("no admin token: could not resolve home directory: %w", err)
		}
		return "", fmt.Errorf("no admin token found.\n\n" +
			"  The server is running but wasn't started by the demo command.\n" +
			"  Stop it and let the demo handle everything:\n\n" +
			"    ayb stop && ayb demo <name>\n\n" +
			"  Or, if using lsof to find orphan processes:\n" +
			"    lsof -ti :8090 | xargs kill && ayb demo <name>")
	}

	if saved == "" {
		return "", fmt.Errorf("admin token file is empty: %s", tokenPath)
	}
	return exchangeSavedAdminAuth(baseURL, saved), nil
}

// seedDemoUsers registers the seed accounts via the auth API.
// Ignores 409 Conflict (user already exists).
func seedDemoUsers(baseURL string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	for _, u := range demoSeedUsers {
		body, err := json.Marshal(map[string]string{"email": u.Email, "password": u.Password})
		if err != nil {
			return err
		}
		resp, err := client.Post(baseURL+"/api/auth/register", "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("registering %s: %w", u.Email, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
			return fmt.Errorf("registering %s: unexpected status %d", u.Email, resp.StatusCode)
		}
	}
	return nil
}

// serveDemoApp starts a Go HTTP server that serves pre-built static assets
// from the embedded FS and reverse-proxies /api requests to the AYB server.
// Blocks until SIGINT/SIGTERM is received.
func serveDemoApp(name string, port int, aybServerURL string) error {
	distFS, err := examples.DemoDist(name)
	if err != nil {
		return fmt.Errorf("loading demo assets: %w", err)
	}

	target, err := url.Parse(aybServerURL)
	if err != nil {
		return fmt.Errorf("parsing server URL: %w", err)
	}

	mux := http.NewServeMux()

	// Reverse-proxy /api to the AYB server.
	// FlushInterval: -1 enables continuous flushing, required for SSE (Server-Sent Events).
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.SetXForwarded()
		},
		FlushInterval: -1,
	}
	mux.Handle("/api/", proxy)

	// Serve pre-built static files with SPA fallback.
	mux.HandleFunc("/", demoFileHandler(distFS))

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Graceful shutdown on signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		signal.Stop(sigCh)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("demo server: %w", err)
	}
	return nil
}

// demoFileHandler returns an http.HandlerFunc that serves files from the given
// FS with SPA index.html fallback for client-side routing.
func demoFileHandler(distFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Clean the path and strip leading slash.
		path := strings.TrimPrefix(r.URL.Path, "/")

		// Try to serve the exact file; fall back to index.html for SPA routing.
		if path == "" || !serveDemoFile(w, distFS, path) {
			serveDemoFile(w, distFS, "index.html")
		}
	}
}

// serveDemoFile writes a file from the demo dist FS to w.
// Returns false if the file doesn't exist (caller should fall back).
func serveDemoFile(w http.ResponseWriter, distFS fs.FS, path string) bool {
	f, err := distFS.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return false
	}

	// Cache static assets (not index.html).
	if path != "index.html" {
		w.Header().Set("Cache-Control", "public, max-age=1209600")
	}
	ct := mime.TypeByExtension(filepath.Ext(path))
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, f)
	return true
}
