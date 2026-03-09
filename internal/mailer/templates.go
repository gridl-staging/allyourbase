package mailer

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

var templates *template.Template

func init() {
	templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))
}

// TemplateData holds values passed to email templates.
type TemplateData struct {
	AppName   string
	ActionURL string
}

// RenderPasswordReset renders the password reset email and returns HTML and plain text.
func RenderPasswordReset(data TemplateData) (html string, text string, err error) {
	return render("password_reset.html", data)
}

// RenderMagicLink renders the magic link email and returns HTML and plain text.
func RenderMagicLink(data TemplateData) (html string, text string, err error) {
	return render("magic_link.html", data)
}

// RenderVerification renders the email verification email and returns HTML and plain text.
func RenderVerification(data TemplateData) (html string, text string, err error) {
	return render("verification.html", data)
}

func render(name string, data TemplateData) (string, string, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", "", fmt.Errorf("rendering template %s: %w", name, err)
	}
	html := buf.String()
	// Simple plain-text fallback: strip tags.
	text := stripHTML(html)
	return html, text, nil
}

// Default subjects for system email templates.
const (
	DefaultPasswordResetSubject  = "Reset your password"
	DefaultVerificationSubject   = "Verify your email"
	DefaultMagicLinkSubject      = "Your login link"
)

// BuiltinHTMLTemplate returns the raw HTML source for a built-in template.
// Valid names: "password_reset.html", "verification.html", "magic_link.html".
func BuiltinHTMLTemplate(name string) (string, error) {
	b, err := templateFS.ReadFile("templates/" + name)
	if err != nil {
		return "", fmt.Errorf("reading built-in template %s: %w", name, err)
	}
	return string(b), nil
}

// stripHTML is a minimal tag stripper for plain-text email fallback.
func stripHTML(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out.WriteRune(r)
		}
	}
	// Collapse whitespace runs.
	lines := strings.Split(out.String(), "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return strings.Join(result, "\n")
}
