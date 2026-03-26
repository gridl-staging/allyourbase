package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewIPAllowlist(t *testing.T) {
	t.Parallel()

	allowlist, err := NewIPAllowlist([]string{
		"203.0.113.0/24",
		"2001:db8::/64",
	})
	testutil.NoError(t, err)
	testutil.True(t, allowlist.Allow("203.0.113.10"))
	testutil.True(t, allowlist.Allow("2001:db8::1"))
	testutil.False(t, allowlist.Allow("198.51.100.9"))
	testutil.False(t, allowlist.Allow("bad-ip"))
}

func TestNewIPAllowlist_EmptyListAllowsAll(t *testing.T) {
	t.Parallel()

	allowlist, err := NewIPAllowlist(nil)
	testutil.NoError(t, err)
	testutil.True(t, allowlist.Allow("203.0.113.10"))
	testutil.True(t, allowlist.Allow("2001:db8::10"))
}

func TestNewIPAllowlist_MixedCIDRAndExactIP(t *testing.T) {
	t.Parallel()

	allowlist, err := NewIPAllowlist([]string{"198.51.100.10", "198.51.100.0/24", "bad-entry"})
	testutil.ErrorContains(t, err, "invalid allowlist entry")
	testutil.Nil(t, allowlist)
}

func TestIPAllowlistMiddleware(t *testing.T) {
	t.Parallel()

	allowlist, err := NewIPAllowlist([]string{"198.51.100.10", "203.0.113.0/24"})
	testutil.NoError(t, err)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	allowlist.Middleware(h).ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "198.51.100.9:1234"
	allowlist.Middleware(h).ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)

	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.Equal(t, "access_denied", body["error"])
	testutil.Equal(t, "Your IP address is not in the allowlist", body["message"])
}

func TestClientIPUsesXForwardedForWhenTrustedProxy(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18, 150.172.238.178")
	testutil.Equal(t, "203.0.113.50", ClientIP(req))

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	testutil.Equal(t, "198.51.100.9", ClientIP(req))

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("X-Forwarded-For", "  203.0.113.50 , 70.41.3.18")
	testutil.Equal(t, "203.0.113.50", ClientIP(req))
}

func TestClientIPUsesXRealIPWhenTrustedProxy(t *testing.T) {
	t.Parallel()

	// X-Real-IP is trusted when RemoteAddr is loopback and no XFF is present.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Real-IP", "198.51.100.1")
	testutil.Equal(t, "198.51.100.1", ClientIP(req))

	// X-Real-IP is trusted when RemoteAddr is private (RFC 1918).
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-IP", "203.0.113.50")
	testutil.Equal(t, "203.0.113.50", ClientIP(req))
}

func TestClientIPIgnoresXForwardedForWhenNotProxy(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.9:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("X-Real-IP", "203.0.113.11")
	testutil.Equal(t, "198.51.100.9", ClientIP(req))
}

func TestClientIPFallsBackToRemoteAddr(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[2001:db8::2]:1234"
	testutil.Equal(t, "2001:db8::2", ClientIP(req))

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.77"
	testutil.Equal(t, "198.51.100.77", ClientIP(req))
}

func TestNewIPAllowlist_EmptyEntriesIgnored(t *testing.T) {
	t.Parallel()

	allowlist, err := NewIPAllowlist([]string{"", "  ", "198.51.100.10"})
	testutil.NoError(t, err)
	testutil.True(t, allowlist.Allow("198.51.100.10"))
	testutil.False(t, allowlist.Allow("198.51.100.11"))
}

func TestClientIPSupportsIPv6AndFirstXFF(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::1]:1234"
	req.Header.Set("X-Forwarded-For", "2001:db8::3, 2001:db8::4")
	testutil.Equal(t, "2001:db8::3", ClientIP(req))

	allowlist, err := NewIPAllowlist([]string{"2001:db8::3"})
	testutil.NoError(t, err)
	testutil.True(t, allowlist.Allow("2001:db8::3"))

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:4444"
	req.Header.Set("X-Forwarded-For", "2001:db8::3")
	testutil.Equal(t, "203.0.113.10", ClientIP(req))
}

func TestParseAuditIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want *string
	}{
		{"empty", "", nil},
		{"whitespace", "  ", nil},
		{"plain IPv4", "198.51.100.10", strPtr("198.51.100.10")},
		{"host:port", "198.51.100.10:3456", strPtr("198.51.100.10")},
		{"IPv6 bracketed port", "[2001:db8::1]:8080", strPtr("2001:db8::1")},
		{"IPv6 bare", "2001:db8::1", strPtr("2001:db8::1")},
		{"XFF chain", "198.51.100.10, 203.0.113.20", strPtr("198.51.100.10")},
		{"invalid", "not-an-ip", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAuditIP(tt.raw)
			if tt.want == nil {
				testutil.Nil(t, got)
			} else {
				if got == nil {
					t.Fatalf("expected %q, got nil", *tt.want)
				}
				testutil.Equal(t, *tt.want, *got)
			}
		})
	}
}

func TestAuditIPFromRequest(t *testing.T) {
	t.Parallel()

	// Prefers X-Forwarded-For
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.10, 203.0.113.20")
	ip := AuditIPFromRequest(req)
	if ip == nil {
		t.Fatal("expected parsed IP")
	}
	testutil.Equal(t, "198.51.100.10", *ip)

	// Falls back to RemoteAddr and strips port
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.44:12345"
	ip = AuditIPFromRequest(req)
	if ip == nil {
		t.Fatal("expected parsed IP")
	}
	testutil.Equal(t, "192.0.2.44", *ip)

	// Rejects invalid RemoteAddr
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "not-an-ip"
	ip = AuditIPFromRequest(req)
	testutil.Nil(t, ip)
}

func strPtr(s string) *string { return &s }

func TestIPAllowlistMiddlewareRejectsUntrustedIPv6(t *testing.T) {
	t.Parallel()

	allowlist, err := NewIPAllowlist([]string{"2001:db8::3"})
	testutil.NoError(t, err)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.77:1234"
	req.Header.Set("X-Forwarded-For", "2001:db8::3")
	w := httptest.NewRecorder()

	allowlist.Middleware(h).ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.True(t, strings.Contains(w.Body.String(), "access_denied"))
}
