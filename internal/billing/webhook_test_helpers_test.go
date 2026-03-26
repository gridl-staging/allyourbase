package billing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

const testStripeWebhookSecret = "whsec_test_123"

func newWebhookRequestWithBody(t *testing.T, body []byte, secret string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", webhookSignatureHeader(t, body, secret))
	return req
}

func webhookEventBody(t *testing.T, eventID, eventType string, object map[string]any) []byte {
	t.Helper()
	payload := map[string]any{
		"id":   eventID,
		"type": eventType,
		"data": map[string]any{
			"object": object,
		},
	}
	b, err := json.Marshal(payload)
	testutil.NoError(t, err)
	return b
}

func webhookSignatureHeader(t *testing.T, body []byte, secret string) string {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := computeStripeSignature(timestamp, string(body), secret)
	return fmt.Sprintf("t=%s,v1=%s", timestamp, signature)
}

func webhookSignatureHeaderWithTimestamp(t *testing.T, body []byte, secret string, ts int64) string {
	t.Helper()
	timestamp := strconv.FormatInt(ts, 10)
	signature := computeStripeSignature(timestamp, string(body), secret)
	return fmt.Sprintf("t=%s,v1=%s", timestamp, signature)
}
