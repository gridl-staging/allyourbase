package mailer

import "context"

// Message represents an email to be sent.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
	From    string // optional per-message override; empty = use mailer default
}

// Mailer sends email messages.
type Mailer interface {
	Send(ctx context.Context, msg *Message) error
}
