package cli

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
)

// --- portError ---

func TestPortErrorAddressInUse(t *testing.T) {
	err := portError(8090, fmt.Errorf("listen tcp :8090: bind: address already in use"))
	testutil.NotNil(t, err)

	msg := err.Error()
	testutil.Contains(t, msg, "port 8090 is already in use")
	testutil.Contains(t, msg, "Try:")
	testutil.Contains(t, msg, "--port 8091")
	testutil.Contains(t, msg, "ayb stop")
}

func TestPortErrorSuggestsNextPort(t *testing.T) {
	err := portError(3000, fmt.Errorf("address already in use"))
	msg := err.Error()
	testutil.Contains(t, msg, "--port 3001")
}

// --- startupProgress ---

func TestStartupProgressHeader(t *testing.T) {
	var buf bytes.Buffer
	sp := newStartupProgress(&buf, true, false)
	sp.header("0.2.0")

	out := buf.String()
	testutil.Contains(t, out, "Allyourbase v0.2.0")
	testutil.Contains(t, out, "👾")
}

func TestStartupProgressInactiveIsNoop(t *testing.T) {
	var buf bytes.Buffer
	sp := newStartupProgress(&buf, false, false)
	sp.header("0.2.0")
	sp.step("Connecting...")
	sp.done()
	sp.fail()

	testutil.Equal(t, "", buf.String())
}

func TestStartupProgressStepDone(t *testing.T) {
	var buf bytes.Buffer
	sp := newStartupProgress(&buf, true, false)
	sp.step("Loading schema...")
	sp.done()

	out := buf.String()
	testutil.Contains(t, out, "Loading schema...")
	testutil.Contains(t, out, "✓")
}

func TestStartupProgressStepFail(t *testing.T) {
	var buf bytes.Buffer
	sp := newStartupProgress(&buf, true, false)
	sp.step("Starting server...")
	sp.fail()

	out := buf.String()
	testutil.Contains(t, out, "Starting server...")
	testutil.Contains(t, out, "✗")
}

// --- logFilePath ---

func TestLogFilePathFormat(t *testing.T) {
	// Set HOME to a known temp dir so the test never skips trivially.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	p := logFilePath()
	if p == "" {
		t.Fatal("logFilePath returned empty even with HOME set")
	}
	testutil.Contains(t, p, ".ayb/logs/ayb-")
	testutil.Contains(t, p, ".log")
	// Should contain today's date in YYYYMMDD format.
	today := time.Now().Format("20060102")
	testutil.Contains(t, p, today)
}

// --- cleanOldLogs ---

