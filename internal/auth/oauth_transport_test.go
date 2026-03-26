package auth

import (
	"net/http"
	"testing"
)

type oauthTransportRT struct{}

func (*oauthTransportRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }

func TestSetOAuthHTTPTransport(t *testing.T) {
	// NOT parallel: mutates the package-level oauthHTTPClient.Transport,
	// which would race with parallel tests that read the shared client
	// (e.g. TestOAuthCallbackWithCodeExchangeFailure via http.Client.Do).
	original := oauthHTTPClient.Transport
	t.Cleanup(func() {
		oauthHTTPClient.Transport = original
	})

	rt := &oauthTransportRT{}
	SetOAuthHTTPTransport(rt)

	if oauthHTTPClient.Transport != rt {
		t.Fatal("expected oauthHTTPClient transport to be set")
	}
}
