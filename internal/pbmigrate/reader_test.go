package pbmigrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewReader_InvalidPath(t *testing.T) {
	t.Parallel()
	_, err := NewReader("/nonexistent/path")
	testutil.ErrorContains(t, err, "source path does not exist")
}

func TestNewReader_MissingDataDB(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	_, err := NewReader(tmpDir)
	testutil.ErrorContains(t, err, "data.db not found")
}

func TestNewReader_ValidPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create empty data.db — sql.Open succeeds on empty files (lazy open).
	dataDB := filepath.Join(tmpDir, "data.db")
	f, err := os.Create(dataDB)
	testutil.NoError(t, err)
	f.Close()

	// Path validation should pass, and sql.Open should succeed (lazy init).
	reader, err := NewReader(tmpDir)
	testutil.NoError(t, err)
	testutil.NotNil(t, reader)
	reader.Close()
}

func TestSanitizeIdentifier_SpecialCharacters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"normal", `"normal"`},
		{"with-dash", `"with-dash"`},
		{"with space", `"with space"`},
		{"with.dot", `"with.dot"`},
		{"with$dollar", `"with$dollar"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			result := SanitizeIdentifier(tt.input)
			testutil.Equal(t, tt.expected, result)
		})
	}
}
