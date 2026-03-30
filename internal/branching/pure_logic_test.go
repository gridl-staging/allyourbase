package branching

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ExtractDBNameFromURL
// ---------------------------------------------------------------------------

func TestExtractDBNameFromURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "standard postgres URL",
			url:  "postgres://user:pass@localhost:5432/mydb",
			want: "mydb",
		},
		{
			name: "URL with query params",
			url:  "postgres://user:pass@localhost:5432/testdb?sslmode=disable",
			want: "testdb",
		},
		{
			name: "URL with path only",
			url:  "postgres://localhost/dbname",
			want: "dbname",
		},
		{
			name:    "URL with no database name",
			url:     "postgres://localhost/",
			wantErr: true,
		},
		{
			name:    "URL with no path at all",
			url:     "postgres://localhost",
			wantErr: true,
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractDBNameFromURL(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ExtractDBNameFromURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// Verify the error message mentions the right context.
func TestExtractDBNameFromURL_ErrorMessage(t *testing.T) {
	t.Parallel()

	_, err := ExtractDBNameFromURL("postgres://localhost/")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "database name") {
		t.Errorf("error should mention 'database name', got: %v", err)
	}
}
