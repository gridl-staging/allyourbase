package cli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/branching"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/pgmanager"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

func TestRunGracefulShutdownExtractionExists(t *testing.T) {
	_ = runGracefulShutdown
}

func TestApplyInitDatabaseBranchURL(t *testing.T) {
	cfg := config.Default()
	cfg.Database.URL = "postgres://user:pass@localhost:5432/app_db?sslmode=disable"

	originalURL := cfg.Database.URL
	branchName := "feature/refactor-startup"
	expectedBranchDB := branching.BranchDBName(branchName)
	expectedURL, err := branching.ReplaceDatabaseInURL(originalURL, expectedBranchDB)
	testutil.NoError(t, err)

	err = applyInitDatabaseBranchURL(cfg, branchName, testNoopLogger())
	testutil.NoError(t, err)
	testutil.Equal(t, expectedURL, cfg.Database.URL)
}

type fakeInitDatabaseManagedPostgres struct {
	connURL   string
	stopCalls int
}

func (f *fakeInitDatabaseManagedPostgres) Start(context.Context) (string, error) {
	return f.connURL, nil
}

func (f *fakeInitDatabaseManagedPostgres) Stop() error {
	f.stopCalls++
	return nil
}

type fakeInitDatabasePool struct {
	closeCalls int
}

func (f *fakeInitDatabasePool) DB() *pgxpool.Pool {
	return nil
}

func (f *fakeInitDatabasePool) Close() {
	f.closeCalls++
}

type fakeInitDatabaseMigrationRunner struct{}

func (fakeInitDatabaseMigrationRunner) Bootstrap(context.Context) error {
	return nil
}

func (fakeInitDatabaseMigrationRunner) Run(context.Context) (int, error) {
	return 0, nil
}

func TestInitDatabaseCleansOwnedResourcesOnFromMigrationFailure(t *testing.T) {
	fakePG := &fakeInitDatabaseManagedPostgres{connURL: "postgresql://managed.example/test"}
	fakePool := &fakeInitDatabasePool{}

	origManagedPostgres := newInitDatabaseManagedPostgres
	origPool := newInitDatabasePool
	origMigrationRunner := newInitDatabaseMigrationRunner
	origRunFromMigration := runFromMigrationForInitDatabase
	t.Cleanup(func() {
		newInitDatabaseManagedPostgres = origManagedPostgres
		newInitDatabasePool = origPool
		newInitDatabaseMigrationRunner = origMigrationRunner
		runFromMigrationForInitDatabase = origRunFromMigration
	})

	newInitDatabaseManagedPostgres = func(pgmanager.Config) initDatabaseManagedPostgres {
		return fakePG
	}
	newInitDatabasePool = func(context.Context, postgres.Config, *slog.Logger) (initDatabasePool, error) {
		return fakePool, nil
	}
	newInitDatabaseMigrationRunner = func(*pgxpool.Pool, *slog.Logger) initDatabaseMigrationRunner {
		return fakeInitDatabaseMigrationRunner{}
	}
	runFromMigrationForInitDatabase = func(context.Context, string, string, *slog.Logger) error {
		return errors.New("boom")
	}

	cfg := config.Default()
	cfg.Database.URL = ""
	cfg.Database.MigrationsDir = ""

	pool, pgMgr, schemaCache, watcherCancel, err := initDatabase(
		context.Background(),
		cfg,
		"seed.sql",
		"",
		make(chan os.Signal, 1),
		testNoopLogger(),
		newStartupProgress(io.Discard, false, false),
	)

	testutil.ErrorContains(t, err, "migration failed: boom")
	testutil.Nil(t, pool)
	testutil.Nil(t, pgMgr)
	testutil.Nil(t, schemaCache)
	testutil.Nil(t, watcherCancel)
	testutil.Equal(t, 1, fakePool.closeCalls)
	testutil.Equal(t, 1, fakePG.stopCalls)
	testutil.Equal(t, fakePG.connURL, cfg.Database.URL)
}

func TestInitDatabaseCleansUpManagedPGOnPoolFailure(t *testing.T) {
	fakePG := &fakeInitDatabaseManagedPostgres{connURL: "postgresql://managed.example/test"}

	origManagedPostgres := newInitDatabaseManagedPostgres
	origPool := newInitDatabasePool
	t.Cleanup(func() {
		newInitDatabaseManagedPostgres = origManagedPostgres
		newInitDatabasePool = origPool
	})

	newInitDatabaseManagedPostgres = func(pgmanager.Config) initDatabaseManagedPostgres {
		return fakePG
	}
	newInitDatabasePool = func(context.Context, postgres.Config, *slog.Logger) (initDatabasePool, error) {
		return nil, errors.New("connection refused")
	}

	cfg := config.Default()
	cfg.Database.URL = ""

	pool, pgMgr, schemaCache, watcherCancel, err := initDatabase(
		context.Background(),
		cfg,
		"",
		"",
		make(chan os.Signal, 1),
		testNoopLogger(),
		newStartupProgress(io.Discard, false, false),
	)

	testutil.ErrorContains(t, err, "connecting to database")
	testutil.Nil(t, pool)
	testutil.Nil(t, pgMgr)
	testutil.Nil(t, schemaCache)
	testutil.Nil(t, watcherCancel)
	testutil.Equal(t, 1, fakePG.stopCalls)
}

