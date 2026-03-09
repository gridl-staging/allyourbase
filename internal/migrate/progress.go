// Package migrate provides shared infrastructure for migration tools
// (PocketBase, Supabase, Firebase).
package migrate

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/cli/ui"
)

// Phase represents a named migration phase (e.g., "Schema", "Data", "Auth").
type Phase struct {
	Name  string // e.g., "Schema", "Data", "Auth users", "RLS policies", "Storage files"
	Index int    // 1-based index (1 of 5)
	Total int    // total number of phases
}

// ProgressReporter receives progress updates from a migrator.
type ProgressReporter interface {
	// StartPhase is called when a new migration phase begins.
	StartPhase(phase Phase, totalItems int)
	// Progress is called as items are processed within a phase.
	Progress(phase Phase, completed int, totalItems int)
	// CompletePhase is called when a phase finishes.
	CompletePhase(phase Phase, totalItems int, elapsed time.Duration)
	// Warn reports a non-fatal warning.
	Warn(msg string)
}

// CLIReporter prints progress to a terminal writer.
type CLIReporter struct {
	w  io.Writer
	mu sync.Mutex
}

// NewCLIReporter creates a reporter that writes to w.
func NewCLIReporter(w io.Writer) *CLIReporter {
	return &CLIReporter{w: w}
}

func (r *CLIReporter) StartPhase(phase Phase, totalItems int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.w, "  [%d/%d] %-16s", phase.Index, phase.Total, phase.Name)
}

func (r *CLIReporter) Progress(phase Phase, completed int, totalItems int) {
	// For CLI, we overwrite the current line with progress
	r.mu.Lock()
	defer r.mu.Unlock()
	if totalItems > 0 {
		fmt.Fprintf(r.w, "\r  [%d/%d] %-16s %d/%d",
			phase.Index, phase.Total, phase.Name, completed, totalItems)
	}
}

func (r *CLIReporter) CompletePhase(phase Phase, totalItems int, elapsed time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	label := fmt.Sprintf("%d items", totalItems)
	if totalItems == 0 {
		label = "skipped"
	}
	fmt.Fprintf(r.w, "\r  [%d/%d] %-16s %-20s %s  (%s)\n",
		phase.Index, phase.Total, phase.Name, label,
		ui.StyleSuccess.Render(ui.SymbolCheck), formatDuration(elapsed))
}

func (r *CLIReporter) Warn(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.w, "  %s %s\n", ui.StyleWarning.Render(ui.SymbolWarning), msg)
}

// NopReporter discards all progress updates (used in tests and --json mode).
type NopReporter struct{}

func (NopReporter) StartPhase(Phase, int)                   {}
func (NopReporter) Progress(Phase, int, int)                {}
func (NopReporter) CompletePhase(Phase, int, time.Duration) {}
func (NopReporter) Warn(string)                             {}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// SourceType identifies the kind of migration source.
type SourceType int

const (
	SourceUnknown    SourceType = iota
	SourcePocketBase            // Local pb_data directory
	SourceSupabase              // Supabase Postgres connection
	SourceFirebase              // Firebase project
	SourcePostgres              // Generic Postgres connection
)

func (s SourceType) String() string {
	switch s {
	case SourcePocketBase:
		return "PocketBase"
	case SourceSupabase:
		return "Supabase"
	case SourceFirebase:
		return "Firebase"
	case SourcePostgres:
		return "PostgreSQL"
	default:
		return "unknown"
	}
}

// DetectSource determines the migration source type from a --from value.
//
// Detection rules:
//   - Path to directory containing data.db → PocketBase
//   - postgres:// URL containing "supabase" → Supabase
//   - firebase:// URL or path to .json service account → Firebase
//   - Any other postgres:// or postgresql:// URL → generic Postgres
func DetectSource(from string) SourceType {
	// Firebase URL scheme
	if strings.HasPrefix(from, "firebase://") {
		return SourceFirebase
	}

	// Firebase service account JSON file
	if strings.HasSuffix(from, ".json") {
		return SourceFirebase
	}

	// PostgreSQL URL
	if strings.HasPrefix(from, "postgres://") || strings.HasPrefix(from, "postgresql://") {
		if strings.Contains(from, "supabase") {
			return SourceSupabase
		}
		return SourcePostgres
	}

	// Check if it's a directory (PocketBase pb_data)
	// We don't check the filesystem here — the caller validates.
	// Heuristic: if it looks like a path, treat as PocketBase.
	if !strings.Contains(from, "://") {
		return SourcePocketBase
	}

	return SourceUnknown
}

