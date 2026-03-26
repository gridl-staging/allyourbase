package webhooks

import (
	"net/http"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

type dispatcherTransportRT struct{}

func (*dispatcherTransportRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }

func TestDispatcherSetHTTPTransport(t *testing.T) {
	t.Parallel()

	d := testDispatcher(&mockLister{})
	rt := &dispatcherTransportRT{}
	d.SetHTTPTransport(rt)

	testutil.True(t, d.client.Transport == rt)
}
