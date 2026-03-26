package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestCORSPassesThroughTusOptions(t *testing.T) {
	t.Parallel()

	nextCalled := false
	h := corsMiddleware([]string{"*"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.Header().Set("Tus-Resumable", "1.0.0")
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/storage/upload/resumable", nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.True(t, nextCalled, "expected TUS OPTIONS request to reach next handler")
	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "1.0.0", w.Header().Get("Tus-Resumable"))
}
