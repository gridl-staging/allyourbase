// Package cli Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_5_webhooks_bug_and_dx_polish/allyourbase_dev/internal/cli/start_banner.go.
package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/allyourbase/ayb/internal/config"
)

func logFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".ayb", "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	// Enforce owner-only access even when the directory already existed.
	if err := os.Chmod(dir, 0o700); err != nil {
		return ""
	}
	return filepath.Join(dir, fmt.Sprintf("ayb-%s.log", time.Now().Format("20060102")))
}

// cleanOldLogs removes log files older than 7 days.
func cleanOldLogs() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".ayb", "logs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -7)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// multiHandler fans out log records to multiple handlers.
type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// TODO: Document newLogger.
func newLogger(level, format string) (*slog.Logger, *slog.LevelVar, string, func()) {
	var lvlVar slog.LevelVar
	lvlVar.Set(parseSlogLevel(level))

	opts := &slog.HandlerOptions{Level: &lvlVar}

	var stderrHandler slog.Handler
	if format == "text" {
		stderrHandler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		stderrHandler = slog.NewJSONHandler(os.Stderr, opts)
	}

	// Try to open a log file for detailed output.
	logPath := logFilePath()
	if logPath == "" {
		return slog.New(stderrHandler), &lvlVar, "", func() {}
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return slog.New(stderrHandler), &lvlVar, "", func() {}
	}
	// Refuse file logging if we cannot keep the logfile private to the current user.
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return slog.New(stderrHandler), &lvlVar, "", func() {}
	}

	fileOpts := &slog.HandlerOptions{Level: slog.LevelDebug}
	fileHandler := slog.NewJSONHandler(f, fileOpts)

	handler := &multiHandler{handlers: []slog.Handler{stderrHandler, fileHandler}}

	// Clean old logs in the background.
	go cleanOldLogs()

	return slog.New(handler), &lvlVar, logPath, func() { f.Close() }
}

func parseSlogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// startupProgress provides human-readable startup steps for interactive terminals.
// In TTY mode it shows animated spinners; in non-TTY mode all methods are no-ops.
type startupProgress struct {
	w        io.Writer
	spinner  *ui.StepSpinner
	active   bool
	useColor bool
}

func newStartupProgress(w io.Writer, active bool, useColor bool) *startupProgress {
	return &startupProgress{
		w:        w,
		spinner:  ui.NewStepSpinner(w, !active),
		active:   active,
		useColor: useColor,
	}
}

func (sp *startupProgress) header(version string) {
	if !sp.active {
		return
	}
	fmt.Fprintf(sp.w, "\n  %s %s\n\n",
		ui.BrandEmoji,
		boldCyan(fmt.Sprintf("Allyourbase v%s", version), sp.useColor))
}

func (sp *startupProgress) step(msg string) {
	if !sp.active {
		return
	}
	sp.spinner.Start(msg)
}

func (sp *startupProgress) done() {
	if !sp.active {
		return
	}
	sp.spinner.Done()
}

func (sp *startupProgress) fail() {
	if !sp.active {
		return
	}
	sp.spinner.Fail()
}

// printBanner writes a human-readable startup summary to stderr.
// This is separate from structured logging and designed for first-time users.
func printBanner(cfg *config.Config, embeddedPG bool, generatedPassword, logPath string) {
	printBannerTo(os.Stderr, cfg, embeddedPG, colorEnabled(), generatedPassword, logPath)
}

// printBannerTo writes the full banner (header + body) to w. Extracted for testing.
func printBannerTo(w io.Writer, cfg *config.Config, embeddedPG bool, useColor bool, generatedPassword, logPath string) {
	ver := bannerVersion(buildVersion)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", ui.BrandEmoji,
		boldCyan(fmt.Sprintf("Allyourbase v%s", ver), useColor))
	printBannerBodyTo(w, cfg, embeddedPG, useColor, generatedPassword, logPath)
}

