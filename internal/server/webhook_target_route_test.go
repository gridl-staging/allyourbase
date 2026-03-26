package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestWebhookTargetRouteIsNotMounted(t *testing.T) {
	t.Parallel()

	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	req := httptest.NewRequest(http.MethodPost, "/__webhook-target__/deterministic-success", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}
