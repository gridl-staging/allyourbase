package realtime_test

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSSEAllowsInternalNotificationsTableSubscription(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, nil, nil, testSchemaCache("posts"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tables=_ayb_notifications")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	testutil.Equal(t, http.StatusOK, resp.StatusCode)
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if scanner.Text() == "" {
			break
		}
	}
	testutil.True(t, len(lines) >= 2, "expected SSE connected event")
	testutil.Equal(t, "event: connected", lines[0])
}
