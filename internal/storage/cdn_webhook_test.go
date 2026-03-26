package storage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestWebhook_PurgeURLs_SendsCanonicalPayloadAndSignature(t *testing.T) {
	t.Parallel()

	var payload webhookPayload
	var signature string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		signature = req.Header.Get(webhookSignatureHeader)
		err := json.NewDecoder(req.Body).Decode(&payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	provider := NewWebhookCDNProvider(WebhookCDNOptions{
		Endpoint:      server.URL,
		SigningSecret: "secret",
		HTTPClient:    server.Client(),
	})

	err := provider.PurgeURLs(context.Background(), []string{"  https://cdn.example.com/a", "", "https://cdn.example.com/b  "})
	testutil.NoError(t, err)

	bodyBytes, err := stableWebhookPayloadBytes(payload)
	testutil.NoError(t, err)
	testutil.Equal(t, webhookPurgeURLsOp, payload.Operation)
	if !reflect.DeepEqual([]string{"https://cdn.example.com/a", "https://cdn.example.com/b"}, payload.URLs) {
		t.Fatalf("expected payload urls %v, got %v", []string{"https://cdn.example.com/a", "https://cdn.example.com/b"}, payload.URLs)
	}
	testutil.Equal(t, webhookPayloadSignature("secret", bodyBytes), signature)
}

func TestWebhook_PurgeURLs_RetriesRetryableErrors(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, "retry")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	provider := NewWebhookCDNProvider(WebhookCDNOptions{
		Endpoint:      server.URL,
		SigningSecret: "secret",
		HTTPClient:    server.Client(),
		MaxRetries:    3,
	})

	err := provider.PurgeAll(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 2, attempts)
}

func TestWebhook_PurgeURLs_NonRetryable4xxFailsFast(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		attempts++
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	provider := NewWebhookCDNProvider(WebhookCDNOptions{
		Endpoint:      server.URL,
		SigningSecret: "secret",
		HTTPClient:    server.Client(),
		MaxRetries:    3,
	})

	err := provider.PurgeAll(context.Background())
	testutil.ErrorContains(t, err, "status=400")
	testutil.Equal(t, 1, attempts)
}

func TestWebhook_PurgeURLs_PassesEmptyInputWithoutRequest(t *testing.T) {
	t.Parallel()

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	provider := NewWebhookCDNProvider(WebhookCDNOptions{
		Endpoint:      server.URL,
		SigningSecret: "secret",
		HTTPClient:    server.Client(),
	})

	err := provider.PurgeURLs(context.Background(), []string{"  ", "\t", ""})
	testutil.NoError(t, err)
	testutil.Equal(t, false, called)
}