func TestCleanOldLogsRemovesStale(t *testing.T) {
	tmpDir := t.TempDir()
	logsDir := filepath.Join(tmpDir, ".ayb", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create an old log file (modification time 10 days ago).
	oldFile := filepath.Join(logsDir, "ayb-20260101.log")
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().AddDate(0, 0, -10)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Create a recent log file.
	newFile := filepath.Join(logsDir, "ayb-20260218.log")
	if err := os.WriteFile(newFile, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Override HOME so cleanOldLogs uses our temp dir.
	t.Setenv("HOME", tmpDir)
	cleanOldLogs()

	// Old file should be removed.
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("expected old log file to be removed")
	}
	// New file should remain.
	if _, err := os.Stat(newFile); err != nil {
		t.Error("expected recent log file to remain")
	}
}

func TestCleanOldLogsNoDir(t *testing.T) {
	// Should not panic when the logs directory doesn't exist.
	t.Setenv("HOME", t.TempDir())
	cleanOldLogs() // no-op, should not panic
}

// --- newLogger ---

func TestNewLoggerReturnsComponents(t *testing.T) {
	logger, lvl, logPath, closer := newLogger("info", "json")
	defer closer()

	testutil.NotNil(t, logger)
	testutil.NotNil(t, lvl)
	// logPath may be empty if HOME is weird, but if present should have .log extension.
	if logPath != "" {
		testutil.Contains(t, logPath, ".log")
	}
}

func TestNewLoggerTextFormat(t *testing.T) {
	logger, _, _, closer := newLogger("info", "text")
	defer closer()
	testutil.NotNil(t, logger)
}

func TestNewLoggerLevelAdjustable(t *testing.T) {
	_, lvl, _, closer := newLogger("info", "json")
	defer closer()

	lvl.Set(slog.LevelWarn)
	testutil.Equal(t, slog.LevelWarn, lvl.Level())
}

// --- Banner body-only path ---

func TestBannerBodyToContainsAPIURL(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultTestConfig()
	printBannerBodyTo(&buf, cfg, false, false, "", "")

	out := buf.String()
	testutil.Contains(t, out, "http://localhost:8090/api")
	// Body only should NOT contain the version header.
	testutil.False(t, strings.Contains(out, "Allyourbase v"))
}

func TestBannerBodyUsesHTTPSWhenTLSEnabled(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultTestConfig()
	cfg.Server.TLSEnabled = true
	cfg.Server.TLSDomain = "api.example.com"
	printBannerBodyTo(&buf, cfg, false, false, "", "")

	out := buf.String()
	// API and admin must use https:// + domain, not http:// + host:port.
	testutil.Contains(t, out, "https://api.example.com/api")
	testutil.Contains(t, out, "https://api.example.com/admin")
	testutil.False(t, strings.Contains(out, "http://"), "body banner must not contain http:// when TLS is enabled")
	// Must not fall back to localhost:port format.
	testutil.False(t, strings.Contains(out, "localhost:8090"), "body banner must not show host:port when TLS is enabled")
}

// --- --domain flag registration ---

func TestDomainFlagIsRegistered(t *testing.T) {
	f := startCmd.Flags().Lookup("domain")
	if f == nil {
		t.Fatal("--domain flag not registered on startCmd")
	}
	testutil.Equal(t, "string", f.Value.Type())
	testutil.Equal(t, "", f.DefValue) // default is empty string (TLS opt-in)
}

// --- buildChildArgs ---

func TestBuildChildArgs(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"ayb", "start", "--port", "9000", "--host", "127.0.0.1"}
	args := buildChildArgs()

	joined := strings.Join(args, " ")
	testutil.Contains(t, joined, "--port")
	testutil.Contains(t, joined, "9000")
	testutil.Contains(t, joined, "--foreground")
}

func TestBuildChildArgsNoDoubleForeground(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"ayb", "start", "--foreground", "--port", "9000"}
	args := buildChildArgs()

	count := 0
	for _, a := range args {
		if a == "--foreground" {
			count++
		}
	}
	testutil.Equal(t, 1, count)
}

func TestBuildChildArgsStripsExistingForeground(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"ayb", "start", "--foreground"}
	args := buildChildArgs()

	// Should contain "start" and "--foreground" but only once.
	testutil.Equal(t, 2, len(args)) // "start", "--foreground"
}

// --- cleanupServerFiles ---

func TestCleanupServerFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	aybDir := filepath.Join(tmpDir, ".ayb")
	testutil.NoError(t, os.MkdirAll(aybDir, 0o755))

	pidFile := filepath.Join(aybDir, "ayb.pid")
	tokenFile := filepath.Join(aybDir, "admin-token")
	testutil.NoError(t, os.WriteFile(pidFile, []byte("12345\n8090"), 0o644))
	testutil.NoError(t, os.WriteFile(tokenFile, []byte("secret"), 0o600))

	cleanupServerFiles()

	_, err1 := os.Stat(pidFile)
	_, err2 := os.Stat(tokenFile)
	testutil.True(t, os.IsNotExist(err1))
	testutil.True(t, os.IsNotExist(err2))
}

// --- isFirstRun ---

func TestIsFirstRunEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	testutil.True(t, isFirstRun())
}

