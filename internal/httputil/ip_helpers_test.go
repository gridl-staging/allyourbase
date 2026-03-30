package httputil

import (
	"testing"
)

// ---------------------------------------------------------------------------
// firstForwardedIP
// ---------------------------------------------------------------------------

func TestFirstForwardedIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		// Standard X-Forwarded-For with multiple IPs — first one is the client.
		{"single IP", "203.0.113.50", "203.0.113.50"},
		{"multiple IPs", "203.0.113.50, 70.41.3.18, 150.172.238.178", "203.0.113.50"},
		{"whitespace around first", "  10.0.0.1 , 10.0.0.2", "10.0.0.1"},
		{"empty string", "", ""},
		{"single with trailing comma", "1.2.3.4,", "1.2.3.4"},
		{"IPv6 first", "2001:db8::1, 10.0.0.1", "2001:db8::1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := firstForwardedIP(tc.raw)
			if got != tc.want {
				t.Errorf("firstForwardedIP(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsPrivateIP
// ---------------------------------------------------------------------------

func TestIsPrivateIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ip   string
		want bool
	}{
		// RFC 1918 private ranges.
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},

		// Loopback addresses.
		{"127.0.0.1", true},
		{"::1", true}, // IPv6 loopback

		// Public addresses.
		{"8.8.8.8", false},
		{"203.0.113.50", false},
		{"2001:db8::1", false},

		// Invalid/unparseable.
		{"not-an-ip", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.ip, func(t *testing.T) {
			t.Parallel()
			got := IsPrivateIP(tc.ip)
			if got != tc.want {
				t.Errorf("IsPrivateIP(%q) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseAllowlistEntry
// ---------------------------------------------------------------------------

func TestParseAllowlistEntry(t *testing.T) {
	t.Parallel()

	t.Run("empty string returns nil", func(t *testing.T) {
		t.Parallel()
		net, err := parseAllowlistEntry("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if net != nil {
			t.Fatalf("expected nil for empty input, got %v", net)
		}
	})

	t.Run("whitespace-only returns nil", func(t *testing.T) {
		t.Parallel()
		net, err := parseAllowlistEntry("   ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if net != nil {
			t.Fatalf("expected nil for whitespace, got %v", net)
		}
	})

	t.Run("plain IPv4 becomes /32 host network", func(t *testing.T) {
		t.Parallel()
		net, err := parseAllowlistEntry("203.0.113.50")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if net == nil {
			t.Fatal("expected non-nil network")
		}
		// /32 mask means only this exact IP matches.
		ones, bits := net.Mask.Size()
		if ones != 32 || bits != 32 {
			t.Errorf("mask = /%d (of %d), want /32", ones, bits)
		}
	})

	t.Run("plain IPv6 becomes /128 host network", func(t *testing.T) {
		t.Parallel()
		net, err := parseAllowlistEntry("2001:db8::1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if net == nil {
			t.Fatal("expected non-nil network")
		}
		ones, bits := net.Mask.Size()
		if ones != 128 || bits != 128 {
			t.Errorf("mask = /%d (of %d), want /128", ones, bits)
		}
	})

	t.Run("CIDR notation parsed as-is", func(t *testing.T) {
		t.Parallel()
		net, err := parseAllowlistEntry("10.0.0.0/8")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if net == nil {
			t.Fatal("expected non-nil network")
		}
		ones, _ := net.Mask.Size()
		if ones != 8 {
			t.Errorf("mask = /%d, want /8", ones)
		}
	})

	t.Run("invalid IP returns error", func(t *testing.T) {
		t.Parallel()
		_, err := parseAllowlistEntry("not-a-valid-ip")
		if err == nil {
			t.Fatal("expected error for invalid IP")
		}
	})

	t.Run("invalid CIDR returns error", func(t *testing.T) {
		t.Parallel()
		_, err := parseAllowlistEntry("10.0.0.0/99")
		if err == nil {
			t.Fatal("expected error for invalid CIDR")
		}
	})

	t.Run("leading/trailing whitespace trimmed", func(t *testing.T) {
		t.Parallel()
		net, err := parseAllowlistEntry("  192.168.1.0/24  ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if net == nil {
			t.Fatal("expected non-nil network")
		}
	})
}
