package pgmanager

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

// Config holds settings for the managed Postgres manager.
type Config struct {
	Port        uint32 // default 15432
	DataDir     string // persistent data directory (default ~/.ayb/data)
	RuntimeDir  string // ephemeral runtime directory (default ~/.ayb/run)
	BinCacheDir string // binary cache directory (default ~/.ayb/pg)
	Logger      *slog.Logger
}

// Manager manages the lifecycle of an managed PostgreSQL child process.
type Manager struct {
	cfg     Config
	db      *embeddedpostgres.EmbeddedPostgres
	connURL string
	running bool
	logger  *slog.Logger
	pidFile string
}

const (
	dbName    = "ayb"
	dbUser    = "ayb"
	dbPass    = "ayb"
	pgVersion = "16"
)

// New creates a new Manager. Does not start anything.
func New(cfg Config) *Manager {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Manager{
		cfg:    cfg,
		logger: cfg.Logger,
	}
}

// Start downloads PG binaries (on first run), initializes the data directory,
// starts the PostgreSQL child process, and returns a connection URL.
func (m *Manager) Start(ctx context.Context) (string, error) {
	if m.running {
		return m.connURL, nil
	}

	// Resolve paths, defaulting to ~/.ayb/ subdirectories.
	home, err := aybHome()
	if err != nil {
		return "", fmt.Errorf("resolving ayb home: %w", err)
	}

	dataDir := m.cfg.DataDir
	if dataDir == "" {
		dataDir = filepath.Join(home, "data")
	}
	runtimeDir := m.cfg.RuntimeDir
	if runtimeDir == "" {
		runtimeDir = filepath.Join(home, "run")
	}
	cacheDir := m.cfg.BinCacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(home, "pg")
	}

	port := m.cfg.Port
	if port == 0 {
		port = 15432
	}

	binDir := filepath.Join(home, "pgbin")

	// Ensure directories exist.
	for _, dir := range []string{dataDir, runtimeDir, cacheDir, binDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Check for orphaned process.
	m.pidFile = filepath.Join(home, "pg.pid")
	cleanupOrphan(m.pidFile, m.logger)

	// Check if first run (no cached binaries).
	if _, err := os.Stat(cacheDir); err == nil {
		entries, _ := os.ReadDir(cacheDir)
		if len(entries) == 0 {
			m.logger.Info("downloading PostgreSQL binaries (first run only)...")
		}
	}

	m.db = embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(port).
		DataPath(dataDir).
		RuntimePath(runtimeDir).
		BinariesPath(binDir).
		CachePath(cacheDir).
		Version(embeddedpostgres.V16).
		Database(dbName).
		Username(dbUser).
		Password(dbPass).
		Logger(newLogWriter(m.logger)).
		StartTimeout(60 * time.Second))

	if err := m.db.Start(); err != nil {
		return "", fmt.Errorf("starting managed postgres: %w", err)
	}

	// Write our PID file by reading the Postgres postmaster.pid.
	pgPidFile := filepath.Join(dataDir, "postmaster.pid")
	if pid, err := readPostmasterPID(pgPidFile); err == nil && pid > 0 {
		_ = writePID(m.pidFile, pid)
	}

	m.connURL = fmt.Sprintf("postgresql://%s:%s@127.0.0.1:%d/%s?sslmode=disable",
		dbUser, dbPass, port, dbName)
	m.running = true

	m.logger.Info("managed postgres started",
		"port", port,
		"data", dataDir,
	)
	return m.connURL, nil
}

// Stop gracefully shuts down the managed PostgreSQL child process.
func (m *Manager) Stop() error {
	if !m.running || m.db == nil {
		return nil
	}

	m.logger.Info("stopping managed postgres")
	err := m.db.Stop()
	m.running = false

	// Clean up PID file.
	_ = removePID(m.pidFile)

	if err != nil {
		return fmt.Errorf("stopping managed postgres: %w", err)
	}
	m.logger.Info("managed postgres stopped")
	return nil
}

// ConnURL returns the connection URL. Only valid after Start() succeeds.
func (m *Manager) ConnURL() string {
	return m.connURL
}

// IsRunning returns true if the managed Postgres is currently running.
func (m *Manager) IsRunning() bool {
	return m.running
}

// --- AYB home directory ---

// aybHome returns ~/.ayb, creating it if necessary.
func aybHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting user home directory: %w", err)
	}
	aybDir := filepath.Join(home, ".ayb")
	if err := os.MkdirAll(aybDir, 0o755); err != nil {
		return "", fmt.Errorf("creating ~/.ayb: %w", err)
	}
	return aybDir, nil
}

// --- PID file management ---

// writePID writes the given PID to a file.
func writePID(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

// readPID reads a PID from a file. Returns 0 if the file doesn't exist.
func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parsing pid file: %w", err)
	}
	return pid, nil
}

// removePID removes a PID file. No error if it doesn't exist.
func removePID(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// readPostmasterPID reads the PID from Postgres's postmaster.pid file.
// The PID is on the first line.
func readPostmasterPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 {
		return 0, fmt.Errorf("empty postmaster.pid")
	}
	return strconv.Atoi(strings.TrimSpace(lines[0]))
}

// cleanupOrphan checks for a stale PID file and kills the orphaned process.
func cleanupOrphan(pidPath string, logger *slog.Logger) {
	pid, err := readPID(pidPath)
	if err != nil || pid == 0 {
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = removePID(pidPath)
		return
	}

	// Check if process is alive (signal 0 tests existence).
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process is dead — remove stale PID file.
		logger.Info("removed stale PID file", "pid", pid)
		_ = removePID(pidPath)
		return
	}

	// Process is alive — kill it.
	logger.Warn("found orphaned postgres process, terminating", "pid", pid)
	_ = proc.Signal(syscall.SIGTERM)

	// Wait up to 5 seconds for graceful shutdown.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process exited.
			_ = removePID(pidPath)
			logger.Info("orphaned postgres process terminated", "pid", pid)
			return
		}
	}

	// Force kill.
	logger.Warn("force-killing orphaned postgres", "pid", pid)
	_ = proc.Signal(syscall.SIGKILL)
	_ = removePID(pidPath)
}

// --- Log writer adapter ---

// logWriter adapts *slog.Logger to io.Writer for embedded-postgres output.
type logWriter struct {
	logger *slog.Logger
}

func newLogWriter(logger *slog.Logger) *logWriter {
	return &logWriter{logger: logger}
}

func (w *logWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n\r")
	if msg != "" {
		w.logger.Debug("postgres", "output", msg)
	}
	return len(p), nil
}