func TestIsFirstRunWithCache(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cacheDir := filepath.Join(tmpDir, ".ayb", "pg")
	testutil.NoError(t, os.MkdirAll(cacheDir, 0o755))
	testutil.NoError(t, os.WriteFile(filepath.Join(cacheDir, "postgres.txz"), []byte("cached"), 0o644))
	testutil.False(t, isFirstRun())
}

// --- buildChildArgs edge cases ---

func TestBuildChildArgsForegroundEqualsTrue(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"ayb", "start", "--foreground=true", "--port", "9000"}
	args := buildChildArgs()

	// --foreground=true should be stripped and replaced with --foreground
	count := 0
	for _, a := range args {
		if a == "--foreground" {
			count++
		}
		if strings.HasPrefix(a, "--foreground=") {
			t.Fatalf("--foreground=value should have been stripped, found: %s", a)
		}
	}
	testutil.Equal(t, 1, count)
	// --port 9000 should still be present
	joined := strings.Join(args, " ")
	testutil.Contains(t, joined, "--port")
	testutil.Contains(t, joined, "9000")
}

func TestBuildChildArgsEmptySubcommand(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"ayb", "start"}
	args := buildChildArgs()

	testutil.Equal(t, 2, len(args)) // "start", "--foreground"
	testutil.Equal(t, "start", args[0])
	testutil.Equal(t, "--foreground", args[1])
}

// --- parseSlogLevel ---

func TestParseSlogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},        // default
		{"unknown", slog.LevelInfo}, // unknown → default
		{"DEBUG", slog.LevelInfo},   // case-sensitive, uppercase → default
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSlogLevel(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}

// --- multiHandler ---

func TestMultiHandlerFanOut(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	h1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelDebug})
	h2 := slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelDebug})

	mh := &multiHandler{handlers: []slog.Handler{h1, h2}}
	logger := slog.New(mh)

	logger.Info("test message", "key", "val")

	testutil.Contains(t, buf1.String(), "test message")
	testutil.Contains(t, buf2.String(), "test message")
}

func TestMultiHandlerLevelFiltering(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	// h1 accepts all levels, h2 only WARN+
	h1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelDebug})
	h2 := slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelWarn})

	mh := &multiHandler{handlers: []slog.Handler{h1, h2}}
	logger := slog.New(mh)

	logger.Info("info message")
	logger.Warn("warn message")

	testutil.Contains(t, buf1.String(), "info message")
	testutil.Contains(t, buf1.String(), "warn message")
	// buf2 should NOT have the info message (below warn threshold)
	testutil.False(t, strings.Contains(buf2.String(), "info message"))
	testutil.Contains(t, buf2.String(), "warn message")
}

func TestMultiHandlerWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	mh := &multiHandler{handlers: []slog.Handler{h}}

	// WithAttrs returns a new handler
	mh2 := mh.WithAttrs([]slog.Attr{slog.String("component", "test")})
	logger := slog.New(mh2)
	logger.Info("with attrs")

	testutil.Contains(t, buf.String(), "component")
	testutil.Contains(t, buf.String(), "test")
}

func TestMultiHandlerWithGroup(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	mh := &multiHandler{handlers: []slog.Handler{h}}

	mh2 := mh.WithGroup("mygroup")
	logger := slog.New(mh2)
	logger.Info("grouped", "key", "val")

	testutil.Contains(t, buf.String(), "mygroup")
}

// --- aybPIDPath / aybAdminTokenPath / aybResetResultPath ---

func TestAYBPathFunctions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	pidPath, err := aybPIDPath()
	testutil.Nil(t, err)
	testutil.Contains(t, pidPath, ".ayb/ayb.pid")

	tokenPath, err := aybAdminTokenPath()
	testutil.Nil(t, err)
	testutil.Contains(t, tokenPath, ".ayb/admin-token")

	resetPath, err := aybResetResultPath()
	testutil.Nil(t, err)
	testutil.Contains(t, resetPath, ".ayb/.pw_reset_result")
}

