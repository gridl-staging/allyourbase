package pbmigrate

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewMigrator_MissingSourcePath(t *testing.T) {
	t.Parallel()
	_, err := NewMigrator(MigrationOptions{
		DatabaseURL: "postgres://localhost/test",
	})
	testutil.ErrorContains(t, err, "source path is required")
}

func TestNewMigrator_MissingDatabaseURL(t *testing.T) {
	t.Parallel()
	_, err := NewMigrator(MigrationOptions{
		SourcePath: "/tmp/pb_data",
	})
	testutil.ErrorContains(t, err, "database URL is required")
}

func TestNewMigrator_InvalidSourcePath(t *testing.T) {
	t.Parallel()
	_, err := NewMigrator(MigrationOptions{
		SourcePath:  "/nonexistent/path",
		DatabaseURL: "postgres://localhost/test",
	})
	testutil.ErrorContains(t, err, "failed to create reader")
}

func TestJoinQuoted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "single column",
			input:    []string{"id"},
			expected: `"id"`,
		},
		{
			name:     "multiple columns",
			input:    []string{"id", "name", "email"},
			expected: `"id", "name", "email"`,
		},
		{
			name:     "empty array",
			input:    []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := joinQuoted(tt.input)
			testutil.Equal(t, tt.expected, result)
		})
	}
}

func TestJoin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []string
		sep      string
		expected string
	}{
		{
			name:     "comma separated",
			input:    []string{"a", "b", "c"},
			sep:      ", ",
			expected: "a, b, c",
		},
		{
			name:     "space separated",
			input:    []string{"foo", "bar"},
			sep:      " ",
			expected: "foo bar",
		},
		{
			name:     "empty array",
			input:    []string{},
			sep:      ",",
			expected: "",
		},
		{
			name:     "single element",
			input:    []string{"only"},
			sep:      ",",
			expected: "only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := join(tt.input, tt.sep)
			testutil.Equal(t, tt.expected, result)
		})
	}
}
