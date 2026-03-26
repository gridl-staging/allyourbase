package urlutil

import (
	"net/url"
	"strings"
)

// RedactURL removes all userinfo from a URL for safe logging.
// Invalid URLs are replaced with a fully redacted marker.
func RedactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "***"
	}

	if parsed.User != nil {
		parsed.User = nil
		redacted := parsed.String()
		return strings.Replace(redacted, "://", "://***@", 1)
	}

	return parsed.String()
}
