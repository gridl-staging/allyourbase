package server

import (
	"net/url"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestParseAdminRequestLogFiltersValid(t *testing.T) {
	t.Parallel()

	query := url.Values{
		"method": {" post "},
		"path":   {" /api/* "},
		"status": {"201"},
		"limit":  {"600"},
		"offset": {"-1"},
		"from":   {"2026-03-01"},
		"to":     {"2026-03-02"},
	}

	filters, badRequestMessage := parseAdminRequestLogFilters(query)
	testutil.Equal(t, "", badRequestMessage)
	testutil.Equal(t, "POST", filters.method)
	testutil.Equal(t, "/api/*", filters.path)
	testutil.Equal(t, 201, filters.statusCode)
	testutil.Equal(t, 500, filters.limit)
	testutil.Equal(t, 0, filters.offset)
	testutil.Equal(t, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), filters.fromTime)
	testutil.Equal(t, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), filters.toTime)
	testutil.True(t, filters.toDateOnly, "date-only to filter should be tracked for exclusive upper bound")
}

func TestParseAdminRequestLogFiltersRejectsStatusOutsideRange(t *testing.T) {
	t.Parallel()

	filters, badRequestMessage := parseAdminRequestLogFilters(url.Values{"status": {"99"}})
	testutil.Equal(t, "", filters.method)
	testutil.Equal(t, "invalid status; must be an integer 100–599", badRequestMessage)
}
