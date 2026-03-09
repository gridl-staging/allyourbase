package sbmigrate

import (
	"fmt"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestNewMigratorMissingSourceURL(t *testing.T) {
	t.Parallel()
	_, err := NewMigrator(MigrationOptions{
		TargetURL: "postgres://localhost/ayb",
	})
	testutil.ErrorContains(t, err, "source database URL is required")
}

func TestNewMigratorMissingTargetURL(t *testing.T) {
	t.Parallel()
	_, err := NewMigrator(MigrationOptions{
		SourceURL: "postgres://localhost/supabase",
	})
	testutil.ErrorContains(t, err, "target database URL is required")
}

func TestNewMigratorBadSourceURL(t *testing.T) {
	t.Parallel()
	_, err := NewMigrator(MigrationOptions{
		SourceURL: "postgres://invalid:5432/nonexistent",
		TargetURL: "postgres://localhost/ayb",
	})
	testutil.ErrorContains(t, err, "source database")
}

func TestExtractString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data map[string]any
		keys []string
		want string
	}{
		{
			name: "first key match",
			data: map[string]any{"sub": "12345", "email": "a@b.com"},
			keys: []string{"sub"},
			want: "12345",
		},
		{
			name: "second key fallback",
			data: map[string]any{"full_name": "John Doe"},
			keys: []string{"name", "full_name"},
			want: "John Doe",
		},
		{
			name: "no match returns empty",
			data: map[string]any{"sub": "12345"},
			keys: []string{"email"},
			want: "",
		},
		{
			name: "empty string value skipped",
			data: map[string]any{"name": "", "full_name": "Jane"},
			keys: []string{"name", "full_name"},
			want: "Jane",
		},
		{
			name: "non-string value skipped",
			data: map[string]any{"sub": 12345},
			keys: []string{"sub"},
			want: "",
		},
		{
			name: "nil map returns empty",
			data: nil,
			keys: []string{"sub"},
			want: "",
		},
		{
			name: "no keys returns empty",
			data: map[string]any{"sub": "12345"},
			keys: []string{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractString(tt.data, tt.keys...)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestPrintStats(t *testing.T) {
	t.Parallel()
	t.Run("includes data fields when non-zero", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		m := &Migrator{
			output: &buf,
			stats: MigrationStats{
				Tables:     5,
				Views:      2,
				Records:    1000,
				Sequences:  3,
				Users:      10,
				OAuthLinks: 5,
				Policies:   2,
			},
		}
		m.printStats()
		out := buf.String()
		testutil.Contains(t, out, "Tables:     5")
		testutil.Contains(t, out, "Views:      2")
		testutil.Contains(t, out, "Records:    1000")
		testutil.Contains(t, out, "Sequences:  3")
		testutil.Contains(t, out, "Users:      10")
		testutil.Contains(t, out, "OAuth:      5")
		testutil.Contains(t, out, "RLS:        2")
	})

	t.Run("omits data fields when zero", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		m := &Migrator{
			output: &buf,
			stats: MigrationStats{
				Users: 10,
			},
		}
		m.printStats()
		out := buf.String()
		testutil.False(t, strings.Contains(out, "Tables:"), "should not show Tables when zero")
		testutil.False(t, strings.Contains(out, "Views:"), "should not show Views when zero")
		testutil.False(t, strings.Contains(out, "Records:"), "should not show Records when zero")
		testutil.False(t, strings.Contains(out, "Sequences:"), "should not show Sequences when zero")
		testutil.Contains(t, out, "Users:      10")
	})

	t.Run("shows errors", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		m := &Migrator{
			output: &buf,
			stats: MigrationStats{
				Errors: []string{"something went wrong"},
			},
		}
		m.printStats()
		out := buf.String()
		testutil.Contains(t, out, "Errors:     1")
		testutil.Contains(t, out, "something went wrong")
	})
}

func TestRedactURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"strips user:pass", "postgres://user:secret@db.xxx.supabase.co:5432/postgres", "postgres://db.xxx.supabase.co:5432/postgres"},
		{"strips user only", "postgres://user@localhost:5432/db", "postgres://localhost:5432/db"},
		{"no credentials unchanged", "postgres://localhost:5432/db", "postgres://localhost:5432/db"},
		{"non-URL returns as-is", "not a url %%", "not a url %%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := redactURL(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestBuildAuthUsersCountQuery(t *testing.T) {
	t.Parallel()
	t.Run("filters anonymous when column exists", func(t *testing.T) {
		t.Parallel()
		query := buildAuthUsersCountQuery(false, true, true)
		testutil.Contains(t, query, "is_anonymous = false")
		testutil.Contains(t, query, "deleted_at IS NULL")
	})

	t.Run("does not reference is_anonymous when column missing", func(t *testing.T) {
		t.Parallel()
		query := buildAuthUsersCountQuery(false, false, true)
		testutil.False(t, strings.Contains(query, "is_anonymous"), "query should not reference is_anonymous")
	})

	t.Run("does not reference deleted_at when column missing", func(t *testing.T) {
		t.Parallel()
		query := buildAuthUsersCountQuery(false, false, false)
		testutil.False(t, strings.Contains(query, "deleted_at"), "query should not reference deleted_at")
	})
}

func TestBuildAuthUsersSelectQuery(t *testing.T) {
	t.Parallel()
	t.Run("uses source is_anonymous column when present", func(t *testing.T) {
		t.Parallel()
		query := buildAuthUsersSelectQuery(false, true, true, "email_confirmed_at")
		testutil.Contains(t, query, "COALESCE(is_anonymous, false)")
		testutil.Contains(t, query, "is_anonymous = false")
		testutil.Contains(t, query, "deleted_at IS NULL")
		testutil.Contains(t, query, "email_confirmed_at AS email_confirmed_at")
	})

	t.Run("degrades when is_anonymous is absent", func(t *testing.T) {
		t.Parallel()
		query := buildAuthUsersSelectQuery(false, false, true, "email_confirmed_at")
		testutil.Contains(t, query, "false AS is_anonymous")
		testutil.False(t, strings.Contains(query, "is_anonymous = false"), "query should not filter on missing column")
	})

	t.Run("degrades when deleted_at is absent", func(t *testing.T) {
		t.Parallel()
		query := buildAuthUsersSelectQuery(false, true, false, "email_confirmed_at")
		testutil.False(t, strings.Contains(query, "deleted_at"), "query should not filter on missing deleted_at")
	})

	t.Run("falls back to confirmed_at expression", func(t *testing.T) {
		t.Parallel()
		query := buildAuthUsersSelectQuery(false, false, false, "confirmed_at")
		testutil.Contains(t, query, "confirmed_at AS email_confirmed_at")
	})
}

func TestBuildOAuthIdentitiesQuery(t *testing.T) {
	t.Parallel()
	t.Run("uses identity_data and created_at when present", func(t *testing.T) {
		t.Parallel()
		query := buildOAuthIdentitiesQuery(true, true, true, true)
		testutil.Contains(t, query, "COALESCE(i.identity_data::text, '{}')")
		testutil.Contains(t, query, "i.created_at")
		testutil.Contains(t, query, "u.deleted_at IS NULL")
	})

	t.Run("falls back to provider_id when identity_data is missing", func(t *testing.T) {
		t.Parallel()
		query := buildOAuthIdentitiesQuery(false, true, false, true)
		testutil.Contains(t, query, "jsonb_build_object")
		testutil.Contains(t, query, "i.provider_id::text")
		testutil.Contains(t, query, "NULL::timestamptz")
		testutil.Contains(t, query, "ORDER BY i.user_id")
	})

	t.Run("falls back to empty identity payload when both columns are missing", func(t *testing.T) {
		t.Parallel()
		query := buildOAuthIdentitiesQuery(false, false, false, false)
		testutil.Contains(t, query, "'{}'::text")
		testutil.False(t, strings.Contains(query, "i.identity_data"), "query should not reference missing identity_data column")
		testutil.False(t, strings.Contains(query, "deleted_at"), "query should not reference missing deleted_at column")
	})
}

func TestIsSkippableSchemaTableError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "undefined function is skippable",
			err:  &pgconn.PgError{Code: "42883"},
			want: true,
		},
		{
			name: "undefined object is skippable",
			err:  &pgconn.PgError{Code: "42704"},
			want: true,
		},
		{
			name: "undefined table is skippable",
			err:  &pgconn.PgError{Code: "42P01"},
			want: true,
		},
		{
			name: "feature not supported is skippable",
			err:  &pgconn.PgError{Code: "0A000"},
			want: true,
		},
		{
			name: "other postgres error is not skippable",
			err:  &pgconn.PgError{Code: "22023"},
			want: false,
		},
		{
			name: "non postgres error is not skippable",
			err:  fmt.Errorf("boom"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isSkippableSchemaTableError(tt.err)
			testutil.Equal(t, tt.want, got)
		})
	}
}