// --- readAYBPID ---

// writePIDFile creates a PID file in a temp .ayb dir. Fails test on error.
func writePIDFile(t *testing.T, content string) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	aybDir := filepath.Join(tmpDir, ".ayb")
	testutil.NoError(t, os.MkdirAll(aybDir, 0o755))
	testutil.NoError(t, os.WriteFile(filepath.Join(aybDir, "ayb.pid"), []byte(content), 0o644))
}

func TestReadAYBPID_ValidTwoLine(t *testing.T) {
	writePIDFile(t, "12345\n8090")

	pid, port, err := readAYBPID()
	testutil.Nil(t, err)
	testutil.Equal(t, 12345, pid)
	testutil.Equal(t, 8090, port)
}

func TestReadAYBPID_SingleLine(t *testing.T) {
	writePIDFile(t, "12345")

	pid, port, err := readAYBPID()
	testutil.Nil(t, err)
	testutil.Equal(t, 12345, pid)
	testutil.Equal(t, 0, port) // old format, no port
}

func TestReadAYBPID_EmptyFile(t *testing.T) {
	writePIDFile(t, "")

	_, _, err := readAYBPID()
	testutil.NotNil(t, err)
}

func TestReadAYBPID_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	_, _, err := readAYBPID()
	testutil.NotNil(t, err)
}

func TestReadAYBPID_MalformedPID(t *testing.T) {
	writePIDFile(t, "notanumber\n8090")

	_, _, err := readAYBPID()
	testutil.NotNil(t, err)
}

func TestReadAYBPID_MalformedPort(t *testing.T) {
	writePIDFile(t, "12345\nnotaport")

	_, _, err := readAYBPID()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "port")
}

func TestReadAYBPID_WhitespaceHandling(t *testing.T) {
	writePIDFile(t, "  12345 \n  8090  \n")

	pid, port, err := readAYBPID()
	testutil.Nil(t, err)
	testutil.Equal(t, 12345, pid)
	testutil.Equal(t, 8090, port)
}

// --- logFilePath ---

func TestLogFilePathCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	p := logFilePath()
	testutil.True(t, p != "")

	// Dir should exist
	dir := filepath.Dir(p)
	info, err := os.Stat(dir)
	testutil.Nil(t, err)
	testutil.True(t, info.IsDir())
}

// --- foreground flag registration ---

func TestForegroundFlagIsHidden(t *testing.T) {
	f := startCmd.Flags().Lookup("foreground")
	if f == nil {
		t.Fatal("--foreground flag not registered")
	}
	testutil.True(t, f.Hidden)
	testutil.Equal(t, "false", f.DefValue)
}

// --- cleanupServerFiles idempotent ---

func TestCleanupServerFilesIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	testutil.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".ayb"), 0o755))

	// Should not panic when files don't exist
	cleanupServerFiles()
	cleanupServerFiles()
}

// --- newLogger with no HOME ---

func TestNewLoggerNoHome(t *testing.T) {
	// Unset HOME to exercise the fallback path
	t.Setenv("HOME", "/nonexistent-path-that-should-not-exist")
	logger, lvl, _, closer := newLogger("info", "json")
	defer closer()

	testutil.NotNil(t, logger)
	testutil.NotNil(t, lvl)
}

// --- portError ---

func TestPortErrorPreservesNonBindError(t *testing.T) {
	orig := fmt.Errorf("connection refused")
	err := portError(8090, orig)
	testutil.Equal(t, orig, err)
}

// --- banner body with log path ---

func TestBannerBodyShowsLogPath(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultTestConfig()
	printBannerBodyTo(&buf, cfg, true, false, "", "/tmp/test.log")

	testutil.Contains(t, buf.String(), "/tmp/test.log")
	testutil.Contains(t, buf.String(), "Logs:")
}

