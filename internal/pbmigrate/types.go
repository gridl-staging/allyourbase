package pbmigrate

import (
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

// PBCollection represents a PocketBase collection from the _collections table
type PBCollection struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Type    string    `json:"type"` // "base", "view", "auth"
	System  bool      `json:"system"`
	Schema  []PBField `json:"schema"`
	Indexes []string  `json:"indexes"`

	// API Rules
	ListRule   *string `json:"listRule"`   // null = locked (admin-only)
	ViewRule   *string `json:"viewRule"`   // "" = open to all
	CreateRule *string `json:"createRule"` // "expr" = filtered
	UpdateRule *string `json:"updateRule"`
	DeleteRule *string `json:"deleteRule"`

	// Options (varies by type)
	Options map[string]interface{} `json:"options"`

	// View-specific
	ViewQuery string `json:"viewQuery,omitempty"`
}

// PBField represents a field in a PocketBase collection schema
type PBField struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"` // text, number, bool, email, url, editor, date, select, json, file, relation
	System       bool                   `json:"system"`
	Required     bool                   `json:"required"`
	Unique       bool                   `json:"unique"`
	MaxSelect    float64                `json:"maxSelect"`    // newer PocketBase stores this at top-level
	CollectionID string                 `json:"collectionId"` // newer PocketBase relation fields
	Options      map[string]interface{} `json:"options"`      // older PocketBase option bag
}

// PBRecord represents a generic record from any collection
type PBRecord struct {
	ID      string                 `json:"id"`
	Created time.Time              `json:"created"`
	Updated time.Time              `json:"updated"`
	Data    map[string]interface{} // field name → value
}

// MigrationStats tracks migration progress
type MigrationStats struct {
	Collections int
	Tables      int
	Views       int
	Records     int
	AuthUsers   int
	Files       int
	Policies    int
	Errors      []string
}

// MigrationOptions configures the migration process
type MigrationOptions struct {
	SourcePath     string // path to pb_data directory
	DatabaseURL    string // PostgreSQL connection string
	DryRun         bool   // if true, report but don't execute
	SkipFiles      bool   // if true, skip file migration
	Force          bool   // if true, allow migration to non-empty database
	Verbose        bool   // if true, show detailed progress
	StorageBackend string // storage backend: "local" or "s3"
	StoragePath    string // local storage path (default: ./ayb_storage)

	// Progress receives live progress updates. If nil, a NopReporter is used.
	Progress migrate.ProgressReporter
}
