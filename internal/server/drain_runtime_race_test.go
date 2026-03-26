package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminDrainsRuntimeMutationIsRaceFree(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	cfg.Logging.RequestLogEnabled = false
	srv := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil, nil)

	loginBody := map[string]string{"password": "testpass"}
	payload, err := json.Marshal(loginBody)
	testutil.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	var out map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	token := out["token"]
	testutil.True(t, token != "")

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				rw := httptest.NewRecorder()
				r := httptest.NewRequest(http.MethodGet, "/health", nil)
				srv.Router().ServeHTTP(rw, r)
			}
		}
	}()

	for i := range 25 {
		create := map[string]any{
			"id":   "racedrain-" + strconv.Itoa(i),
			"type": "http",
			"url":  "http://127.0.0.1:1/ingest",
		}
		body, err := json.Marshal(create)
		testutil.NoError(t, err)
		cw := httptest.NewRecorder()
		cr := httptest.NewRequest(http.MethodPost, "/api/admin/logging/drains", bytes.NewReader(body))
		cr.Header.Set("Content-Type", "application/json")
		cr.Header.Set("Authorization", "Bearer "+token)
		srv.Router().ServeHTTP(cw, cr)
		testutil.Equal(t, http.StatusCreated, cw.Code)
	}

	close(stop)
	wg.Wait()
}
