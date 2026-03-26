package extensions

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

// ExtensionInfo describes a PostgreSQL extension's availability and install status.
type ExtensionInfo struct {
	Name             string `json:"name"`
	Installed        bool   `json:"installed"`
	Available        bool   `json:"available"`
	InstalledVersion string `json:"installed_version,omitempty"`
	DefaultVersion   string `json:"default_version,omitempty"`
	Comment          string `json:"comment,omitempty"`
}

// DB is the minimal database interface needed by the extension service.
type DB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Service provides extension management operations against a PostgreSQL database.
type Service struct {
	db DB
}

// NewService creates a new extension management service.
func NewService(db DB) *Service {
	return &Service{db: db}
}

// validName matches PostgreSQL extension name conventions: alphanumeric, underscores, hyphens.
var validName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// ValidateExtensionName validates that an extension name is safe for use in SQL.
func ValidateExtensionName(name string) error {
	if name == "" {
		return fmt.Errorf("extension name must not be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("extension name must not exceed 64 characters")
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("extension name %q contains invalid characters: must start with a letter and contain only letters, digits, underscores, or hyphens", name)
	}
	return nil
}

// List returns all available extensions with their install status.
func (s *Service) List(ctx context.Context) ([]ExtensionInfo, error) {
	const query = `
		SELECT
			a.name,
			a.default_version,
			a.comment,
			e.extversion
		FROM pg_available_extensions a
		LEFT JOIN pg_extension e ON e.extname = a.name
		ORDER BY a.name`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying extensions: %w", err)
	}
	defer rows.Close()

	var exts []ExtensionInfo
	for rows.Next() {
		var info ExtensionInfo
		var defaultVer, comment, installedVer sql.NullString
		if err := rows.Scan(&info.Name, &defaultVer, &comment, &installedVer); err != nil {
			return nil, fmt.Errorf("scanning extension row: %w", err)
		}
		info.Available = true
		info.DefaultVersion = defaultVer.String
		info.Comment = comment.String
		if installedVer.Valid {
			info.Installed = true
			info.InstalledVersion = installedVer.String
		}
		exts = append(exts, info)
	}
	return exts, rows.Err()
}

// Enable installs a PostgreSQL extension. Returns nil if the extension is already installed.
func (s *Service) Enable(ctx context.Context, name string) error {
	if err := ValidateExtensionName(name); err != nil {
		return err
	}

	// Verify extension is available before attempting to create.
	avail, err := s.isAvailable(ctx, name)
	if err != nil {
		return err
	}
	if !avail {
		return fmt.Errorf("extension %q is not available on this server", name)
	}

	_, err = s.db.ExecContext(ctx, enableSQL(name))
	if err != nil {
		return fmt.Errorf("enabling extension %q: %w", name, err)
	}
	return nil
}

// Disable removes a PostgreSQL extension. Returns nil if the extension is not installed.
// Returns an error if the extension has dependent objects (use DisableCascade for that).
func (s *Service) Disable(ctx context.Context, name string) error {
	if err := ValidateExtensionName(name); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, disableSQL(name))
	if err != nil {
		if isDependencyError(err) {
			return fmt.Errorf("extension %q has dependent objects; use cascade to force removal: %w", name, err)
		}
		return fmt.Errorf("disabling extension %q: %w", name, err)
	}
	return nil
}

// isAvailable checks if an extension is available for installation.
func (s *Service) isAvailable(ctx context.Context, name string) (bool, error) {
	var count int
	row, err := s.db.QueryContext(ctx, "SELECT COUNT(*) FROM pg_available_extensions WHERE name = $1", name)
	if err != nil {
		return false, fmt.Errorf("checking extension availability: %w", err)
	}
	defer row.Close()
	if row.Next() {
		if err := row.Scan(&count); err != nil {
			return false, fmt.Errorf("scanning availability check: %w", err)
		}
	}
	return count > 0, row.Err()
}

func enableSQL(name string) string {
	return "CREATE EXTENSION IF NOT EXISTS " + sqlutil.QuoteIdent(name)
}

func disableSQL(name string) string {
	return "DROP EXTENSION IF EXISTS " + sqlutil.QuoteIdent(name)
}

func disableSQLCascade(name string) string {
	return "DROP EXTENSION IF EXISTS " + sqlutil.QuoteIdent(name) + " CASCADE"
}

// isDependencyError checks if a PostgreSQL error is about dependent objects.
func isDependencyError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "depends on") || strings.Contains(msg, "dependency")
}
