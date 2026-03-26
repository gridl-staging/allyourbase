// Package pbmigrate Types for PocketBase to PostgreSQL data migration, including collection definitions, field schemas, records, and migration configuration.
package pbmigrate

import (
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

// PBCollection represents a PocketBase collection from the _collections table.
// Rules are nil for admin-only access, empty strings for open access, or
// expression strings for conditional access. Type is "base", "view", or "auth".
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

// RuleStatus classifies a PocketBase rule for RLS conversion.
type RuleStatus int

const (
	// RuleStatusConvertible means the rule can be automatically converted to PostgreSQL RLS.
	RuleStatusConvertible RuleStatus = iota
	// RuleStatusLocked means the rule is nil (admin-only), no policy needed.
	RuleStatusLocked
	// RuleStatusUnsupported means the rule contains PocketBase-specific syntax
	// that cannot be automatically converted to PostgreSQL RLS.
	RuleStatusUnsupported
)

// RuleClassification is the result of classifying a PocketBase rule expression.
// Every rule is classified into exactly one bucket via classifyRule/convertRuleExpression.
type RuleClassification struct {
	Status     RuleStatus
	PgExpr     string // converted PostgreSQL expression; non-empty only when Status == RuleStatusConvertible
	Rule       string // original PocketBase rule expression
	Diagnostic string // human-readable explanation; non-empty when Status != RuleStatusConvertible
}

// RLSDiagnostic records a non-convertible RLS rule encountered during migration or analysis.
type RLSDiagnostic struct {
	Collection string
	Action     string
	Rule       string
	Message    string
}

// ruleAction pairs an action name with a collection's rule pointer.
type ruleAction struct {
	name string
	rule *string
}

// ruleActions returns the four CRUD rule actions for a collection.
// ViewRule is intentionally excluded: both ListRule and ViewRule map to SELECT
// in PostgreSQL, so we use the more restrictive ListRule for the SELECT policy.
func ruleActions(coll PBCollection) []ruleAction {
	return []ruleAction{
		{"list", coll.ListRule},
		{"create", coll.CreateRule},
		{"update", coll.UpdateRule},
		{"delete", coll.DeleteRule},
	}
}

// MigrationStats tracks migration progress
type MigrationStats struct {
	Collections    int
	Tables         int
	Views          int
	Records        int
	AuthUsers      int
	Files          int
	Policies       int
	Errors         []string
	FailedFiles    []string        // exact "collection/relpath" for each file copy failure
	RLSDiagnostics []RLSDiagnostic // non-convertible rules found during RLS generation
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
