package storage

import (
	"net/url"
	"strings"
)

// RewritePublicURL swaps the scheme/host of originURL with cdnBaseURL while
// preserving path and query from originURL.
func RewritePublicURL(originURL, cdnBaseURL string) string {
	originURL = strings.TrimSpace(originURL)
	cdnBaseURL = strings.TrimSpace(cdnBaseURL)
	if originURL == "" || cdnBaseURL == "" {
		return originURL
	}

	origin, err := url.Parse(originURL)
	if err != nil || origin.Scheme == "" || origin.Host == "" {
		return originURL
	}

	cdn, err := url.Parse(cdnBaseURL)
	if err != nil || cdn.Scheme == "" || cdn.Host == "" {
		return originURL
	}

	origin.Scheme = cdn.Scheme
	origin.Host = cdn.Host
	origin.User = nil
	return origin.String()
}
