package cli

import (
	"bytes"
	"context"
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

	// Check if auth is enabled (warn but don't block)
	checkDemoAuth(baseURL, useColor)

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

// ensureDemoServer checks if the AYB server is running. If not, starts it
// via `ayb start` (which backgrounds itself). Returns the base URL,
// whether we started the server (for cleanup), and any error.
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

	startCmd := exec.Command(aybBin, "start")
	startCmd.Env = append(os.Environ(), "AYB_AUTH_ENABLED=true", "AYB_AUTH_JWT_SECRET=devsecret-min-32-chars-long-000000")
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

// checkDemoAuth probes the server to warn if auth is disabled.
func checkDemoAuth(baseURL string, useColor bool) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/api/auth/me")
	if err != nil {
		return
	}
	resp.Body.Close()

	// 401 = auth is enabled (expected). 404 = auth endpoint not found (disabled).
	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "\n  %s %s\n\n",
			yellow("⚠", useColor),
			yellow("Auth appears to be disabled. Demos require auth for registration/login.", useColor))
		fmt.Fprintf(os.Stderr, "  %s\n", dim("Add to ayb.toml:", useColor))
		fmt.Fprintf(os.Stderr, "    [auth]\n    enabled = true\n\n")
		fmt.Fprintf(os.Stderr, "  %s\n\n", dim("Then restart: ayb stop && ayb start", useColor))
	}
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

// resolveDemoAdminToken finds an admin token for the running server.
// Reuses the same logic as `ayb sql`.
func resolveDemoAdminToken(baseURL string) (string, error) {
	// Check env var first
	if token := os.Getenv("AYB_ADMIN_TOKEN"); token != "" {
		return token, nil
	}

	// Try saved admin password from ~/.ayb/admin-token
	tokenPath, pathErr := aybAdminTokenPath()
	if pathErr != nil {
		return "", fmt.Errorf("no admin token: could not resolve home directory: %w", pathErr)
	}

	data, readErr := os.ReadFile(tokenPath)
	if readErr != nil {
		return "", fmt.Errorf("no admin token found.\n\n" +
			"  The server is running but wasn't started by the demo command.\n" +
			"  Stop it and let the demo handle everything:\n\n" +
			"    ayb stop && ayb demo <name>\n\n" +
			"  Or, if using lsof to find orphan processes:\n" +
			"    lsof -ti :8090 | xargs kill && ayb demo <name>")
	}

	password := strings.TrimSpace(string(data))
	t, loginErr := adminLogin(baseURL, password)
	if loginErr != nil {
		return "", fmt.Errorf("admin login failed (saved password may be stale): %w", loginErr)
	}
	return t, nil
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
