package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/caddyserver/certmagic"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/support"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBuildBillingService_DisabledProviderReturnsNoop(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.Provider = ""
	svc := buildBillingService(cfg, nil, testNoopLogger())

	customer, err := svc.CreateCustomer(context.Background(), "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, "tenant-1", customer.TenantID)
}

func TestBuildBillingService_StripeWithoutPoolFallsBackNoop(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.Provider = "stripe"
	cfg.Billing.StripeSecretKey = "sk_test_123"
	cfg.Billing.StripeWebhookSecret = "whsec_123"
	cfg.Billing.StripeStarterPriceID = "price_starter"
	cfg.Billing.StripeProPriceID = "price_pro"
	cfg.Billing.StripeEnterprisePriceID = "price_enterprise"

	svc := buildBillingService(cfg, nil, testNoopLogger())

	checkout, err := svc.CreateCheckoutSession(context.Background(), "tenant-1", "starter", "https://ok", "https://cancel")
	testutil.NoError(t, err)
	testutil.Equal(t, "tenant-1", checkout.TenantID)
}

func TestBuildSupportService_DisabledReturnsNoop(t *testing.T) {
	cfg := &config.Config{}
	cfg.Support.Enabled = false

	svc := buildSupportService(cfg, nil)
	tickets, err := svc.ListTickets(context.Background(), "tenant-1", support.TicketFilters{})
	testutil.NoError(t, err)
	testutil.SliceLen(t, tickets, 0)
}

func TestBuildSupportService_EnabledWithoutPoolFallsBackNoop(t *testing.T) {
	cfg := &config.Config{}
	cfg.Support.Enabled = true

	svc := buildSupportService(cfg, nil)
	tickets, err := svc.ListTickets(context.Background(), "tenant-1", support.TicketFilters{})
	testutil.NoError(t, err)
	testutil.SliceLen(t, tickets, 0)
}

func TestWireBillingUsageSyncJobs_RegisterWhenStripe(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.Provider = "stripe"
	cfg.Billing.UsageSyncIntervalSecs = 3600

	billingSvc := billing.NewNoopBillingService()
	var handlerRegistered, scheduleRegistered bool
	var gotPool *pgxpool.Pool
	var gotInterval int
	var gotBillingSvc billing.BillingService
	var gotHandlerSvc *jobs.Service

	origRegisterHandler := registerBillingUsageSyncHandler
	origRegisterSchedule := registerBillingUsageSyncSchedule
	t.Cleanup(func() {
		registerBillingUsageSyncHandler = origRegisterHandler
		registerBillingUsageSyncSchedule = origRegisterSchedule
	})

	registerBillingUsageSyncHandler = func(svc *jobs.Service, bSvc billing.BillingService, pool *pgxpool.Pool) {
		handlerRegistered = true
		gotBillingSvc = bSvc
		gotHandlerSvc = svc
		gotPool = pool
	}
	registerBillingUsageSyncSchedule = func(_ context.Context, svc *jobs.Service, interval int) error {
		scheduleRegistered = true
		gotInterval = interval
		return nil
	}

	wireBillingUsageSyncJobs(
		context.Background(),
		cfg,
		&jobs.Service{},
		billingSvc,
		&pgxpool.Pool{},
		testNoopLogger(),
	)

	testutil.True(t, handlerRegistered)
	testutil.True(t, scheduleRegistered)
	testutil.True(t, gotBillingSvc == billingSvc)
	testutil.True(t, gotHandlerSvc != nil)
	testutil.True(t, gotPool != nil)
	testutil.Equal(t, 3600, gotInterval)
}

func TestWireBillingUsageSyncJobs_SkipsWhenNotStripe(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.Provider = "mock"

	var handlerRegistered, scheduleRegistered bool
	origRegisterHandler := registerBillingUsageSyncHandler
	origRegisterSchedule := registerBillingUsageSyncSchedule
	t.Cleanup(func() {
		registerBillingUsageSyncHandler = origRegisterHandler
		registerBillingUsageSyncSchedule = origRegisterSchedule
	})

	registerBillingUsageSyncHandler = func(_ *jobs.Service, _ billing.BillingService, _ *pgxpool.Pool) {
		handlerRegistered = true
	}
	registerBillingUsageSyncSchedule = func(_ context.Context, _ *jobs.Service, _ int) error {
		scheduleRegistered = true
		return nil
	}

	wireBillingUsageSyncJobs(
		context.Background(),
		cfg,
		&jobs.Service{},
		billing.NewNoopBillingService(),
		&pgxpool.Pool{},
		testNoopLogger(),
	)

	testutil.False(t, handlerRegistered)
	testutil.False(t, scheduleRegistered)
}

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

// --- TOTP encryption key resolution ---

func TestResolveTOTPEncryptionKey_UsesConfiguredHex(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
	cfg.Auth.EncryptionKey = strings.Repeat("ab", 32)

	key, err := resolveTOTPEncryptionKey(cfg.Auth)
	testutil.NoError(t, err)
	testutil.Equal(t, 32, len(key))
	testutil.Equal(t, 0xab, int(key[0]))
}

func TestResolveTOTPEncryptionKey_UsesConfiguredBase64(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 1)
	}

	cfg := config.Default()
	cfg.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
	cfg.Auth.EncryptionKey = base64.StdEncoding.EncodeToString(raw)

	key, err := resolveTOTPEncryptionKey(cfg.Auth)
	testutil.NoError(t, err)
	testutil.Equal(t, 32, len(key))
	testutil.Equal(t, int(raw[0]), int(key[0]))
	testutil.Equal(t, int(raw[31]), int(key[31]))
}

