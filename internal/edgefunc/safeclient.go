package edgefunc

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// Private/reserved CIDR blocks that SSRF-safe clients must block.
// These cover: loopback, RFC 1918, link-local, carrier-grade NAT,
// cloud metadata, this-network, and IPv6 equivalents.
var privateCIDRs []*net.IPNet

func init() {
	cidrs := []string{
		"0.0.0.0/8",          // This network (RFC 1122)
		"10.0.0.0/8",         // Private (RFC 1918)
		"100.64.0.0/10",      // Carrier-grade NAT (RFC 6598)
		"127.0.0.0/8",        // Loopback (RFC 1122)
		"169.254.0.0/16",     // Link-local (RFC 3927)
		"172.16.0.0/12",      // Private (RFC 1918)
		"192.168.0.0/16",     // Private (RFC 1918)
		"224.0.0.0/4",        // Multicast (RFC 5771)
		"240.0.0.0/4",        // Reserved (RFC 1112)
		"255.255.255.255/32", // Broadcast
		"::1/128",            // IPv6 loopback
		"::/128",             // IPv6 unspecified
		"fc00::/7",           // IPv6 unique local (RFC 4193)
		"fe80::/10",          // IPv6 link-local (RFC 4291)
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid CIDR %q: %v", cidr, err))
		}
		privateCIDRs = append(privateCIDRs, network)
	}
}

// isPrivateOrReserved returns true if the IP falls within any private/reserved range.
// Handles IPv4-mapped IPv6 addresses by extracting the IPv4 portion.
func isPrivateOrReserved(ip net.IP) bool {
	// Handle IPv4-mapped IPv6 (e.g., ::ffff:10.0.0.1)
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, cidr := range privateCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// ssrfControlFunc returns a net.Dialer Control function that blocks connections
// to private/reserved IPs. The control function receives the resolved IP
// (post-DNS), so DNS rebinding is inherently mitigated.
func ssrfControlFunc(allowedDomains []string) func(network, address string, c syscall.RawConn) error {
	// NOTE: allowedDomains is a placeholder for future domain allowlist support.
	// The control function receives resolved IPs, not hostnames, so domain-level
	// allowlisting requires a separate check layer before dialing.
	_ = allowedDomains

	return func(network, address string, c syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("ssrf: invalid address %q: %w", address, err)
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("ssrf: could not parse IP from %q", host)
		}
		if isPrivateOrReserved(ip) {
			return fmt.Errorf("ssrf: connection to private/reserved IP %s blocked", ip)
		}
		return nil
	}
}

// NewSSRFSafeClient returns an *http.Client that blocks outbound connections
// to private/reserved IP ranges. Use this for edge function fetch() calls.
func NewSSRFSafeClient(allowedDomains []string) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: ssrfControlFunc(allowedDomains),
	}
	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}
