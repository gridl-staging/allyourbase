package server

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/support"
	"github.com/allyourbase/ayb/internal/testutil"
)

const (
	testSupportWebhookTicketID = "00000000-0000-0000-0000-000000000111"
	testSupportWebhookAddress  = "support@support.example.com"
	testSupportWebhookReply    = "reply text"
)

func newInboundEmailRequest(t *testing.T, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		testutil.NoError(t, mw.WriteField(k, v))
	}
	testutil.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/support/email", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func authorizeSupportWebhookRequest(req *http.Request) *http.Request {
	req.Header.Set("X-Webhook-Secret", "secret-1")
	return req
}

func inboundEmailFields(overrides map[string]string) map[string]string {
	fields := map[string]string{
		"from":     "user-1@example.com",
		"to":       testSupportWebhookAddress,
		"subject":  "Re: [Ticket #" + testSupportWebhookTicketID + "]",
		"text":     testSupportWebhookReply,
		"envelope": `{"to":["` + testSupportWebhookAddress + `"]}`,
	}
	for key, value := range overrides {
		fields[key] = value
	}
	return fields
}

func TestHandleSupportEmailWebhookMatchedReplyAddsCustomerMessage(t *testing.T) {
	t.Parallel()
	svc := &fakeSupportService{}
	srv := supportTestServer(svc)
	req := authorizeSupportWebhookRequest(newInboundEmailRequest(t, inboundEmailFields(map[string]string{
		"from": "Support User <user-1@example.com>",
	})))
	w := httptest.NewRecorder()

	srv.handleSupportEmailWebhook(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, testSupportWebhookTicketID, svc.lastAddTicketID)
	testutil.Equal(t, support.SenderCustomer, svc.lastAddSender)
}

func TestHandleSupportEmailWebhookSenderMismatchRejected(t *testing.T) {
	t.Parallel()
	svc := &fakeSupportService{}
	srv := supportTestServer(svc)
	req := authorizeSupportWebhookRequest(newInboundEmailRequest(t, inboundEmailFields(map[string]string{
		"from": "attacker@example.com",
	})))
	w := httptest.NewRecorder()

	srv.handleSupportEmailWebhook(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Equal(t, "", svc.lastAddTicketID)
}

func TestHandleSupportEmailWebhookUnmatchedSubjectIsNoOp(t *testing.T) {
	t.Parallel()
	svc := &fakeSupportService{}
	srv := supportTestServer(svc)
	req := authorizeSupportWebhookRequest(newInboundEmailRequest(t, inboundEmailFields(map[string]string{
		"subject": "new request no ticket id",
	})))
	w := httptest.NewRecorder()

	srv.handleSupportEmailWebhook(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "", svc.lastAddTicketID)
}

func TestHandleSupportEmailWebhookInvalidAuthRejected(t *testing.T) {
	t.Parallel()
	srv := supportTestServer(&fakeSupportService{})
	req := newInboundEmailRequest(t, inboundEmailFields(nil))
	req.Header.Set("X-Webhook-Secret", "wrong")
	w := httptest.NewRecorder()

	srv.handleSupportEmailWebhook(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleSupportEmailWebhookInvalidPayloadRejected(t *testing.T) {
	t.Parallel()
	svc := &fakeSupportService{}
	srv := supportTestServer(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/support/email", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "text/plain")
	req = authorizeSupportWebhookRequest(req)
	w := httptest.NewRecorder()

	srv.handleSupportEmailWebhook(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Equal(t, "", svc.lastAddTicketID)
}

func TestHandleSupportEmailWebhookDomainMismatchRejected(t *testing.T) {
	t.Parallel()
	srv := supportTestServer(&fakeSupportService{})
	req := authorizeSupportWebhookRequest(newInboundEmailRequest(t, inboundEmailFields(map[string]string{
		"to":       "support@other.example.com",
		"envelope": `{"to":["support@other.example.com"]}`,
	})))
	w := httptest.NewRecorder()

	srv.handleSupportEmailWebhook(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSupportEmailWebhookNilTicketRejected(t *testing.T) {
	t.Parallel()
	svc := &fakeSupportService{
		getTicketFn: func(ctx context.Context, ticketID string) (*support.Ticket, error) {
			return nil, nil
		},
	}
	srv := supportTestServer(svc)
	req := authorizeSupportWebhookRequest(newInboundEmailRequest(t, inboundEmailFields(nil)))
	w := httptest.NewRecorder()

	srv.handleSupportEmailWebhook(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, "", svc.lastAddTicketID)
}

func TestHandleSupportEmailWebhookOversizedPayloadRejected(t *testing.T) {
	t.Parallel()
	svc := &fakeSupportService{}
	srv := supportTestServer(svc)
	req := authorizeSupportWebhookRequest(newInboundEmailRequest(t, inboundEmailFields(map[string]string{
		"text": strings.Repeat("a", supportWebhookMaxBodySize),
	})))
	w := httptest.NewRecorder()

	srv.handleSupportEmailWebhook(w, req)
	testutil.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	testutil.Equal(t, "", svc.lastAddTicketID)
}

func TestWebhookSecretQueryParamRejected(t *testing.T) {
	t.Parallel()
	srv := supportTestServer(&fakeSupportService{})
	q := url.Values{}
	q.Set("secret", "secret-1")
	req := newInboundEmailRequest(t, inboundEmailFields(map[string]string{
		"subject": "no ticket",
	}))
	req.URL.RawQuery = q.Encode()
	w := httptest.NewRecorder()

	srv.handleSupportEmailWebhook(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSupportWebhookSecretRequired(t *testing.T) {
	t.Parallel()
	srv := supportTestServer(&fakeSupportService{})
	srv.cfg.Support.WebhookSecret = ""
	req := newInboundEmailRequest(t, inboundEmailFields(nil))
	w := httptest.NewRecorder()

	srv.handleSupportEmailWebhook(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}
