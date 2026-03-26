// Package server Email template administration handlers provide HTTP endpoints for managing, previewing, and sending email templates with variable substitution.
package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/emailtemplates"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// emailTemplateAdmin is the interface for email template admin operations.
type emailTemplateAdmin interface {
	List(ctx context.Context) ([]*emailtemplates.Template, error)
	Upsert(ctx context.Context, key, subjectTpl, htmlTpl string) (*emailtemplates.Template, error)
	Delete(ctx context.Context, key string) error
	SetEnabled(ctx context.Context, key string, enabled bool) error
	GetEffective(ctx context.Context, key string) (*emailtemplates.EffectiveTemplate, error)
	Preview(ctx context.Context, key, subjectTpl, htmlTpl string, vars map[string]string) (*emailtemplates.RenderedEmail, error)
	Render(ctx context.Context, key string, vars map[string]string) (*emailtemplates.RenderedEmail, error)
	Send(ctx context.Context, key, to string, vars map[string]string) error
	SystemKeys() []emailtemplates.EffectiveTemplate
}

// Response types.

type emailTemplateListItem struct {
	TemplateKey     string `json:"templateKey"`
	Source          string `json:"source"`
	SubjectTemplate string `json:"subjectTemplate"`
	Enabled         bool   `json:"enabled"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
}

type emailTemplateListResponse struct {
	Items []emailTemplateListItem `json:"items"`
	Count int                     `json:"count"`
}

type upsertEmailTemplateRequest struct {
	SubjectTemplate string `json:"subjectTemplate"`
	HTMLTemplate    string `json:"htmlTemplate"`
}

type patchEmailTemplateRequest struct {
	Enabled *bool `json:"enabled"`
}

type previewEmailTemplateRequest struct {
	SubjectTemplate string            `json:"subjectTemplate"`
	HTMLTemplate    string            `json:"htmlTemplate"`
	Variables       map[string]string `json:"variables"`
}

type previewEmailTemplateResponse struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
}

type sendEmailRequest struct {
	TemplateKey string            `json:"templateKey"`
	To          string            `json:"to"`
	Variables   map[string]string `json:"variables"`
}

type effectiveTemplateResponse struct {
	Source          string   `json:"source"`
	TemplateKey     string   `json:"templateKey"`
	SubjectTemplate string   `json:"subjectTemplate"`
	HTMLTemplate    string   `json:"htmlTemplate"`
	Enabled         bool     `json:"enabled"`
	Variables       []string `json:"variables,omitempty"`
}

// Returns an HTTP handler that lists all email templates, merging system-provided builtin templates with user-created custom overrides and returning them with their source, subject, enabled status, and last update time.
func handleAdminListEmailTemplates(svc emailTemplateAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		custom, err := svc.List(r.Context())
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list email templates")
			return
		}

		// Build list: start with system keys, then add custom overrides.
		customMap := make(map[string]*emailtemplates.Template, len(custom))
		for _, t := range custom {
			customMap[t.TemplateKey] = t
		}

		var items []emailTemplateListItem

		// Add system keys (always shown).
		for _, sk := range svc.SystemKeys() {
			item := emailTemplateListItem{
				TemplateKey:     sk.TemplateKey,
				Source:          "builtin",
				SubjectTemplate: sk.SubjectTemplate,
				Enabled:         true,
			}
			if c, ok := customMap[sk.TemplateKey]; ok {
				item.Source = "custom"
				item.SubjectTemplate = c.SubjectTemplate
				item.Enabled = c.Enabled
				if !c.UpdatedAt.IsZero() {
					item.UpdatedAt = c.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
				}
				delete(customMap, sk.TemplateKey)
			}
			items = append(items, item)
		}

		// Add remaining custom templates (non-system keys).
		for _, t := range custom {
			if _, remaining := customMap[t.TemplateKey]; !remaining {
				// System key override already merged above.
				continue
			}
			item := emailTemplateListItem{
				TemplateKey:     t.TemplateKey,
				Source:          "custom",
				SubjectTemplate: t.SubjectTemplate,
				Enabled:         t.Enabled,
			}
			if !t.UpdatedAt.IsZero() {
				item.UpdatedAt = t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
			}
			items = append(items, item)
		}

		httputil.WriteJSON(w, http.StatusOK, emailTemplateListResponse{
			Items: items,
			Count: len(items),
		})
	}
}

// Returns an HTTP handler that retrieves the effective email template for a given key from the URL path, returning the merged result of any custom override and the system template, including all rendered variables.
func handleAdminGetEmailTemplate(svc emailTemplateAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		if err := emailtemplates.ValidateKey(key); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		eff, err := svc.GetEffective(r.Context(), key)
		if err != nil {
			if errors.Is(err, emailtemplates.ErrNoTemplate) {
				httputil.WriteError(w, http.StatusNotFound, "template not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get template")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, effectiveTemplateResponse{
			Source:          eff.Source,
			TemplateKey:     eff.TemplateKey,
			SubjectTemplate: eff.SubjectTemplate,
			HTMLTemplate:    eff.HTMLTemplate,
			Enabled:         eff.Enabled,
			Variables:       eff.Variables,
		})
	}
}

// Returns an HTTP handler that creates or updates an email template identified by key in the URL, accepting a subject and HTML template body in the request, validating the template syntax before persisting.
func handleAdminUpsertEmailTemplate(svc emailTemplateAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")

		if err := emailtemplates.ValidateKey(key); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		var req upsertEmailTemplateRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.SubjectTemplate == "" {
			httputil.WriteError(w, http.StatusBadRequest, "subjectTemplate is required")
			return
		}
		if req.HTMLTemplate == "" {
			httputil.WriteError(w, http.StatusBadRequest, "htmlTemplate is required")
			return
		}

		t, err := svc.Upsert(r.Context(), key, req.SubjectTemplate, req.HTMLTemplate)
		if err != nil {
			if errors.Is(err, emailtemplates.ErrInvalidKey) ||
				errors.Is(err, emailtemplates.ErrParseFailed) ||
				errors.Is(err, emailtemplates.ErrTooLarge) {
				httputil.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to save template")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, t)
	}
}

// Returns an HTTP handler that deletes the custom email template identified by key in the URL, returning no content on success.
func handleAdminDeleteEmailTemplate(svc emailTemplateAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		if err := emailtemplates.ValidateKey(key); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		err := svc.Delete(r.Context(), key)
		if err != nil {
			if errors.Is(err, emailtemplates.ErrNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "template not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to delete template")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Returns an HTTP handler that updates only the enabled status of an email template identified by key in the URL, accepting a boolean enabled field in the request.
func handleAdminPatchEmailTemplate(svc emailTemplateAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		if err := emailtemplates.ValidateKey(key); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		var req patchEmailTemplateRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.Enabled == nil {
			httputil.WriteError(w, http.StatusBadRequest, "enabled field is required")
			return
		}

		err := svc.SetEnabled(r.Context(), key, *req.Enabled)
		if err != nil {
			if errors.Is(err, emailtemplates.ErrNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "template not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update template")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"templateKey": key,
			"enabled":     *req.Enabled,
		})
	}
}

// Returns an HTTP handler that renders an email template with provided variables for preview purposes without sending or persisting, accepting subject, HTML template, and variable map in the request.
func handleAdminPreviewEmailTemplate(svc emailTemplateAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		if err := emailtemplates.ValidateKey(key); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		var req previewEmailTemplateRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.SubjectTemplate == "" {
			httputil.WriteError(w, http.StatusBadRequest, "subjectTemplate is required")
			return
		}
		if req.HTMLTemplate == "" {
			httputil.WriteError(w, http.StatusBadRequest, "htmlTemplate is required")
			return
		}

		vars := req.Variables
		if vars == nil {
			vars = map[string]string{}
		}

		rendered, err := svc.Preview(r.Context(), key, req.SubjectTemplate, req.HTMLTemplate, vars)
		if err != nil {
			if errors.Is(err, emailtemplates.ErrParseFailed) || errors.Is(err, emailtemplates.ErrRenderFailed) {
				httputil.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to preview template")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, previewEmailTemplateResponse{
			Subject: rendered.Subject,
			HTML:    rendered.HTML,
			Text:    rendered.Text,
		})
	}
}

// Returns an HTTP handler that sends an email using a stored template, accepting the template key, recipient address, and template variables in the request, validating the recipient email format before transmission.
func handleAdminSendEmail(svc emailTemplateAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req sendEmailRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.TemplateKey == "" {
			httputil.WriteError(w, http.StatusBadRequest, "templateKey is required")
			return
		}
		if err := emailtemplates.ValidateKey(req.TemplateKey); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.To == "" {
			httputil.WriteError(w, http.StatusBadRequest, "to is required")
			return
		}
		if err := httputil.ValidateEmail(req.To); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid email address")
			return
		}

		vars := req.Variables
		if vars == nil {
			vars = map[string]string{}
		}

		err := svc.Send(r.Context(), req.TemplateKey, req.To, vars)
		if err != nil {
			if errors.Is(err, emailtemplates.ErrNoTemplate) {
				httputil.WriteError(w, http.StatusNotFound, "template not found")
				return
			}
			if errors.Is(err, emailtemplates.ErrParseFailed) ||
				errors.Is(err, emailtemplates.ErrRenderFailed) ||
				errors.Is(err, emailtemplates.ErrTooLarge) {
				httputil.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to send email")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "sent",
		})
	}
}

// --- Server delegation methods (nil-check + dispatch) ---

func (s *Server) handleEmailTemplatesList(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		serviceUnavailable(w, serviceUnavailableEmailTemplates)
		return
	}
	handleAdminListEmailTemplates(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailTemplatesGet(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		serviceUnavailable(w, serviceUnavailableEmailTemplates)
		return
	}
	handleAdminGetEmailTemplate(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailTemplatesUpsert(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		serviceUnavailable(w, serviceUnavailableEmailTemplates)
		return
	}
	handleAdminUpsertEmailTemplate(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailTemplatesDelete(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		serviceUnavailable(w, serviceUnavailableEmailTemplates)
		return
	}
	handleAdminDeleteEmailTemplate(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailTemplatesPatch(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		serviceUnavailable(w, serviceUnavailableEmailTemplates)
		return
	}
	handleAdminPatchEmailTemplate(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailTemplatesPreview(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		serviceUnavailable(w, serviceUnavailableEmailTemplates)
		return
	}
	handleAdminPreviewEmailTemplate(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailSend(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		serviceUnavailable(w, serviceUnavailableEmailTemplates)
		return
	}
	handleAdminSendEmail(s.emailTplSvc).ServeHTTP(w, r)
}