// AnalysisReport summarizes what a migration will do, shown before proceeding.
type AnalysisReport struct {
	SourceType    string   `json:"sourceType"`
	SourceInfo    string   `json:"sourceInfo"` // e.g., "PocketBase v0.22, SQLite 7.2 MB"
	Tables        int      `json:"tables"`
	Views         int      `json:"views"`
	Records       int      `json:"records"`
	AuthUsers     int      `json:"authUsers"`
	OAuthLinks    int      `json:"oauthLinks"`
	RLSPolicies   int      `json:"rlsPolicies"`
	Files         int      `json:"files"`
	FileSizeBytes int64    `json:"fileSizeBytes"`
	Warnings      []string `json:"warnings,omitempty"`
}

// PrintReport writes a formatted pre-flight report to w.
func (r *AnalysisReport) PrintReport(w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  AYB Migration Report — %s\n", r.SourceType)
	fmt.Fprintln(w)
	if r.SourceInfo != "" {
		fmt.Fprintf(w, "  Source: %s\n", r.SourceInfo)
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "  Tables:       %d\n", r.Tables)
	if r.Views > 0 {
		fmt.Fprintf(w, "  Views:        %d\n", r.Views)
	}
	fmt.Fprintf(w, "  Records:      %d\n", r.Records)
	if r.AuthUsers > 0 {
		fmt.Fprintf(w, "  Auth users:   %d\n", r.AuthUsers)
	}
	if r.OAuthLinks > 0 {
		fmt.Fprintf(w, "  OAuth links:  %d\n", r.OAuthLinks)
	}
	if r.RLSPolicies > 0 {
		fmt.Fprintf(w, "  RLS policies: %d\n", r.RLSPolicies)
	}
	if r.Files > 0 {
		fmt.Fprintf(w, "  Files:        %d (%s)\n", r.Files, FormatBytes(r.FileSizeBytes))
	}
	fmt.Fprintln(w)

	if len(r.Warnings) > 0 {
		fmt.Fprintln(w, "  Warnings:")
		for _, w2 := range r.Warnings {
			fmt.Fprintf(w, "    - %s\n", w2)
		}
		fmt.Fprintln(w)
	}
}

// FormatBytes formats a byte count as a human-readable string (B, KB, MB, GB).
func FormatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// ValidationSummary compares source and target counts after migration.
type ValidationSummary struct {
	SourceLabel string
	TargetLabel string
	Rows        []ValidationRow
	Warnings    []string
}

// ValidationRow is a single line in the validation summary.
type ValidationRow struct {
	Label       string
	SourceCount int
	TargetCount int
}

// PrintSummary writes a formatted validation summary to w.
func (v *ValidationSummary) PrintSummary(w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Validation Summary")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-28s  %-20s\n", v.SourceLabel, v.TargetLabel)
	fmt.Fprintf(w, "  %-28s  %-20s\n", strings.Repeat("-", 24), strings.Repeat("-", 16))

	allMatch := true
	for _, row := range v.Rows {
		match := ui.StyleSuccess.Render(ui.SymbolCheck)
		if row.SourceCount != row.TargetCount {
			match = ui.StyleError.Render("MISMATCH")
			allMatch = false
		}
		fmt.Fprintf(w, "  %-16s %6d  %s  %6d  %s\n",
			row.Label, row.SourceCount, ui.SymbolArrow, row.TargetCount, match)
	}
	fmt.Fprintln(w)

	if allMatch {
		fmt.Fprintf(w, "  %s All counts match.\n", ui.StyleSuccess.Render(ui.SymbolCheck))
	}

	if len(v.Warnings) > 0 {
		fmt.Fprintf(w, "  %s Warnings:\n", ui.StyleWarning.Render(ui.SymbolWarning))
		for _, warn := range v.Warnings {
			fmt.Fprintf(w, "    %s %s\n", ui.StyleWarning.Render(ui.SymbolArrow), warn)
		}
	}
	fmt.Fprintln(w)
}
