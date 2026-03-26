package server

import (
	"net/url"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNormalizeAuditUserID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty", input: "", want: "", wantErr: false},
		{name: "trimmed valid uuid", input: " 11111111-1111-1111-1111-111111111111 ", want: "11111111-1111-1111-1111-111111111111", wantErr: false},
		{name: "invalid uuid", input: "not-a-uuid", want: "", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeAuditUserID(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseAdminAuditFiltersValid(t *testing.T) {
	t.Parallel()

	filters, badRequestMessage := parseAdminAuditFilters(url.Values{
		"table":     {" orders "},
		"user_id":   {" 11111111-1111-1111-1111-111111111111 "},
		"operation": {" update "},
		"limit":     {"600"},
		"offset":    {"-7"},
		"from":      {"2026-03-01"},
		"to":        {"2026-03-02"},
	})

	testutil.Equal(t, "", badRequestMessage)
	testutil.Equal(t, "orders", filters.tableName)
	testutil.Equal(t, "11111111-1111-1111-1111-111111111111", filters.userID)
	testutil.Equal(t, "UPDATE", filters.operation)
	testutil.Equal(t, 500, filters.limit)
	testutil.Equal(t, 0, filters.offset)
	testutil.Equal(t, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), filters.fromTime)
	testutil.Equal(t, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), filters.toTime)
	testutil.True(t, filters.toDateOnly, "date-only to filter should be tracked for exclusive upper bound")
}

func TestParseAdminAuditFiltersRejectsInvalidOperation(t *testing.T) {
	t.Parallel()

	_, badRequestMessage := parseAdminAuditFilters(url.Values{"operation": {"merge"}})
	testutil.Equal(t, "invalid operation filter; must be one of: INSERT, UPDATE, DELETE", badRequestMessage)
}
