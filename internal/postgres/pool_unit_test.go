package postgres

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// TestConfigDefaults removed — tested Go zero-value semantics for a plain
// struct, not package behavior. Would always pass regardless of code changes.

func TestNewEmptyURL(t *testing.T) {
	// Empty URL should be rejected before attempting any connection.
	t.Parallel()

	_, err := New(t.Context(), Config{URL: ""}, testutil.DiscardLogger())
	testutil.ErrorContains(t, err, "database URL is required")
}

func TestNewInvalidURLFormat(t *testing.T) {
	// A URL that pgxpool.ParseConfig cannot parse should return a parse error.
	t.Parallel()

	_, err := New(t.Context(), Config{URL: "://bad"}, testutil.DiscardLogger())
	testutil.ErrorContains(t, err, "parsing database URL")
}

func TestNewUnreachableHost(t *testing.T) {
	// A syntactically valid URL pointing at a host that doesn't exist.
	// pgxpool.NewWithConfig or Ping should fail with a connection error.
	t.Parallel()

	_, err := New(t.Context(), Config{
		URL:      "postgresql://nouser:nopass@127.0.0.1:1/nodb?connect_timeout=1",
		MaxConns: 1,
		MinConns: 0,
	}, testutil.DiscardLogger())
	testutil.NotNil(t, err)
	// Verify it's a connection-related error, not a URL parse error.
	// Depending on timing, the error may be from pool creation or ping.
	errMsg := err.Error()
	isConnErr := strings.Contains(errMsg, "creating connection pool") ||
		strings.Contains(errMsg, "database ping failed")
	testutil.True(t, isConnErr, "expected connection error, got: %s", errMsg)
}
