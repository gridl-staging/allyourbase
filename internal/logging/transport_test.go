package logging

import (
	"net/http"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

type testRoundTripper struct{}

func (*testRoundTripper) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }

func TestHTTPDrainSetHTTPTransport(t *testing.T) {
	t.Parallel()

	drain := NewHTTPDrain(DrainConfig{ID: "d1", URL: "http://example.com"})
	rt := &testRoundTripper{}
	drain.SetHTTPTransport(rt)

	testutil.True(t, drain.client.Transport == rt)
}

func TestDatadogDrainSetHTTPTransport(t *testing.T) {
	t.Parallel()

	drain := NewDatadogDrain(DrainConfig{ID: "d1", URL: "http://example.com"})
	rt := &testRoundTripper{}
	drain.SetHTTPTransport(rt)

	testutil.True(t, drain.client.Transport == rt)
}

func TestLokiDrainSetHTTPTransport(t *testing.T) {
	t.Parallel()

	drain := NewLokiDrain(DrainConfig{ID: "d1", URL: "http://example.com"})
	rt := &testRoundTripper{}
	drain.SetHTTPTransport(rt)

	testutil.True(t, drain.client.Transport == rt)
}
