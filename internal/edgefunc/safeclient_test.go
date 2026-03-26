package edgefunc

import (
	"net"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestIsPrivateIP_IPv4(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ip      string
		private bool
	}{
		// Loopback
		{"loopback", "127.0.0.1", true},
		{"loopback-high", "127.255.255.255", true},
		// RFC 1918 private ranges
		{"10.0.0.0/8", "10.0.0.1", true},
		{"10.0.0.0/8-high", "10.255.255.255", true},
		{"172.16.0.0/12", "172.16.0.1", true},
		{"172.16.0.0/12-high", "172.31.255.255", true},
		{"172.15-not-private", "172.15.0.1", false},
		{"172.32-not-private", "172.32.0.1", false},
		{"192.168.0.0/16", "192.168.1.1", true},
		{"192.168.0.0/16-high", "192.168.255.255", true},
		// Link-local
		{"link-local", "169.254.1.1", true},
		{"link-local-high", "169.254.254.254", true},
		// Carrier-grade NAT (100.64.0.0/10)
		{"cgnat", "100.64.0.1", true},
		{"cgnat-high", "100.127.255.255", true},
		{"cgnat-below", "100.63.255.255", false},
		// Cloud metadata
		{"metadata-aws", "169.254.169.254", true},
		// This/source (0.0.0.0/8)
		{"this-network", "0.0.0.0", true},
		{"this-network-2", "0.255.255.255", true},
		// Broadcast
		{"broadcast", "255.255.255.255", true},
		// Multicast (224.0.0.0/4)
		{"multicast-low", "224.0.0.1", true},
		{"multicast-high", "239.255.255.255", true},
		// Reserved (240.0.0.0/4)
		{"reserved-low", "240.0.0.1", true},
		{"reserved-high", "254.255.255.255", true},
		// Public IPs — should NOT be blocked
		{"google-dns", "8.8.8.8", false},
		{"cloudflare", "1.1.1.1", false},
		{"public-random", "203.0.113.1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tc.ip)
			testutil.NotNil(t, ip)
			got := isPrivateOrReserved(ip)
			testutil.Equal(t, tc.private, got)
		})
	}
}

func TestIsPrivateIP_IPv6(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ip      string
		private bool
	}{
		// Loopback
		{"ipv6-loopback", "::1", true},
		// Unique local (fc00::/7)
		{"unique-local-fc", "fc00::1", true},
		{"unique-local-fd", "fd00::1", true},
		// Link-local (fe80::/10)
		{"link-local-v6", "fe80::1", true},
		// Unspecified
		{"unspecified-v6", "::", true},
		// IPv4-mapped IPv6 — private
		{"v4-mapped-loopback", "::ffff:127.0.0.1", true},
		{"v4-mapped-10net", "::ffff:10.0.0.1", true},
		{"v4-mapped-192", "::ffff:192.168.1.1", true},
		// IPv4-mapped IPv6 — public
		{"v4-mapped-public", "::ffff:8.8.8.8", false},
		// Public IPv6
		{"public-v6", "2001:4860:4860::8888", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tc.ip)
			testutil.NotNil(t, ip)
			got := isPrivateOrReserved(ip)
			testutil.Equal(t, tc.private, got)
		})
	}
}

func TestNewSSRFSafeClient(t *testing.T) {
	t.Parallel()

	client := NewSSRFSafeClient(nil)
	testutil.NotNil(t, client)
	testutil.NotNil(t, client.Transport)
}

func TestSSRFControlFunc_BlocksPrivateIP(t *testing.T) {
	t.Parallel()

	control := ssrfControlFunc(nil)

	// Simulate a connection to a private IP
	addr := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 80}
	err := control("tcp4", addr.String(), nil)
	testutil.True(t, err != nil, "expected SSRF block for private IP")
	testutil.True(t, strings.Contains(err.Error(), "ssrf"), "error should mention ssrf, got: %s", err.Error())
	testutil.True(t, strings.Contains(err.Error(), "blocked"), "error should mention blocked, got: %s", err.Error())
}

func TestSSRFControlFunc_AllowsPublicIP(t *testing.T) {
	t.Parallel()

	control := ssrfControlFunc(nil)

	addr := &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 443}
	err := control("tcp4", addr.String(), nil)
	testutil.Nil(t, err)
}

func TestSSRFControlFunc_BlocksIPv4MappedIPv6(t *testing.T) {
	t.Parallel()

	control := ssrfControlFunc(nil)

	// IPv4-mapped IPv6 private address
	addr := &net.TCPAddr{IP: net.ParseIP("::ffff:127.0.0.1"), Port: 80}
	err := control("tcp4", addr.String(), nil)
	testutil.True(t, err != nil, "expected SSRF block for IPv4-mapped loopback")
}
