package server

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"

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
		if !isValidEmailAddress(req.To) {
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

// isValidEmailAddress performs basic email format validation at the API boundary.
func isValidEmailAddress(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" || strings.ContainsAny(email, "\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	// Only accept plain addr-spec form (no display names).
	if addr.Address != email {
		return false
	}
	atIdx := strings.LastIndex(email, "@")
	if atIdx < 1 || atIdx == len(email)-1 {
		return false
	}
	domain := email[atIdx+1:]
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return false
	}
	return strings.Contains(domain, ".")
}
