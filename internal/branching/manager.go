// Package branching manages PostgreSQL database branches via a Manager that supports branch creation through cloning, deletion, and listing.
package branching

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os/exec"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

// Manager orchestrates branch create/delete/list workflows.
type Manager struct {
	repo             Repo
	pool             *pgxpool.Pool
	logger           *slog.Logger
	pgDumpPath       string
	psqlPath         string
	defaultSourceURL string
}

// ManagerConfig configures the Manager.
type ManagerConfig struct {
	PgDumpPath       string
	PsqlPath         string
	DefaultSourceURL string // fallback source database URL when callers omit it
}

// NewManager creates a Manager.
func NewManager(pool *pgxpool.Pool, repo Repo, logger *slog.Logger, cfg ManagerConfig) *Manager {
	return &Manager{
		repo:             repo,
		pool:             pool,
		logger:           logger,
		pgDumpPath:       cfg.PgDumpPath,
		psqlPath:         cfg.PsqlPath,
		defaultSourceURL: cfg.DefaultSourceURL,
	}
}

// Create creates a new database branch by cloning the source database.
// Steps: validate name → record metadata → CREATE DATABASE → pg_dump | psql → mark ready.
// On failure: drops the created DB and marks metadata as failed.
// If sourceDBURL is empty, the configured DefaultSourceURL is used.
func (m *Manager) Create(ctx context.Context, branchName, sourceDBURL string) (*BranchRecord, error) {
	if err := ValidateBranchName(branchName); err != nil {
		return nil, fmt.Errorf("invalid branch name: %w", err)
	}

	// Fall back to the configured default when no explicit source is given.
	if sourceDBURL == "" {
		sourceDBURL = m.defaultSourceURL
	}
	if sourceDBURL == "" {
		return nil, fmt.Errorf("no source database URL provided and no default configured")
	}

	branchDB := BranchDBName(branchName)

	// Extract source DB name from URL for metadata.
	sourceDBName, err := ExtractDBNameFromURL(sourceDBURL)
	if err != nil {
		return nil, fmt.Errorf("extracting source db name: %w", err)
	}

	// Check if branch already exists.
	existing, err := m.repo.Get(ctx, branchName)
	if err != nil {
		return nil, fmt.Errorf("checking existing branch: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("branch %q already exists (status: %s)", branchName, existing.Status)
	}

	// Record metadata.
	rec, err := m.repo.Create(ctx, branchName, sourceDBName, branchDB)
	if err != nil {
		return nil, fmt.Errorf("recording branch metadata: %w", err)
	}

	// Create the target database.
	if err := m.createDatabase(ctx, branchDB); err != nil {
		_ = m.repo.UpdateStatus(ctx, rec.ID, StatusFailed, err.Error())
		return nil, fmt.Errorf("creating branch database: %w", err)
	}

	// Clone via pg_dump | psql.
	branchDBURL, err := ReplaceDatabaseInURL(sourceDBURL, branchDB)
	if err != nil {
		_ = m.dropDatabase(ctx, branchDB)
		_ = m.repo.UpdateStatus(ctx, rec.ID, StatusFailed, err.Error())
		return nil, fmt.Errorf("building branch database URL: %w", err)
	}

	if err := m.cloneDatabase(ctx, sourceDBURL, branchDBURL); err != nil {
		_ = m.dropDatabase(ctx, branchDB)
		_ = m.repo.UpdateStatus(ctx, rec.ID, StatusFailed, err.Error())
		return nil, fmt.Errorf("cloning database: %w", err)
	}

	// Mark ready.
	if err := m.repo.UpdateStatus(ctx, rec.ID, StatusReady, ""); err != nil {
		return nil, fmt.Errorf("marking branch ready: %w", err)
	}

	rec.Status = StatusReady
	m.logger.Info("branch created", "name", branchName, "database", branchDB)
	return rec, nil
}

// Delete removes a database branch: terminate connections, drop DB, remove metadata.
func (m *Manager) Delete(ctx context.Context, branchName string) error {
	rec, err := m.repo.Get(ctx, branchName)
	if err != nil {
		return fmt.Errorf("looking up branch: %w", err)
	}
	if rec == nil {
		return fmt.Errorf("branch %q not found", branchName)
	}

	if IsProtectedDatabase(rec.BranchDatabase) {
		return fmt.Errorf("cannot delete protected database %q", rec.BranchDatabase)
	}

	// Terminate active connections.
	if err := m.terminateConnections(ctx, rec.BranchDatabase); err != nil {
		m.logger.Warn("failed to terminate connections", "database", rec.BranchDatabase, "error", err)
	}

	// Drop the database.
	if err := m.dropDatabase(ctx, rec.BranchDatabase); err != nil {
		return fmt.Errorf("dropping branch database: %w", err)
	}

	// Remove metadata.
	if err := m.repo.Delete(ctx, rec.ID); err != nil {
		return fmt.Errorf("removing branch metadata: %w", err)
	}

	m.logger.Info("branch deleted", "name", branchName, "database", rec.BranchDatabase)
	return nil
}

// List returns all branches sorted by created time.
func (m *Manager) List(ctx context.Context) ([]BranchRecord, error) {
	return m.repo.List(ctx)
}

// Get returns a single branch by name.
func (m *Manager) Get(ctx context.Context, name string) (*BranchRecord, error) {
	return m.repo.Get(ctx, name)
}

// --- internal helpers ---

func (m *Manager) createDatabase(ctx context.Context, dbName string) error {
	_, err := m.pool.Exec(ctx, createDBSQL(dbName))
	return err
}

func (m *Manager) dropDatabase(ctx context.Context, dbName string) error {
	_, err := m.pool.Exec(ctx, dropDBSQL(dbName))
	return err
}

func (m *Manager) terminateConnections(ctx context.Context, dbName string) error {
	_, err := m.pool.Exec(ctx, terminateConnsSQL(), dbName)
	return err
}

// cloneDatabase copies a PostgreSQL database from source to target by piping pg_dump to psql, capturing stderr and returning any error.
func (m *Manager) cloneDatabase(ctx context.Context, sourceURL, targetURL string) error {
	pgDump, err := m.resolveBinary("pg_dump", m.pgDumpPath)
	if err != nil {
		return err
	}
	psql, err := m.resolveBinary("psql", m.psqlPath)
	if err != nil {
		return err
	}

	// pg_dump source | psql target
	dumpCmd := exec.CommandContext(ctx, pgDump, dumpArgs(sourceURL)...)
	restoreCmd := exec.CommandContext(ctx, psql, restoreArgs(targetURL)...)

	pr, pw := io.Pipe()
	dumpCmd.Stdout = pw
	restoreCmd.Stdin = pr

	var dumpStderr, restoreStderr bytes.Buffer
	dumpCmd.Stderr = &dumpStderr
	restoreCmd.Stderr = &restoreStderr

	if err := dumpCmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return fmt.Errorf("starting pg_dump: %w", err)
	}
	if err := restoreCmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		_ = dumpCmd.Process.Kill()
		_ = dumpCmd.Wait()
		return fmt.Errorf("starting psql: %w", err)
	}

	// Wait for dump to finish writing, then close the write end so psql sees EOF.
	dumpErr := dumpCmd.Wait()
	pw.Close()

	// Wait for restore to finish reading.
	restoreErr := restoreCmd.Wait()
	pr.Close()

	if dumpErr != nil {
		return fmt.Errorf("pg_dump failed: %w: %s", dumpErr, dumpStderr.String())
	}
	if restoreErr != nil {
		return fmt.Errorf("psql restore failed: %w: %s", restoreErr, restoreStderr.String())
	}

	return nil
}