// printBannerBodyTo writes the banner body (URLs, hints, demos) to w.
func printBannerBodyTo(w io.Writer, cfg *config.Config, embeddedPG bool, useColor bool, generatedPassword, logPath string) {
	apiURL := cfg.PublicBaseURL() + "/api"

	dbMode := "external"
	if embeddedPG {
		dbMode = "managed"
	}

	// Pad labels before colorizing so ANSI codes don't break alignment.
	padLabel := func(label string, width int) string {
		return bold(fmt.Sprintf("%-*s", width, label), useColor)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", padLabel("API:", 10), cyan(apiURL, useColor))
	if cfg.Admin.Enabled {
		adminURL := cfg.PublicBaseURL() + cfg.Admin.Path
		fmt.Fprintf(w, "  %s %s\n", padLabel("Admin:", 10), cyan(adminURL, useColor))
	}
	fmt.Fprintf(w, "  %s %s\n", padLabel("Database:", 10), dbMode)
	if cfg.Auth.MinPasswordLength > 0 && cfg.Auth.MinPasswordLength < 8 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s\n", yellow(fmt.Sprintf(
			"WARNING: min_password_length is %d (recommended: 8+). Not suitable for production.",
			cfg.Auth.MinPasswordLength), useColor))
	}
	if cfg.Admin.Enabled && generatedPassword != "" {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s  %s\n", bold("Admin password:", useColor), boldGreen(generatedPassword, useColor))
		// This nudge is only for first-run generated credentials; we do not store
		// the plaintext password anywhere users can retrieve later.
		fmt.Fprintf(w, "  %s\n", dim("Save this password now; it won't be shown again.", useColor))
		fmt.Fprintf(w, "  %s\n", dim("To reset: ayb admin reset-password", useColor))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", padLabel("Docs:", 10), dim("https://allyourbase.io/guide/quickstart", useColor))
	if logPath != "" {
		fmt.Fprintf(w, "  %s %s\n", padLabel("Logs:", 10), dim(logPath, useColor))
	}

	// Print next-step hints for new users (no leading whitespace for easy copy-paste).
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", dim("Try:", useColor))
	fmt.Fprintf(w, "%s\n", green(`ayb sql "CREATE TABLE posts (id serial PRIMARY KEY, title text)"`, useColor))
	fmt.Fprintf(w, "%s\n", green("ayb schema", useColor))
	fmt.Fprintln(w)

	// Demo hints — show new users how to run the bundled demo apps.
	fmt.Fprintf(w, "  %s\n", dim("Demos:", useColor))
	fmt.Fprintf(w, "%s  %s\n", green("ayb demo kanban    ", useColor), dim("# Trello-lite kanban board  (port 5173)", useColor))
	fmt.Fprintf(w, "%s  %s\n", green("ayb demo live-polls", useColor), dim("# real-time polling app     (port 5175)", useColor))
	fmt.Fprintln(w)
}

// bannerVersion extracts a clean semver string for the startup banner.
// Release builds (e.g. "v0.1.0") → "0.1.0".
// Dev builds (e.g. "v0.1.0-43-ge534c04-dirty") → "0.1.0-dev".
// Full version is always available via `ayb version`.
func bannerVersion(raw string) string {
	v := strings.TrimPrefix(raw, "v")
	// A bare semver tag (e.g. "0.1.0") has no hyphen after the patch number,
	// or has a pre-release label like "0.1.0-beta.1". Git-describe appends
	// "-<N>-g<hash>" when commits exist past the tag. Detect that pattern.
	parts := strings.SplitN(v, "-", 2)
	if len(parts) == 1 {
		return v // clean tag, e.g. "0.1.0"
	}
	// If the first segment after the hyphen is a number, it's a git-describe
	// commit count (e.g. "0.1.0-43-ge534c04"), not a semver pre-release.
	if len(parts[1]) > 0 && parts[1][0] >= '0' && parts[1][0] <= '9' {
		return parts[0] + "-dev"
	}
	return v // pre-release tag like "0.1.0-beta.1"
}