func TestResolveTOTPEncryptionKey_DerivesStableKeyFromJWTSecret(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"

	key1, err1 := resolveTOTPEncryptionKey(cfg.Auth)
	key2, err2 := resolveTOTPEncryptionKey(cfg.Auth)

	testutil.NoError(t, err1)
	testutil.NoError(t, err2)
	testutil.Equal(t, 32, len(key1))
	testutil.Equal(t, string(key1), string(key2))
}

func TestResolveTOTPEncryptionKey_RejectsInvalidConfiguredKey(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
	cfg.Auth.EncryptionKey = "not-hex-or-base64"

	_, err := resolveTOTPEncryptionKey(cfg.Auth)
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "hex or base64")
}

func TestResolveTOTPEncryptionKey_RequiresJWTSecretWhenNoConfiguredKey(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.JWTSecret = ""
	cfg.Auth.EncryptionKey = ""

	_, err := resolveTOTPEncryptionKey(cfg.Auth)
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "jwt_secret")
}

// --- TOTP key wiring: always-set policy ---

// TestResolveTOTPEncryptionKey_SucceedsEvenWhenTOTPDisabled verifies that the
// encryption key can be derived when TOTP is disabled in config, which is required
// because users may enable TOTP at runtime via the admin API.
func TestResolveTOTPEncryptionKey_SucceedsEvenWhenTOTPDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.JWTSecret = "this-is-a-secret-that-is-at-least-32-characters-long"
	cfg.Auth.TOTPEnabled = false
	cfg.Auth.EncryptionKey = ""

	key, err := resolveTOTPEncryptionKey(cfg.Auth)
	testutil.NoError(t, err)
	testutil.Equal(t, 32, len(key))

	// The key should be usable by the auth service.
	authSvc := auth.NewService(nil, cfg.Auth.JWTSecret, time.Hour, 24*time.Hour, 8, slog.Default())
	testutil.NoError(t, authSvc.SetEncryptionKey(key))
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

func testNoopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
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