func (m *Manager) resolveBinary(name, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found in PATH: %w", name, err)
	}
	return path, nil
}

func dumpArgs(dbURL string) []string {
	return []string{
		"--dbname=" + dbURL,
		"--format=plain",
		"--no-owner",
		"--no-privileges",
	}
}

func restoreArgs(dbURL string) []string {
	return []string{
		"--dbname=" + dbURL,
		"--quiet",
		"--no-psqlrc",
	}
}

func createDBSQL(dbName string) string {
	return "CREATE DATABASE " + sqlutil.QuoteIdent(dbName)
}

func dropDBSQL(dbName string) string {
	return "DROP DATABASE IF EXISTS " + sqlutil.QuoteIdent(dbName)
}

func terminateConnsSQL() string {
	return `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`
}

// ReplaceDatabaseInURL swaps the database name in a PostgreSQL URL.
func ReplaceDatabaseInURL(dbURL, newDB string) (string, error) {
	u, err := url.Parse(dbURL)
	if err != nil {
		return "", fmt.Errorf("parsing database URL: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	u.Path = "/" + newDB
	return u.String(), nil
}

// ExtractDBNameFromURL returns the database name from a PostgreSQL URL.
func ExtractDBNameFromURL(dbURL string) (string, error) {
	u, err := url.Parse(dbURL)
	if err != nil {
		return "", fmt.Errorf("parsing database URL: %w", err)
	}
	path := u.Path
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	if path == "" {
		return "", fmt.Errorf("no database name in URL")
	}
	return path, nil
}