func TestStartInitDatabaseManagedPostgresUsesDeprecatedEmbeddedPortWhenManagedPGStillDefault(t *testing.T) {
	origManagedPostgres := newInitDatabaseManagedPostgres
	t.Cleanup(func() {
		newInitDatabaseManagedPostgres = origManagedPostgres
	})

	var got pgmanager.Config
	newInitDatabaseManagedPostgres = func(cfg pgmanager.Config) initDatabaseManagedPostgres {
		got = cfg
		return &fakeInitDatabaseManagedPostgres{connURL: "postgresql://managed.example/test"}
	}

	cfg := config.Default()
	cfg.Database.URL = ""
	cfg.Database.EmbeddedPort = 19999

	managedPG, pgMgr, err := startInitDatabaseManagedPostgres(
		context.Background(),
		cfg,
		testNoopLogger(),
		newStartupProgress(io.Discard, false, false),
	)

	testutil.NoError(t, err)
	testutil.NotNil(t, managedPG)
	testutil.Nil(t, pgMgr)
	testutil.Equal(t, uint32(19999), got.Port)
	testutil.Equal(t, "postgresql://managed.example/test", cfg.Database.URL)
}

func TestStartInitDatabaseManagedPostgresPrefersManagedPGPortOverDeprecatedEmbeddedPort(t *testing.T) {
	origManagedPostgres := newInitDatabaseManagedPostgres
	t.Cleanup(func() {
		newInitDatabaseManagedPostgres = origManagedPostgres
	})

	var got pgmanager.Config
	newInitDatabaseManagedPostgres = func(cfg pgmanager.Config) initDatabaseManagedPostgres {
		got = cfg
		return &fakeInitDatabaseManagedPostgres{connURL: "postgresql://managed.example/test"}
	}

	cfg := config.Default()
	cfg.Database.URL = ""
	cfg.Database.EmbeddedPort = 19999
	cfg.ManagedPG.Port = 25432

	_, _, err := startInitDatabaseManagedPostgres(
		context.Background(),
		cfg,
		testNoopLogger(),
		newStartupProgress(io.Discard, false, false),
	)

	testutil.NoError(t, err)
	testutil.Equal(t, uint32(25432), got.Port)
}

func TestInitDatabaseCleansUpBothOnBootstrapFailure(t *testing.T) {
	fakePG := &fakeInitDatabaseManagedPostgres{connURL: "postgresql://managed.example/test"}
	fakePool := &fakeInitDatabasePool{}

	origManagedPostgres := newInitDatabaseManagedPostgres
	origPool := newInitDatabasePool
	origMigrationRunner := newInitDatabaseMigrationRunner
	t.Cleanup(func() {
		newInitDatabaseManagedPostgres = origManagedPostgres
		newInitDatabasePool = origPool
		newInitDatabaseMigrationRunner = origMigrationRunner
	})

	newInitDatabaseManagedPostgres = func(pgmanager.Config) initDatabaseManagedPostgres {
		return fakePG
	}
	newInitDatabasePool = func(context.Context, postgres.Config, *slog.Logger) (initDatabasePool, error) {
		return fakePool, nil
	}
	newInitDatabaseMigrationRunner = func(*pgxpool.Pool, *slog.Logger) initDatabaseMigrationRunner {
		return &failingBootstrapMigrationRunner{}
	}

	cfg := config.Default()
	cfg.Database.URL = ""

	pool, pgMgr, schemaCache, watcherCancel, err := initDatabase(
		context.Background(),
		cfg,
		"",
		"",
		make(chan os.Signal, 1),
		testNoopLogger(),
		newStartupProgress(io.Discard, false, false),
	)

	testutil.ErrorContains(t, err, "bootstrapping migrations")
	testutil.Nil(t, pool)
	testutil.Nil(t, pgMgr)
	testutil.Nil(t, schemaCache)
	testutil.Nil(t, watcherCancel)
	testutil.Equal(t, 1, fakePool.closeCalls)
	testutil.Equal(t, 1, fakePG.stopCalls)
}

type failingBootstrapMigrationRunner struct{}

func (failingBootstrapMigrationRunner) Bootstrap(context.Context) error {
	return errors.New("bootstrap exploded")
}

func (failingBootstrapMigrationRunner) Run(context.Context) (int, error) {
	return 0, nil
}

func prepareAYBHome(t *testing.T) string {
	t.Helper()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	testutil.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".ayb"), 0o755))
	return homeDir
}

func writeTestAdminToken(t *testing.T, homeDir, token string) {
	t.Helper()
	testutil.NoError(t, os.WriteFile(filepath.Join(homeDir, ".ayb", "admin-token"), []byte(token), 0o600))
}