func TestBuildEdgeFuncRuntimeConfig_UsesConfiguredValues(t *testing.T) {
	cfg := config.Default()
	cfg.EdgeFunctions.PoolSize = 3
	cfg.EdgeFunctions.DefaultTimeoutMs = 7000
	cfg.EdgeFunctions.MaxRequestBodyBytes = 2048
	cfg.EdgeFunctions.FetchDomainAllowlist = []string{"api.example.com"}
	cfg.EdgeFunctions.MemoryLimitMB = 192
	cfg.EdgeFunctions.MaxConcurrentInvocations = 80
	cfg.EdgeFunctions.CodeCacheSize = 333

	got := buildEdgeFuncRuntimeConfig(cfg)

	testutil.Equal(t, 3, got.PoolSize)
	testutil.Equal(t, 7*time.Second, got.DefaultTimeout)
	testutil.Equal(t, int64(2048), got.MaxRequestBodyBytes)
	testutil.SliceLen(t, got.FetchDomainAllowlist, 1)
	testutil.Equal(t, "api.example.com", got.FetchDomainAllowlist[0])
	testutil.Equal(t, 192, got.MemoryLimitMB)
	testutil.Equal(t, 80, got.MaxConcurrentInvocations)
	testutil.Equal(t, 333, got.CodeCacheSize)
}

func TestBuildEdgeFuncRuntimeConfig_FallsBackToSafeDefaults(t *testing.T) {
	cfg := config.Default()
	cfg.EdgeFunctions.PoolSize = 0
	cfg.EdgeFunctions.DefaultTimeoutMs = 0
	cfg.EdgeFunctions.MaxRequestBodyBytes = 0
	cfg.EdgeFunctions.MemoryLimitMB = 0
	cfg.EdgeFunctions.MaxConcurrentInvocations = 0
	cfg.EdgeFunctions.CodeCacheSize = 0

	got := buildEdgeFuncRuntimeConfig(cfg)

	testutil.Equal(t, 1, got.PoolSize)
	testutil.Equal(t, 5*time.Second, got.DefaultTimeout)
	testutil.Equal(t, int64(1<<20), got.MaxRequestBodyBytes)
	testutil.Equal(t, 128, got.MemoryLimitMB)
	testutil.Equal(t, 50, got.MaxConcurrentInvocations)
	testutil.Equal(t, 256, got.CodeCacheSize)
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

// configureTLSDefaults is not safe to call concurrently because it mutates
// certmagic global state, so we serialize these tests.
var tlsDefaultsMu sync.Mutex

// --- configureTLSDefaults ---

func TestConfigureTLSDefaults_StagingCA(t *testing.T) {
	tlsDefaultsMu.Lock()
	defer tlsDefaultsMu.Unlock()

	cfg := config.Default()
	cfg.Server.TLSStaging = true
	cfg.Server.TLSEmail = "ops@example.com"
	logger := slog.Default()

	configureTLSDefaults(cfg, logger)

	testutil.Equal(t, "https://acme-staging-v02.api.letsencrypt.org/directory", certmagic.DefaultACME.CA)
	testutil.Equal(t, "ops@example.com", certmagic.DefaultACME.Email)
}

func TestConfigureTLSDefaults_ProductionCA(t *testing.T) {
	tlsDefaultsMu.Lock()
	defer tlsDefaultsMu.Unlock()

	// Reset to a known state so test is hermetic.
	certmagic.DefaultACME.CA = ""
	certmagic.DefaultACME.Email = ""

	cfg := config.Default()
	cfg.Server.TLSStaging = false
	cfg.Server.TLSEmail = "admin@example.com"
	logger := slog.Default()

	configureTLSDefaults(cfg, logger)

	// When staging is false, CA should remain at default (not staging URL).
	testutil.True(t, certmagic.DefaultACME.CA != "https://acme-staging-v02.api.letsencrypt.org/directory",
		"expected production CA, got staging")
	testutil.Equal(t, "admin@example.com", certmagic.DefaultACME.Email)
}

func TestConfigureTLSDefaults_NoEmail(t *testing.T) {
	tlsDefaultsMu.Lock()
	defer tlsDefaultsMu.Unlock()

	certmagic.DefaultACME.Email = ""

	cfg := config.Default()
	cfg.Server.TLSStaging = true
	cfg.Server.TLSEmail = ""
	logger := slog.Default()

	configureTLSDefaults(cfg, logger)

	testutil.Equal(t, "https://acme-staging-v02.api.letsencrypt.org/directory", certmagic.DefaultACME.CA)
	testutil.Equal(t, "", certmagic.DefaultACME.Email)
}
