package directusmigrate

import "github.com/allyourbase/ayb/internal/migrate"

// MigrationOptions configures Directus migration.
type MigrationOptions struct {
	SnapshotPath string
	DatabaseURL  string
	DryRun       bool
	Verbose      bool
	Progress     migrate.ProgressReporter
	SkipRLS      bool
}

// MigrationStats tracks Directus migration progress.
type MigrationStats struct {
	Collections int      `json:"collections"`
	Fields      int      `json:"fields"`
	Relations   int      `json:"relations"`
	Policies    int      `json:"policies"`
	Skipped     int      `json:"skipped"`
	Errors      []string `json:"errors,omitempty"`
}
