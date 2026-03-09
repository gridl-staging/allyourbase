// Package emailtemplates provides custom email template storage, rendering,
// and fallback to built-in defaults embedded in the binary.
package emailtemplates

import (
	"context"
	"errors"
	"fmt"
	"html"
	htmltemplate "html/template"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	texttemplate "text/template"
	"time"

	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors.
var (
	ErrNotFound     = errors.New("template not found")
	ErrNoTemplate   = errors.New("no template exists for key")
	ErrInvalidKey   = errors.New("invalid template key format")
	ErrParseFailed  = errors.New("template parse error")
	ErrRenderFailed = errors.New("template render error")
	ErrTooLarge     = errors.New("template exceeds size limit")
)

// Size limits matching database CHECK constraints.
const (
	MaxSubjectLen = 1000
	MaxHTMLLen    = 256000
)

// keyPattern validates template key format: dot-separated lowercase segments.
var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9_]*)+$`)

// ValidateKey checks if a template key matches the required format.
func ValidateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return nil
}

// Template represents a custom email template stored in the database.
type Template struct {
	ID              string    `json:"id"`
	TemplateKey     string    `json:"templateKey"`
	SubjectTemplate string    `json:"subjectTemplate"`
	HTMLTemplate    string    `json:"htmlTemplate"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// RenderedEmail holds the result of rendering a template.
type RenderedEmail struct {
	Subject string
	HTML    string
	Text    string
}

// EffectiveTemplate holds template source info for admin API responses.
type EffectiveTemplate struct {
	Source          string // "custom" or "builtin"
	TemplateKey     string
	SubjectTemplate string
	HTMLTemplate    string
	Enabled         bool
	Variables       []string // available variable names (for system keys)
}

// BuiltinTemplate holds a compiled built-in template and its default subject.
type BuiltinTemplate struct {
	SubjectTemplate string // raw subject template string
	HTMLTemplate    string // raw HTML template string
	Variables       []string
}

// DefaultBuiltins constructs the built-in template map from the embedded
// mailer templates and their default subjects. Used by start.go to wire
// the email template service.
func DefaultBuiltins() map[string]BuiltinTemplate {
	systemVars := []string{"AppName", "ActionURL"}
	builtins := make(map[string]BuiltinTemplate, 3)

	keys := []struct {
		key     string
		subject string
		file    string
	}{
		{"auth.password_reset", mailer.DefaultPasswordResetSubject, "password_reset.html"},
		{"auth.email_verification", mailer.DefaultVerificationSubject, "verification.html"},
		{"auth.magic_link", mailer.DefaultMagicLinkSubject, "magic_link.html"},
	}
	for _, k := range keys {
		html, err := mailer.BuiltinHTMLTemplate(k.file)
		if err != nil {
			// Embedded templates must always be available; panic on missing.
			panic(fmt.Sprintf("missing built-in email template %q: %v", k.file, err))
		}
		builtins[k.key] = BuiltinTemplate{
			SubjectTemplate: k.subject,
			HTMLTemplate:    html,
			Variables:       systemVars,
		}
	}
	return builtins
}

// Store handles database CRUD for custom email templates.
type Store struct {
	pool *pgxpool.Pool
}

// TemplateStore defines storage operations needed by Service.
type TemplateStore interface {
	Upsert(ctx context.Context, key, subjectTpl, htmlTpl string) (*Template, error)
	Get(ctx context.Context, key string) (*Template, error)
	List(ctx context.Context) ([]*Template, error)
	Delete(ctx context.Context, key string) error
	SetEnabled(ctx context.Context, key string, enabled bool) error
}

