package ai

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// IsValidAssistantMode
// ---------------------------------------------------------------------------

func TestIsValidAssistantMode(t *testing.T) {
	t.Parallel()

	// All four supported modes must be accepted.
	valid := []AssistantMode{
		AssistantModeSQL,
		AssistantModeRLS,
		AssistantModeMigration,
		AssistantModeGeneral,
	}
	for _, m := range valid {
		if !IsValidAssistantMode(m) {
			t.Errorf("IsValidAssistantMode(%q) = false, want true", m)
		}
	}

	// Invalid modes — empty, typos, case variants, arbitrary strings.
	invalid := []AssistantMode{
		"",
		"SQL",        // uppercase — mode constants are lowercase
		"Sql",        // mixed case
		"chat",       // not a real mode
		"migration ", // trailing space
		" rls",       // leading space
	}
	for _, m := range invalid {
		if IsValidAssistantMode(m) {
			t.Errorf("IsValidAssistantMode(%q) = true, want false", m)
		}
	}
}

// ---------------------------------------------------------------------------
// NormalizeAssistantMode
// ---------------------------------------------------------------------------

func TestNormalizeAssistantMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  AssistantMode
	}{
		// Exact lowercase — pass through unchanged.
		{"sql", AssistantModeSQL},
		{"rls", AssistantModeRLS},
		{"migration", AssistantModeMigration},
		{"general", AssistantModeGeneral},

		// Uppercase and mixed case are lowered.
		{"SQL", AssistantModeSQL},
		{"Rls", AssistantModeRLS},
		{"MIGRATION", AssistantModeMigration},
		{"General", AssistantModeGeneral},

		// Leading/trailing whitespace is trimmed.
		{"  sql  ", AssistantModeSQL},
		{"\tgeneral\n", AssistantModeGeneral},

		// Unknown modes fall back to general.
		{"", AssistantModeGeneral},
		{"chat", AssistantModeGeneral},
		{"unknown", AssistantModeGeneral},
		{"   ", AssistantModeGeneral}, // whitespace-only
	}
	for _, tc := range tests {
		got := NormalizeAssistantMode(AssistantMode(tc.input))
		if got != tc.want {
			t.Errorf("NormalizeAssistantMode(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// firstNonEmpty
// ---------------------------------------------------------------------------

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"first is non-empty", []string{"a", "b"}, "a"},
		{"first is empty, second non-empty", []string{"", "b"}, "b"},
		{"first is whitespace-only, second non-empty", []string{"   ", "b"}, "b"},
		{"all empty", []string{"", "", ""}, ""},
		{"all whitespace", []string{" ", "\t", "\n"}, ""},
		{"no arguments", []string{}, ""},
		{"single non-empty", []string{"only"}, "only"},
		{"tabs and newlines skipped", []string{"\t\n", "real"}, "real"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := firstNonEmpty(tc.values...)
			if got != tc.want {
				t.Errorf("firstNonEmpty(%v) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// assembleAssistantUserPrompt
// ---------------------------------------------------------------------------

func TestAssembleAssistantUserPrompt(t *testing.T) {
	t.Parallel()

	t.Run("basic assembly", func(t *testing.T) {
		t.Parallel()
		got := assembleAssistantUserPrompt(AssistantModeSQL, "CREATE TABLE users...", "Show me all users")
		// Must contain the mode, schema context, and query.
		if got == "" {
			t.Fatal("expected non-empty prompt")
		}
		for _, want := range []string{"sql", "CREATE TABLE users...", "Show me all users"} {
			if !strings.Contains(got, want) {
				t.Errorf("prompt missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("trims whitespace from query", func(t *testing.T) {
		t.Parallel()
		got := assembleAssistantUserPrompt(AssistantModeGeneral, "", "  padded query  ")
		// Query whitespace should be trimmed in the output.
		if strings.Contains(got, "  padded query  ") {
			t.Error("expected query whitespace to be trimmed")
		}
		if !strings.Contains(got, "padded query") {
			t.Error("expected trimmed query in output")
		}
	})

	t.Run("empty schema context still includes structure", func(t *testing.T) {
		t.Parallel()
		got := assembleAssistantUserPrompt(AssistantModeRLS, "", "enable RLS")
		// Even with empty schema, the prompt structure should be present.
		if !strings.Contains(got, "Schema context:") {
			t.Error("expected 'Schema context:' section header")
		}
		if !strings.Contains(got, "enable RLS") {
			t.Error("expected query text in output")
		}
	})
}

// ---------------------------------------------------------------------------
// detectDestructiveWarnings
// ---------------------------------------------------------------------------

func TestDetectDestructiveWarnings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantLen  int    // expected number of warnings
		contains string // at least one warning must contain this substring (empty = skip check)
	}{
		{
			name:     "DROP DATABASE triggers warning",
			input:    "DROP DATABASE production;",
			wantLen:  1,
			contains: "DROP DATABASE",
		},
		{
			name:     "DROP TABLE triggers warning",
			input:    "DROP TABLE users;",
			wantLen:  1,
			contains: "DROP TABLE",
		},
		{
			name:     "TRUNCATE triggers warning",
			input:    "TRUNCATE orders;",
			wantLen:  1,
			contains: "TRUNCATE",
		},
		{
			name:     "DELETE without WHERE triggers warning",
			input:    "DELETE FROM users;",
			wantLen:  1,
			contains: "DELETE without WHERE",
		},
		{
			name:    "DELETE with WHERE is safe",
			input:   "DELETE FROM users WHERE id = 1;",
			wantLen: 0,
		},
		{
			name:    "SELECT is safe",
			input:   "SELECT * FROM users;",
			wantLen: 0,
		},
		{
			name:     "multiple destructive statements",
			input:    "DROP DATABASE test; TRUNCATE users; DELETE FROM orders;",
			wantLen:  3, // DROP DATABASE + TRUNCATE + DELETE without WHERE
			contains: "",
		},
		{
			name:    "case insensitive — lowercase drop database",
			input:   "drop database mydb;",
			wantLen: 1,
		},
		{
			name:    "empty input",
			input:   "",
			wantLen: 0,
		},
		{
			name:    "benign DDL is safe",
			input:   "CREATE TABLE new_table (id int); ALTER TABLE users ADD COLUMN email text;",
			wantLen: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := detectDestructiveWarnings(tc.input)
			if len(got) != tc.wantLen {
				t.Fatalf("detectDestructiveWarnings(%q) returned %d warnings, want %d: %v",
					tc.input, len(got), tc.wantLen, got)
			}
			if tc.contains != "" {
				found := false
				for _, w := range got {
					if strings.Contains(w, tc.contains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no warning contains %q in: %v", tc.contains, got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// hasDeleteWithoutWhere
// ---------------------------------------------------------------------------

func TestHasDeleteWithoutWhere(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"DELETE without WHERE", "delete from users;", true},
		{"DELETE with WHERE", "delete from users where id = 1;", false},
		{"DELETE without WHERE, no semicolon", "delete from users", true},
		{"multiple DELETEs, one without WHERE", "delete from a where x=1; delete from b;", true},
		{"multiple DELETEs, all with WHERE", "delete from a where x=1; delete from b where y=2;", false},
		{"no DELETE at all", "select * from users;", false},
		{"empty input", "", false},
		// Case insensitivity — the function receives lowered input from detectDestructiveWarnings.
		{"lowercase", "delete from users;", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := hasDeleteWithoutWhere(tc.input)
			if got != tc.want {
				t.Errorf("hasDeleteWithoutWhere(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normalizeBreakerConfig
// ---------------------------------------------------------------------------

func TestNormalizeBreakerConfig(t *testing.T) {
	t.Parallel()

	t.Run("zero values get defaults", func(t *testing.T) {
		t.Parallel()
		got := normalizeBreakerConfig(BreakerConfig{})
		if got.FailureThreshold != 5 {
			t.Errorf("FailureThreshold = %d, want 5", got.FailureThreshold)
		}
		if got.OpenDuration != 30*time.Second {
			t.Errorf("OpenDuration = %v, want 30s", got.OpenDuration)
		}
		if got.HalfOpenMaxRequests != 1 {
			t.Errorf("HalfOpenMaxRequests = %d, want 1", got.HalfOpenMaxRequests)
		}
	})

	t.Run("negative values get defaults", func(t *testing.T) {
		t.Parallel()
		got := normalizeBreakerConfig(BreakerConfig{
			FailureThreshold:    -1,
			OpenDuration:        -time.Second,
			HalfOpenMaxRequests: -5,
		})
		if got.FailureThreshold != 5 {
			t.Errorf("FailureThreshold = %d, want 5", got.FailureThreshold)
		}
		if got.OpenDuration != 30*time.Second {
			t.Errorf("OpenDuration = %v, want 30s", got.OpenDuration)
		}
		if got.HalfOpenMaxRequests != 1 {
			t.Errorf("HalfOpenMaxRequests = %d, want 1", got.HalfOpenMaxRequests)
		}
	})

	t.Run("positive values are preserved", func(t *testing.T) {
		t.Parallel()
		cfg := BreakerConfig{
			FailureThreshold:    10,
			OpenDuration:        time.Minute,
			HalfOpenMaxRequests: 3,
		}
		got := normalizeBreakerConfig(cfg)
		if got.FailureThreshold != 10 {
			t.Errorf("FailureThreshold = %d, want 10", got.FailureThreshold)
		}
		if got.OpenDuration != time.Minute {
			t.Errorf("OpenDuration = %v, want 1m", got.OpenDuration)
		}
		if got.HalfOpenMaxRequests != 3 {
			t.Errorf("HalfOpenMaxRequests = %d, want 3", got.HalfOpenMaxRequests)
		}
	})

	t.Run("partial defaults — only missing fields filled", func(t *testing.T) {
		t.Parallel()
		// Provide FailureThreshold but leave the rest zero.
		got := normalizeBreakerConfig(BreakerConfig{FailureThreshold: 3})
		if got.FailureThreshold != 3 {
			t.Errorf("FailureThreshold = %d, want 3 (should be preserved)", got.FailureThreshold)
		}
		if got.OpenDuration != 30*time.Second {
			t.Errorf("OpenDuration = %v, want 30s (should default)", got.OpenDuration)
		}
		if got.HalfOpenMaxRequests != 1 {
			t.Errorf("HalfOpenMaxRequests = %d, want 1 (should default)", got.HalfOpenMaxRequests)
		}
	})
}

// ---------------------------------------------------------------------------
// AssistantMode constants — guard against accidental renames
// ---------------------------------------------------------------------------

func TestAssistantModeConstants(t *testing.T) {
	t.Parallel()

	// These values are used in API requests and stored in history — renaming breaks compatibility.
	consts := map[AssistantMode]string{
		AssistantModeSQL:       "sql",
		AssistantModeRLS:       "rls",
		AssistantModeMigration: "migration",
		AssistantModeGeneral:   "general",
	}
	for c, want := range consts {
		if string(c) != want {
			t.Errorf("AssistantMode constant = %q, want %q", c, want)
		}
	}
}

// ---------------------------------------------------------------------------
// BreakerState constants — guard against accidental renames
// ---------------------------------------------------------------------------

func TestBreakerStateConstants(t *testing.T) {
	t.Parallel()

	consts := map[BreakerState]string{
		BreakerStateClosed:   "closed",
		BreakerStateOpen:     "open",
		BreakerStateHalfOpen: "half_open",
	}
	for c, want := range consts {
		if string(c) != want {
			t.Errorf("BreakerState constant = %q, want %q", c, want)
		}
	}
}
