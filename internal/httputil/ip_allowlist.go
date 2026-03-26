// Package httputil IPAllowlist provides efficient IP/CIDR allowlist filtering for HTTP middleware, with support for client IP resolution and proxy header handling.
package httputil

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// IPAllowlist is an efficient IP/CIDR allowlist.
// An empty allowlist allows all IPs.
type IPAllowlist struct {
	allowAll bool
	nets     []*net.IPNet
}

// NewIPAllowlist parses a list of IP addresses and CIDR ranges.
// Entries may be plain IPv4/IPv6 literals or CIDR ranges (e.g. "203.0.113.0/24").
func NewIPAllowlist(entries []string) (*IPAllowlist, error) {
	if len(entries) == 0 {
		return &IPAllowlist{allowAll: true}, nil
	}

	nets := make([]*net.IPNet, 0, len(entries))
	for _, raw := range entries {
		n, err := parseAllowlistEntry(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid allowlist entry %q: %w", raw, err)
		}
		if n != nil {
			nets = append(nets, n)
		}
	}

	if len(nets) == 0 {
		return &IPAllowlist{allowAll: true}, nil
	}

	return &IPAllowlist{nets: nets}, nil
}

// Allow returns whether the provided IP is included in the allowlist.
// It returns true for any parseable IP when the allowlist is empty.
func (a *IPAllowlist) Allow(rawIP string) bool {
	if a == nil || a.allowAll {
		return true
	}
	ip := net.ParseIP(rawIP)
	if ip == nil {
		return false
	}
	for _, network := range a.nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// Middleware denies requests from IPs not present in the allowlist.
// When denied, it returns JSON:
// {"error":"access_denied","message":"Your IP address is not in the allowlist"}
func (a *IPAllowlist) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r)
		if a.Allow(ip) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"access_denied","message":"Your IP address is not in the allowlist"}`))
	})
}

// ClientIP resolves a best-effort client IP.
// X-Forwarded-For and X-Real-IP are trusted only when the direct peer is private/loopback.
func ClientIP(r *http.Request) string {
	host := r.RemoteAddr
	if hostAndPort, _, err := net.SplitHostPort(host); err == nil {
		host = hostAndPort
	}

	if isPrivateAddress(host) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return firstForwardedIP(xff)
		}
		if xReal := strings.TrimSpace(r.Header.Get("X-Real-IP")); xReal != "" {
			return xReal
		}
	}

	return host
}

func firstForwardedIP(raw string) string {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 {
		return strings.TrimSpace(raw)
	}
	return strings.TrimSpace(parts[0])
}

// IsPrivateIP reports whether an address is private or loopback.
func IsPrivateIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback() || parsed.IsPrivate()
}

func isPrivateAddress(ip string) bool { return IsPrivateIP(ip) }

// ParseAuditIP extracts and normalizes a single IP address from a raw value
// that may contain a port suffix (host:port) or a comma-separated chain
// (X-Forwarded-For). Returns nil if the value is empty or unparseable.
func ParseAuditIP(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if comma := strings.Index(trimmed, ","); comma >= 0 {
		trimmed = strings.TrimSpace(trimmed[:comma])
	}
	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		trimmed = host
	}
	ip := net.ParseIP(trimmed)
	if ip == nil {
		return nil
	}
	normalized := ip.String()
	return &normalized
}

// AuditIPFromRequest extracts the client IP for audit logging, preferring
// X-Forwarded-For over RemoteAddr, and normalizing the result.
func AuditIPFromRequest(r *http.Request) *string {
	if ip := ParseAuditIP(r.Header.Get("X-Forwarded-For")); ip != nil {
		return ip
	}
	return ParseAuditIP(r.RemoteAddr)
}

// parses a single entry into a network. Empty entries return nil. Plain IPs are wrapped as /32 (IPv4) or /128 (IPv6) host networks; CIDR ranges are parsed as-is.
func parseAllowlistEntry(value string) (*net.IPNet, error) {
	entry := strings.TrimSpace(value)
	if entry == "" {
		return nil, nil
	}
	if strings.Contains(entry, "/") {
		_, network, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, err
		}
		return network, nil
	}
	ip := net.ParseIP(entry)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP")
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}, nil
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
}