// NewStore creates a new template store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Upsert creates or updates a custom template override. It validates the key
// format and parses both templates to catch syntax errors before saving.
func (s *Store) Upsert(ctx context.Context, key, subjectTpl, htmlTpl string) (*Template, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}

	// Enforce size limits before parsing (matches DB CHECK constraints).
	if len(subjectTpl) > MaxSubjectLen {
		return nil, fmt.Errorf("%w: subject exceeds %d characters", ErrTooLarge, MaxSubjectLen)
	}
	if len(htmlTpl) > MaxHTMLLen {
		return nil, fmt.Errorf("%w: html exceeds %d characters", ErrTooLarge, MaxHTMLLen)
	}

	// Parse templates to catch syntax errors before saving.
	if _, err := parseSubject(key, subjectTpl); err != nil {
		return nil, fmt.Errorf("%w: subject: %v", ErrParseFailed, err)
	}
	if _, err := parseHTML(key, htmlTpl); err != nil {
		return nil, fmt.Errorf("%w: html: %v", ErrParseFailed, err)
	}

	var t Template
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_email_templates (template_key, subject_template, html_template)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (template_key) DO UPDATE
		   SET subject_template = EXCLUDED.subject_template,
		       html_template = EXCLUDED.html_template,
		       updated_at = now()
		 RETURNING id, template_key, subject_template, html_template, enabled, created_at, updated_at`,
		key, subjectTpl, htmlTpl,
	).Scan(&t.ID, &t.TemplateKey, &t.SubjectTemplate, &t.HTMLTemplate,
		&t.Enabled, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting template %q: %w", key, err)
	}
	return &t, nil
}

// Get returns a custom template by key, or ErrNotFound.
func (s *Store) Get(ctx context.Context, key string) (*Template, error) {
	var t Template
	err := s.pool.QueryRow(ctx,
		`SELECT id, template_key, subject_template, html_template, enabled, created_at, updated_at
		 FROM _ayb_email_templates WHERE template_key = $1`, key,
	).Scan(&t.ID, &t.TemplateKey, &t.SubjectTemplate, &t.HTMLTemplate,
		&t.Enabled, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting template %q: %w", key, err)
	}
	return &t, nil
}

// List returns all custom template overrides.
func (s *Store) List(ctx context.Context) ([]*Template, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, template_key, subject_template, html_template, enabled, created_at, updated_at
		 FROM _ayb_email_templates ORDER BY template_key`)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}
	defer rows.Close()

	var templates []*Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.TemplateKey, &t.SubjectTemplate, &t.HTMLTemplate,
			&t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning template row: %w", err)
		}
		templates = append(templates, &t)
	}
	return templates, rows.Err()
}