func startAdminAuthStubServer(t *testing.T, expectedPassword, issuedToken string) (string, int) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	testutil.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/auth", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if body.Password != expectedPassword {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"token":"%s"}`, issuedToken)
	})

	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})

	addr := listener.Addr().(*net.TCPAddr)
	return fmt.Sprintf("http://127.0.0.1:%d", addr.Port), addr.Port
}

func TestWaitForServerReadyWritesBearerTokenToAdminTokenFile(t *testing.T) {
	homeDir := prepareAYBHome(t)

	const adminPassword = "stage-admin-password"
	const bearerToken = "stage-admin-bearer-token"
	_, port := startAdminAuthStubServer(t, adminPassword, bearerToken)

	cfg := config.Default()
	cfg.Server.Port = port
	cfg.Admin.Enabled = true
	cfg.Admin.Password = adminPassword

	ready := make(chan struct{})
	close(ready)
	errCh := make(chan error, 1)
	usrCh := make(chan os.Signal)

	cleanup, err := waitForServerReady(
		ready,
		errCh,
		usrCh,
		cfg,
		nil,
		nil,
		testNoopLogger(),
		newStartupProgress(io.Discard, false, false),
		readyState{},
	)
	testutil.NoError(t, err)
	t.Cleanup(cleanup)
	t.Cleanup(func() { close(usrCh) })

	tokenPath := filepath.Join(homeDir, ".ayb", "admin-token")
	data, err := os.ReadFile(tokenPath)
	testutil.NoError(t, err)
	testutil.Equal(t, bearerToken, strings.TrimSpace(string(data)))
}

func TestResolveSavedAdminTokenExchangesSavedPassword(t *testing.T) {
	homeDir := prepareAYBHome(t)

	const savedPassword = "saved-password"
	const bearerToken = "exchanged-bearer-token"
	baseURL, _ := startAdminAuthStubServer(t, savedPassword, bearerToken)

	writeTestAdminToken(t, homeDir, savedPassword)
	testutil.Equal(t, bearerToken, resolveSavedAdminToken(baseURL))
}

func TestResolveSavedAdminTokenReturnsFileTokenWhenPasswordExchangeFails(t *testing.T) {
	homeDir := prepareAYBHome(t)

	const savedToken = "already-a-bearer-token"
	baseURL, _ := startAdminAuthStubServer(t, "expected-password", "unused-token")

	writeTestAdminToken(t, homeDir, savedToken)
	testutil.Equal(t, savedToken, resolveSavedAdminToken(baseURL))
}

func TestResolveSavedAdminTokenSkipsSavedAuthForNonLocalURL(t *testing.T) {
	homeDir := prepareAYBHome(t)

	writeTestAdminToken(t, homeDir, "legacy-password")
	testutil.Equal(t, "", resolveSavedAdminToken("https://example.com"))
}

func TestRunSQLRequiresExplicitTokenForNonLocalURL(t *testing.T) {
	homeDir := prepareAYBHome(t)
	t.Setenv("AYB_ADMIN_TOKEN", "")
	writeTestAdminToken(t, homeDir, "legacy-password")

	cmd := &cobra.Command{}
	cmd.Flags().String("admin-token", "", "")
	cmd.Flags().String("url", "", "")
	testutil.NoError(t, cmd.Flags().Set("url", "https://example.com"))

	err := runSQL(cmd, []string{"select 1"})
	testutil.ErrorContains(t, err, "pass --admin-token or set AYB_ADMIN_TOKEN")
}

func TestResolveDemoJWTSecretUsesExistingEnv(t *testing.T) {
	t.Setenv("AYB_AUTH_JWT_SECRET", "preconfigured-secret")
	secret, err := resolveDemoJWTSecret()
	testutil.NoError(t, err)
	testutil.Equal(t, "preconfigured-secret", secret)
}

func TestResolveDemoJWTSecretGeneratesRandomHexWhenUnset(t *testing.T) {
	t.Setenv("AYB_AUTH_JWT_SECRET", "")
	secret, err := resolveDemoJWTSecret()
	testutil.NoError(t, err)
	testutil.Equal(t, 64, len(secret))
	_, decodeErr := hex.DecodeString(secret)
	testutil.NoError(t, decodeErr)
}

func TestWritePasswordResetResultFileWritesPassword(t *testing.T) {
	prepareAYBHome(t)

	const newPassword = "reset-password-123"
	testutil.NoError(t, writePasswordResetResultFile(newPassword))

	resultPath, err := aybResetResultPath()
	testutil.NoError(t, err)

	data, err := os.ReadFile(resultPath)
	testutil.NoError(t, err)
	testutil.Equal(t, newPassword, string(data))
}

func TestWritePasswordResetResultFileReturnsErrorWhenDirectoryMissing(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	err := writePasswordResetResultFile("reset-password-123")
	testutil.ErrorContains(t, err, "writing password reset result file")
}
