// Package sbmigrate migrates auth users, OAuth identities, RLS policies,
// and data tables from a Supabase PostgreSQL database to AYB.
package sbmigrate

import (
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

// SupabaseUser represents a user from Supabase's auth.users table.
type SupabaseUser struct {
	ID                string
	Email             string
	EncryptedPassword string     // bcrypt hash
	EmailConfirmedAt  *time.Time // non-nil = verified
	RawUserMetaData   map[string]any
	CreatedAt         time.Time
	UpdatedAt         time.Time
	IsAnonymous       bool
}

// SupabaseIdentity represents an OAuth identity from Supabase's auth.identities table.
type SupabaseIdentity struct {
	UserID       string
	Provider     string
	IdentityData map[string]any // JSONB with sub, email, name, full_name
	CreatedAt    time.Time
}

// RLSPolicy represents an existing RLS policy read from pg_catalog.
type RLSPolicy struct {
	PolicyName string
	TableName  string
	SchemaName string
	Command    string // SELECT, INSERT, UPDATE, DELETE, ALL
	Permissive bool
	UsingExpr  string
	CheckExpr  string
}

// TableInfo represents a source table's schema.
type TableInfo struct {
	Name        string
	Columns     []ColumnInfo
	PrimaryKey  string // column name of the PK (empty if composite/none)
	ForeignKeys []ForeignKeyInfo
	RowCount    int64
}

// ColumnInfo describes a single column in a table.
type ColumnInfo struct {
	Name         string
	DataType     string // PostgreSQL type name (e.g., "integer", "text", "uuid")
	IsNullable   bool
	DefaultValue string // empty string = no default
	OrdinalPos   int
}

// ForeignKeyInfo describes a foreign key constraint.
type ForeignKeyInfo struct {
	ConstraintName string
	ColumnName     string
	RefTable       string
	RefColumn      string
}

// ViewInfo represents a source view.
type ViewInfo struct {
	Name       string
	Definition string // CREATE OR REPLACE VIEW ... AS ...
}

// SequenceInfo represents a source sequence.
type SequenceInfo struct {
	Name       string
	TableName  string // owning table
	ColumnName string // owning column
}

// MigrationStats tracks migration progress.
type MigrationStats struct {
	Users        int      `json:"users"`
	OAuthLinks   int      `json:"oauthLinks"`
	Policies     int      `json:"policies"`
	Tables       int      `json:"tables"`
	Views        int      `json:"views"`
	Records      int      `json:"records"`
	Sequences    int      `json:"sequences"`
	StorageFiles int      `json:"storageFiles"`
	StorageBytes int64    `json:"storageBytes"`
	Skipped      int      `json:"skipped"`
	Errors       []string `json:"errors,omitempty"`
}

// MigrationOptions configures the Supabase migration process.
type MigrationOptions struct {
	SourceURL         string // Supabase PostgreSQL connection URL
	TargetURL         string // AYB PostgreSQL connection URL
	DryRun            bool
	Force             bool // allow migration when _ayb_users is not empty
	Verbose           bool
	SkipRLS           bool   // skip RLS policy rewriting
	SkipOAuth         bool   // skip OAuth identity migration
	SkipData          bool   // skip data table migration
	SkipStorage       bool   // skip storage file migration
	IncludeAnonymous  bool   // include is_anonymous users (default: skip)
	StorageExportPath string // local directory containing exported Supabase storage files
	StoragePath       string // destination path for AYB storage (default: ./ayb_storage)
	Progress          migrate.ProgressReporter
}