// Delete removes a custom template override by key.
func (s *Store) Delete(ctx context.Context, key string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_email_templates WHERE template_key = $1`, key)
	if err != nil {
		return fmt.Errorf("deleting template %q: %w", key, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetEnabled toggles the enabled flag on a custom template.
func (s *Store) SetEnabled(ctx context.Context, key string, enabled bool) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_email_templates SET enabled = $2, updated_at = now()
		 WHERE template_key = $1`, key, enabled)
	if err != nil {
		return fmt.Errorf("toggling template %q enabled: %w", key, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Service provides template rendering with fallback to built-in defaults.
type Service struct {
	store    TemplateStore
	builtins map[string]BuiltinTemplate
	mailer   mailer.Mailer
	logger   *slog.Logger
	mu       sync.RWMutex
}

// NewService creates a new template service.
func NewService(store TemplateStore, builtins map[string]BuiltinTemplate) *Service {
	return &Service{
		store:    store,
		builtins: builtins,
		logger:   slog.Default(),
	}
}

// SetLogger sets the logger for the service.
func (s *Service) SetLogger(l *slog.Logger) {
	s.logger = l
}

// List delegates to the store to list all custom overrides.
func (s *Service) List(ctx context.Context) ([]*Template, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.List(ctx)
}

// Upsert delegates to the store to create or update a custom template.
func (s *Service) Upsert(ctx context.Context, key, subjectTpl, htmlTpl string) (*Template, error) {
	if s.store == nil {
		return nil, errors.New("email template store not configured")
	}
	return s.store.Upsert(ctx, key, subjectTpl, htmlTpl)
}

// Delete delegates to the store to remove a custom template.
func (s *Service) Delete(ctx context.Context, key string) error {
	if s.store == nil {
		return errors.New("email template store not configured")
	}
	return s.store.Delete(ctx, key)
}

// SetEnabled delegates to the store to toggle the enabled flag.
func (s *Service) SetEnabled(ctx context.Context, key string, enabled bool) error {
	if s.store == nil {
		return errors.New("email template store not configured")
	}
	return s.store.SetEnabled(ctx, key, enabled)
}

// SystemKeys returns the list of built-in system template keys with their metadata.
// Keys are returned in sorted order for deterministic API responses.
func (s *Service) SystemKeys() []EffectiveTemplate {
	sortedKeys := make([]string, 0, len(s.builtins))
	for key := range s.builtins {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	keys := make([]EffectiveTemplate, 0, len(s.builtins))
	for _, key := range sortedKeys {
		b := s.builtins[key]
		keys = append(keys, EffectiveTemplate{
			Source:          "builtin",
			TemplateKey:     key,
			SubjectTemplate: b.SubjectTemplate,
			HTMLTemplate:    b.HTMLTemplate,
			Enabled:         true,
			Variables:       b.Variables,
		})
	}
	return keys
}

// SetMailer sets the mailer for sending emails.
func (s *Service) SetMailer(m mailer.Mailer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mailer = m
}

// renderTimeout is the maximum time allowed for template execution.
const renderTimeout = 5 * time.Second

// Render renders an email template by key, with fallback to built-in defaults.
func (s *Service) Render(ctx context.Context, key string, vars map[string]string) (*RenderedEmail, error) {
	// Set render timeout.
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()

	var customRenderErr error

	// Try custom override first (only if store is available).
	if s.store != nil {
		custom, err := s.store.Get(ctx, key)
		switch {
		case err == nil && custom.Enabled:
			rendered, renderErr := renderTemplates(ctx, key, custom.SubjectTemplate, custom.HTMLTemplate, vars)
			if renderErr == nil {
				return rendered, nil
			}
			customRenderErr = renderErr
			// Custom template failed — log and fall through to built-in.
			s.logger.Error("custom email template render failed, falling back to builtin",
				"key", key, "error", renderErr)
		case err == nil:
			// Disabled custom override; fall through to built-in.
		case errors.Is(err, ErrNotFound):
			// No custom override; fall through to built-in.
		default:
			return nil, fmt.Errorf("loading custom template %q: %w", key, err)
		}
	}

	// Try built-in template.
	builtin, ok := s.builtins[key]
	if !ok {
		if customRenderErr != nil {
			return nil, customRenderErr
		}
		return nil, fmt.Errorf("%w: %q", ErrNoTemplate, key)
	}

	return renderTemplates(ctx, key, builtin.SubjectTemplate, builtin.HTMLTemplate, vars)
}

// RenderWithFallback renders a template with graceful degradation: if custom
// template rendering fails, falls back to built-in. Only returns error if
// built-in also fails. Returns (subject, html, text, err) to satisfy
// auth.EmailTemplateRenderer without import coupling.
func (s *Service) RenderWithFallback(ctx context.Context, key string, vars map[string]string) (string, string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()

	// Try custom override first (only if store is available).
	if s.store != nil {
		custom, err := s.store.Get(ctx, key)
		switch {
		case err == nil && custom.Enabled:
			rendered, renderErr := renderTemplates(ctx, key, custom.SubjectTemplate, custom.HTMLTemplate, vars)
			if renderErr == nil {
				return rendered.Subject, rendered.HTML, rendered.Text, nil
			}
			// Custom template failed — log and fall through to built-in.
			s.logger.Error("custom email template render failed, falling back to builtin",
				"key", key, "error", renderErr)
		case err == nil:
			// Disabled custom override; fall through to built-in.
		case errors.Is(err, ErrNotFound):
			// No custom override; fall through to built-in.
		default:
			// Store error (DB down, etc.) — log and fall through to built-in.
			s.logger.Error("failed to load custom email template, falling back to builtin",
				"key", key, "error", err)
		}
	}

	// Built-in fallback (should always succeed for system keys).
	builtin, ok := s.builtins[key]
	if !ok {
		return "", "", "", fmt.Errorf("%w: %q", ErrNoTemplate, key)
	}

	rendered, err := renderTemplates(ctx, key, builtin.SubjectTemplate, builtin.HTMLTemplate, vars)
	if err != nil {
		return "", "", "", err
	}
	return rendered.Subject, rendered.HTML, rendered.Text, nil
}

// GetEffective returns the active template source for a key.
func (s *Service) GetEffective(ctx context.Context, key string) (*EffectiveTemplate, error) {
	// Try custom override first (only if store is available).
	var custom *Template
	if s.store != nil {
		var err error
		custom, err = s.store.Get(ctx, key)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("getting custom template %q: %w", key, err)
			}
			custom = nil
		}
	}
	if custom != nil && custom.Enabled {
		et := &EffectiveTemplate{
			Source:          "custom",
			TemplateKey:     key,
			SubjectTemplate: custom.SubjectTemplate,
			HTMLTemplate:    custom.HTMLTemplate,
			Enabled:         custom.Enabled,
		}
		if b, ok := s.builtins[key]; ok {
			et.Variables = b.Variables
		}
		return et, nil
	}

	// Fall back to built-in.
	builtin, ok := s.builtins[key]
	if !ok {
		// Check if we have a disabled custom template.
		if custom != nil {
			return &EffectiveTemplate{
				Source:          "custom",
				TemplateKey:     key,
				SubjectTemplate: custom.SubjectTemplate,
				HTMLTemplate:    custom.HTMLTemplate,
				Enabled:         false,
			}, nil
		}
		return nil, fmt.Errorf("%w: %q", ErrNoTemplate, key)
	}

	return &EffectiveTemplate{
		Source:          "builtin",
		TemplateKey:     key,
		SubjectTemplate: builtin.SubjectTemplate,
		HTMLTemplate:    builtin.HTMLTemplate,
		Enabled:         true,
		Variables:       builtin.Variables,
	}, nil
}

// Preview renders provided template strings without saving them.
func (s *Service) Preview(ctx context.Context, key, subjectTpl, htmlTpl string, vars map[string]string) (*RenderedEmail, error) {
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()
	return renderTemplates(ctx, key, subjectTpl, htmlTpl, vars)
}

// Send renders a template and sends it via the mailer.
func (s *Service) Send(ctx context.Context, key, to string, vars map[string]string) error {
	s.mu.RLock()
	m := s.mailer
	s.mu.RUnlock()

	if m == nil {
		return errors.New("mailer not configured")
	}

	rendered, err := s.Render(ctx, key, vars)
	if err != nil {
		return err
	}

	return m.Send(ctx, &mailer.Message{
		To:      to,
		Subject: rendered.Subject,
		HTML:    rendered.HTML,
		Text:    rendered.Text,
	})
}

// renderTemplates parses and executes subject + HTML templates against vars.
func renderTemplates(ctx context.Context, key, subjectTpl, htmlTpl string, vars map[string]string) (*RenderedEmail, error) {
	// Parse subject (text/template).
	st, err := parseSubject(key, subjectTpl)
	if err != nil {
		return nil, fmt.Errorf("%w: subject: %v", ErrParseFailed, err)
	}

	// Parse HTML (html/template).
	ht, err := parseHTML(key, htmlTpl)
	if err != nil {
		return nil, fmt.Errorf("%w: html: %v", ErrParseFailed, err)
	}

	// Check context before execution.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRenderFailed, err)
	}

	// Execute subject.
	var subBuf strings.Builder
	if err := st.Execute(&subBuf, vars); err != nil {
		return nil, fmt.Errorf("%w: subject: %v", ErrRenderFailed, err)
	}

	// Execute HTML.
	var htmlBuf strings.Builder
	if err := ht.Execute(&htmlBuf, vars); err != nil {
		return nil, fmt.Errorf("%w: html: %v", ErrRenderFailed, err)
	}

	html := htmlBuf.String()
	text := stripHTML(html)

	return &RenderedEmail{
		Subject: subBuf.String(),
		HTML:    html,
		Text:    text,
	}, nil
}

// parseSubject parses a subject template string with missingkey=error.
func parseSubject(key, tpl string) (*texttemplate.Template, error) {
	return texttemplate.New(key + ".subject").
		Option("missingkey=error").
		Parse(tpl)
}

// parseHTML parses an HTML template string with missingkey=error and empty FuncMap.
func parseHTML(key, tpl string) (*htmltemplate.Template, error) {
	return htmltemplate.New(key + ".html").
		Option("missingkey=error").
		Funcs(htmltemplate.FuncMap{}).
		Parse(tpl)
}

// stripHTML removes HTML tags, decodes HTML entities, and collapses whitespace
// for plaintext fallback.
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
	// Decode HTML entities (e.g. &amp; → &, &#39; → ', &lt; → <).
	decoded := html.UnescapeString(out.String())
	lines := strings.Split(decoded, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return strings.Join(result, "\n")
}
