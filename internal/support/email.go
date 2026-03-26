package support

import (
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
)

// InboundEmail is a normalized SendGrid inbound parse payload.
type InboundEmail struct {
	From     string
	To       string
	Subject  string
	Text     string
	Envelope string
}

var ticketSubjectRe = regexp.MustCompile(`\[Ticket #([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})\]`)

// ParseInboundEmail parses SendGrid inbound parse multipart form payload.
func ParseInboundEmail(r *http.Request) (*InboundEmail, error) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		return nil, fmt.Errorf("parse inbound email form: %w", err)
	}

	email := &InboundEmail{
		From:     strings.TrimSpace(r.FormValue("from")),
		To:       strings.TrimSpace(r.FormValue("to")),
		Subject:  strings.TrimSpace(r.FormValue("subject")),
		Text:     strings.TrimSpace(r.FormValue("text")),
		Envelope: strings.TrimSpace(r.FormValue("envelope")),
	}

	if email.From == "" {
		return nil, fmt.Errorf("inbound email missing from")
	}
	if email.To == "" {
		return nil, fmt.Errorf("inbound email missing to")
	}
	if email.Subject == "" {
		return nil, fmt.Errorf("inbound email missing subject")
	}
	if email.Text == "" {
		return nil, fmt.Errorf("inbound email missing text")
	}
	if email.Envelope == "" {
		return nil, fmt.Errorf("inbound email missing envelope")
	}

	return email, nil
}

// ExtractTicketIDFromSubject extracts a support ticket UUID from [Ticket #UUID].
func ExtractTicketIDFromSubject(subject string) (string, bool) {
	match := ticketSubjectRe.FindStringSubmatch(subject)
	if len(match) != 2 {
		return "", false
	}
	return strings.ToLower(match[1]), true
}

// NormalizeEmailAddress extracts a single email address and lowercases it.
func NormalizeEmailAddress(raw string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse email address: %w", err)
	}
	return strings.ToLower(strings.TrimSpace(addr.Address)), nil
}
