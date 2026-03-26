package nhostmigrate

import "github.com/allyourbase/ayb/internal/migrate"

// MigrationOptions configures the NHost import process.
type MigrationOptions struct {
	HasuraMetadataPath string
	PgDumpPath         string
	DatabaseURL        string
	DryRun             bool
	Verbose            bool
	Progress           migrate.ProgressReporter
	SkipRLS            bool
}

// MigrationStats tracks NHost migration progress.
type MigrationStats struct {
	Tables      int      `json:"tables"`
	Views       int      `json:"views"`
	Records     int      `json:"records"`
	Indexes     int      `json:"indexes"`
	ForeignKeys int      `json:"foreignKeys"`
	Policies    int      `json:"policies"`
	Skipped     int      `json:"skipped"`
	Errors      []string `json:"errors,omitempty"`
}
