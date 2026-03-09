package mailer

import (
	"context"
	"fmt"

	mail "github.com/wneessen/go-mail"
)

// SMTPConfig holds SMTP connection parameters.
type SMTPConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	From       string
	FromName   string
	TLS        bool
	AuthMethod string // PLAIN, LOGIN, CRAM-MD5
}

// SMTPMailer sends emails via SMTP using go-mail.
type SMTPMailer struct {
	cfg SMTPConfig
}

// NewSMTPMailer creates an SMTPMailer with the given config.
func NewSMTPMailer(cfg SMTPConfig) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

func (m *SMTPMailer) Send(ctx context.Context, msg *Message) error {
	message := mail.NewMsg()
	if err := message.From(m.formatFrom()); err != nil {
		return fmt.Errorf("setting from address: %w", err)
	}
	if err := message.To(msg.To); err != nil {
		return fmt.Errorf("setting to address: %w", err)
	}
	message.Subject(msg.Subject)
	message.SetBodyString(mail.TypeTextHTML, msg.HTML)
	if msg.Text != "" {
		message.AddAlternativeString(mail.TypeTextPlain, msg.Text)
	}

	opts := []mail.Option{
		mail.WithPort(m.cfg.Port),
	}
	if m.cfg.TLS {
		opts = append(opts, mail.WithSSLPort(false))
	} else {
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSOpportunistic))
	}
	if m.cfg.Username != "" {
		opts = append(opts, mail.WithSMTPAuth(m.authType()))
		opts = append(opts, mail.WithUsername(m.cfg.Username))
		opts = append(opts, mail.WithPassword(m.cfg.Password))
	}

	client, err := mail.NewClient(m.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, message); err != nil {
		return fmt.Errorf("sending email via SMTP: %w", err)
	}
	return nil
}

func (m *SMTPMailer) formatFrom() string {
	if m.cfg.FromName != "" {
		return fmt.Sprintf("%s <%s>", m.cfg.FromName, m.cfg.From)
	}
	return m.cfg.From
}

func (m *SMTPMailer) authType() mail.SMTPAuthType {
	switch m.cfg.AuthMethod {
	case "LOGIN":
		return mail.SMTPAuthLogin
	case "CRAM-MD5":
		return mail.SMTPAuthCramMD5
	default:
		return mail.SMTPAuthPlain
	}
}