func TestBannerBodyHidesLogPathWhenEmpty(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultTestConfig()
	printBannerBodyTo(&buf, cfg, true, false, "", "")

	testutil.False(t, strings.Contains(buf.String(), "Logs:"))
}

type oauthProviderModeConfigSetterStub struct {
	called bool
	cfg    auth.OAuthProviderModeConfig
}

func (s *oauthProviderModeConfigSetterStub) SetOAuthProviderModeConfig(cfg auth.OAuthProviderModeConfig) {
	s.called = true
	s.cfg = cfg
}

func TestApplyOAuthProviderModeConfig_DisabledDoesNotApply(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.OAuthProviderMode.Enabled = false
	cfg.Auth.OAuthProviderMode.AccessTokenDuration = 1200
	cfg.Auth.OAuthProviderMode.RefreshTokenDuration = 86400
	cfg.Auth.OAuthProviderMode.AuthCodeDuration = 180

	stub := &oauthProviderModeConfigSetterStub{}
	applyOAuthProviderModeConfig(stub, cfg)
	testutil.False(t, stub.called)
}

func TestApplyOAuthProviderModeConfig_EnabledAppliesDurations(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.OAuthProviderMode.Enabled = true
	cfg.Auth.OAuthProviderMode.AccessTokenDuration = 1200
	cfg.Auth.OAuthProviderMode.RefreshTokenDuration = 86400
	cfg.Auth.OAuthProviderMode.AuthCodeDuration = 180

	stub := &oauthProviderModeConfigSetterStub{}
	applyOAuthProviderModeConfig(stub, cfg)

	testutil.True(t, stub.called)
	testutil.Equal(t, 20*time.Minute, stub.cfg.AccessTokenDuration)
	testutil.Equal(t, 24*time.Hour, stub.cfg.RefreshTokenDuration)
	testutil.Equal(t, 3*time.Minute, stub.cfg.AuthCodeDuration)
}

// --- portInUse ---

func TestPortInUseFreePort(t *testing.T) {
	// Allocate a port, close it immediately, then verify it is not in use.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	testutil.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	testutil.False(t, portInUse(port))
}

func TestPortInUseOccupiedPort(t *testing.T) {
	// Bind a port, then check that portInUse detects it.
	ln, err := net.Listen("tcp", ":0")
	testutil.NoError(t, err)
	defer ln.Close()

	// Extract the port number.
	addr := ln.Addr().(*net.TCPAddr)
	testutil.True(t, portInUse(addr.Port))
}

func TestPortInUseAfterRelease(t *testing.T) {
	// Bind and release a port — should report not in use.
	ln, err := net.Listen("tcp", ":0")
	testutil.NoError(t, err)
	addr := ln.Addr().(*net.TCPAddr)
	ln.Close()

	testutil.False(t, portInUse(addr.Port))
}

// --- healthCheckURL ---

func TestHealthCheckURLDefaultPort(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.Port = 0
	got := healthCheckURL(cfg)
	testutil.Equal(t, "http://127.0.0.1:8090/health", got)
}

func TestHealthCheckURLCustomPort(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.Port = 3000
	got := healthCheckURL(cfg)
	testutil.Equal(t, "http://127.0.0.1:3000/health", got)
}

func TestHealthCheckURLWithTLS(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.TLSEnabled = true
	cfg.Server.TLSDomain = "api.example.com"
	got := healthCheckURL(cfg)
	testutil.Equal(t, "https://127.0.0.1:443/health", got)
}

func TestHealthCheckURLTLSIgnoresPort(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.TLSEnabled = true
	cfg.Server.TLSDomain = "api.example.com"
	cfg.Server.Port = 3000
	got := healthCheckURL(cfg)
	// TLS always uses 443, regardless of configured port.
	testutil.Equal(t, "https://127.0.0.1:443/health", got)
}
