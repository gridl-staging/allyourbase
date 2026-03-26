package support

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestParseInboundEmail(t *testing.T) {
	t.Parallel()

	t.Run("valid multipart parsed", func(t *testing.T) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		testutil.NoError(t, mw.WriteField("from", "user@example.com"))
		testutil.NoError(t, mw.WriteField("to", "support@support.example.com"))
		testutil.NoError(t, mw.WriteField("subject", "Re: [Ticket #00000000-0000-0000-0000-000000000111]"))
		testutil.NoError(t, mw.WriteField("text", "hello"))
		testutil.NoError(t, mw.WriteField("envelope", `{"to":["support@support.example.com"]}`))
		testutil.NoError(t, mw.Close())

		req := httptest.NewRequest("POST", "/api/webhooks/support/email", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())

		email, err := ParseInboundEmail(req)
		testutil.NoError(t, err)
		testutil.Equal(t, "user@example.com", email.From)
		testutil.Equal(t, "support@support.example.com", email.To)
		testutil.Equal(t, "hello", email.Text)
	})

	t.Run("missing required fields rejected", func(t *testing.T) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		testutil.NoError(t, mw.WriteField("from", "user@example.com"))
		testutil.NoError(t, mw.Close())

		req := httptest.NewRequest("POST", "/api/webhooks/support/email", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())

		_, err := ParseInboundEmail(req)
		testutil.ErrorContains(t, err, "to")
	})
}

func TestExtractTicketIDFromSubject(t *testing.T) {
	t.Parallel()

	id, ok := ExtractTicketIDFromSubject("Re: [Ticket #00000000-0000-0000-0000-000000000111]")
	testutil.True(t, ok, "expected ticket match")
	testutil.Equal(t, "00000000-0000-0000-0000-000000000111", id)

	_, ok = ExtractTicketIDFromSubject("no ticket id")
	testutil.True(t, !ok, "expected no match")
}

func TestNormalizeEmailAddress(t *testing.T) {
	t.Parallel()

	t.Run("parses plain and display-name forms", func(t *testing.T) {
		email, err := NormalizeEmailAddress("Support User <User.Name@Example.com>")
		testutil.NoError(t, err)
		testutil.Equal(t, "user.name@example.com", email)

		email, err = NormalizeEmailAddress("simple@example.com")
		testutil.NoError(t, err)
		testutil.Equal(t, "simple@example.com", email)
	})

	t.Run("rejects invalid address", func(t *testing.T) {
		_, err := NormalizeEmailAddress("not an email")
		testutil.ErrorContains(t, err, "parse email address")
	})
}
