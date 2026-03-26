package appwritemigrate

import "github.com/allyourbase/ayb/internal/migrate"

// MigrationOptions configures Appwrite migration.
type MigrationOptions struct {
	ExportPath  string
	DatabaseURL string
	DryRun      bool
	Verbose     bool
	Progress    migrate.ProgressReporter
	SkipRLS     bool
	SkipData    bool
}

// MigrationStats tracks Appwrite migration progress.
type MigrationStats struct {
	Collections int      `json:"collections"`
	Attributes  int      `json:"attributes"`
	Indexes     int      `json:"indexes"`
	Policies    int      `json:"policies"`
	Documents   int      `json:"documents"`
	Skipped     int      `json:"skipped"`
	Errors      []string `json:"errors,omitempty"`
}
