package server

import (
	"encoding/json"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestParsePublicEmailSendInputValid(t *testing.T) {
	t.Parallel()

	parsed, badRequestMessage := parsePublicEmailSendInput(emailSendRequest{
		To:      json.RawMessage(`[" first@example.com ","second@example.com"]`),
		Subject: "Hello",
		HTML:    "<p>Hello</p>",
	})

	testutil.Equal(t, "", badRequestMessage)
	testutil.SliceLen(t, parsed.recipients, 2)
	testutil.Equal(t, "first@example.com", parsed.recipients[0])
	testutil.Equal(t, "second@example.com", parsed.recipients[1])
	testutil.True(t, parsed.hasBody, "direct sends should be marked as body sends")
	testutil.True(t, !parsed.hasTemplate, "direct sends should not be marked as template sends")
}

func TestParsePublicEmailSendInputRejectsBodyTemplateConflict(t *testing.T) {
	t.Parallel()

	_, badRequestMessage := parsePublicEmailSendInput(emailSendRequest{
		To:          json.RawMessage(`"user@example.com"`),
		Subject:     "Hello",
		HTML:        "<p>Hello</p>",
		TemplateKey: "auth.verify",
	})

	testutil.Equal(t, "cannot specify both html/text and templateKey", badRequestMessage)
}
