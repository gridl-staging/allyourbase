package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/emailtemplates"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// fakeEmailTemplateAdmin implements emailTemplateAdmin for handler tests.
type fakeEmailTemplateAdmin struct {
	templates  map[string]*emailtemplates.Template
	upsertErr  error
	deleteErr  error
	enableErr  error
	sendKey    string
	sendTo     string
	sendVars   map[string]string
	sendCalled bool
	sendErr    error
	previewErr error
	// Preview captures args to verify the handler passes them correctly.
	previewKey     string
	previewSubject string
	previewHTML    string
	previewVars    map[string]string
}

func newFakeEmailTemplateAdmin() *fakeEmailTemplateAdmin {
	return &fakeEmailTemplateAdmin{
		templates: make(map[string]*emailtemplates.Template),
	}
}

func (f *fakeEmailTemplateAdmin) List(ctx context.Context) ([]*emailtemplates.Template, error) {
	var result []*emailtemplates.Template
	for _, t := range f.templates {
		result = append(result, t)
	}
	return result, nil
}

func (f *fakeEmailTemplateAdmin) Upsert(ctx context.Context, key, subjectTpl, htmlTpl string) (*emailtemplates.Template, error) {
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	t := &emailtemplates.Template{
		ID:              "00000000-0000-0000-0000-000000000001",
		TemplateKey:     key,
		SubjectTemplate: subjectTpl,
		HTMLTemplate:    htmlTpl,
		Enabled:         true,
	}
	f.templates[key] = t
	return t, nil
}

func (f *fakeEmailTemplateAdmin) Delete(ctx context.Context, key string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.templates[key]; !ok {
		return emailtemplates.ErrNotFound
	}
	delete(f.templates, key)
	return nil
}

func (f *fakeEmailTemplateAdmin) SetEnabled(ctx context.Context, key string, enabled bool) error {
	if f.enableErr != nil {
		return f.enableErr
	}
	t, ok := f.templates[key]
	if !ok {
		return emailtemplates.ErrNotFound
	}
	t.Enabled = enabled
	return nil
}

func (f *fakeEmailTemplateAdmin) GetEffective(ctx context.Context, key string) (*emailtemplates.EffectiveTemplate, error) {
	t, ok := f.templates[key]
	if ok && t.Enabled {
		return &emailtemplates.EffectiveTemplate{
			Source:          "custom",
			TemplateKey:     key,
			SubjectTemplate: t.SubjectTemplate,
			HTMLTemplate:    t.HTMLTemplate,
			Enabled:         true,
		}, nil
	}
	return nil, fmt.Errorf("%w: %q", emailtemplates.ErrNoTemplate, key)
}

func (f *fakeEmailTemplateAdmin) Preview(ctx context.Context, key, subjectTpl, htmlTpl string, vars map[string]string) (*emailtemplates.RenderedEmail, error) {
	if f.previewErr != nil {
		return nil, f.previewErr
	}
	f.previewKey = key
	f.previewSubject = subjectTpl
	f.previewHTML = htmlTpl
	f.previewVars = vars
	// Simulate real rendering by substituting vars in subject/html.
	subject := subjectTpl
	htmlOut := htmlTpl
	for k, v := range vars {
		subject = strings.ReplaceAll(subject, "{{."+k+"}}", v)
		htmlOut = strings.ReplaceAll(htmlOut, "{{."+k+"}}", v)
	}
	return &emailtemplates.RenderedEmail{
		Subject: subject,
		HTML:    htmlOut,
		Text:    "stripped",
	}, nil
}

func (f *fakeEmailTemplateAdmin) Send(ctx context.Context, key, to string, vars map[string]string) error {
	f.sendCalled = true
	f.sendKey = key
	f.sendTo = to
	f.sendVars = vars
	return f.sendErr
}

func (f *fakeEmailTemplateAdmin) SystemKeys() []emailtemplates.EffectiveTemplate {
	return []emailtemplates.EffectiveTemplate{
		{Source: "builtin", TemplateKey: "auth.email_verification", SubjectTemplate: "Verify your email", HTMLTemplate: "<p>Verify</p>", Enabled: true, Variables: []string{"AppName", "ActionURL"}},
		{Source: "builtin", TemplateKey: "auth.magic_link", SubjectTemplate: "Your login link", HTMLTemplate: "<p>Link</p>", Enabled: true, Variables: []string{"AppName", "ActionURL"}},
		{Source: "builtin", TemplateKey: "auth.password_reset", SubjectTemplate: "Reset your password", HTMLTemplate: "<p>Reset</p>", Enabled: true, Variables: []string{"AppName", "ActionURL"}},
	}
}

// helper to build a chi router with URL params for handlers that use chi.URLParam.
func emailTemplateRouter(svc emailTemplateAdmin) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/api/admin/email/templates", handleAdminListEmailTemplates(svc))
	r.Get("/api/admin/email/templates/{key}", handleAdminGetEmailTemplate(svc))
	r.Put("/api/admin/email/templates/{key}", handleAdminUpsertEmailTemplate(svc))
	r.Delete("/api/admin/email/templates/{key}", handleAdminDeleteEmailTemplate(svc))
	r.Patch("/api/admin/email/templates/{key}", handleAdminPatchEmailTemplate(svc))
	r.Post("/api/admin/email/templates/{key}/preview", handleAdminPreviewEmailTemplate(svc))
	r.Post("/api/admin/email/send", handleAdminSendEmail(svc))
	return r
}

func TestEmailTemplatesList(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	fake.templates["app.welcome"] = &emailtemplates.Template{
		TemplateKey:     "app.welcome",
		SubjectTemplate: "Welcome",
		HTMLTemplate:    "<p>Hi</p>",
		Enabled:         true,
	}
	router := emailTemplateRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/email/templates", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)

	var resp emailTemplateListResponse
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	// 3 system keys + 1 custom app key = 4.
	testutil.Equal(t, 4, resp.Count)

	// Verify system keys are present.
	keys := make(map[string]string, len(resp.Items))
	for _, item := range resp.Items {
		keys[item.TemplateKey] = item.Source
	}
	testutil.Equal(t, "builtin", keys["auth.password_reset"])
	testutil.Equal(t, "builtin", keys["auth.email_verification"])
	testutil.Equal(t, "builtin", keys["auth.magic_link"])
	testutil.Equal(t, "custom", keys["app.welcome"])
}

func TestEmailTemplatesGetEffective(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	fake.templates["auth.password_reset"] = &emailtemplates.Template{
		TemplateKey:     "auth.password_reset",
		SubjectTemplate: "Custom Reset",
		HTMLTemplate:    "<p>Custom</p>",
		Enabled:         true,
	}
	router := emailTemplateRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/email/templates/auth.password_reset", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)

	var resp effectiveTemplateResponse
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, "custom", resp.Source)
	testutil.Equal(t, "Custom Reset", resp.SubjectTemplate)
	testutil.Equal(t, "<p>Custom</p>", resp.HTMLTemplate)
}

func TestEmailTemplatesGetEffective_NotFound(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/email/templates/nonexistent.key", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEmailTemplatesGetEffective_InvalidKey(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	req := httptest.NewRequest("GET", "/api/admin/email/templates/INVALID", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesUpsert(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"subjectTemplate":"Hello {{.Name}}","htmlTemplate":"<p>Hi {{.Name}}</p>"}`
	req := httptest.NewRequest("PUT", "/api/admin/email/templates/app.welcome", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, "app.welcome", resp["templateKey"])
	testutil.Equal(t, "Hello {{.Name}}", resp["subjectTemplate"])
	testutil.Equal(t, "<p>Hi {{.Name}}</p>", resp["htmlTemplate"])
}

func TestEmailTemplatesUpsert_InvalidKey(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"subjectTemplate":"Hello","htmlTemplate":"<p>Hi</p>"}`
	req := httptest.NewRequest("PUT", "/api/admin/email/templates/INVALID", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesUpsert_EmptyBody(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	// Missing htmlTemplate.
	body := `{"subjectTemplate":"Hello"}`
	req := httptest.NewRequest("PUT", "/api/admin/email/templates/app.test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)

	// Missing subjectTemplate.
	body = `{"htmlTemplate":"<p>Hi</p>"}`
	req = httptest.NewRequest("PUT", "/api/admin/email/templates/app.test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesUpsert_ParseError(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	fake.upsertErr = fmt.Errorf("%w: html: unclosed action", emailtemplates.ErrParseFailed)
	router := emailTemplateRouter(fake)

	body := `{"subjectTemplate":"Hello","htmlTemplate":"<p>{{.Name</p>"}`
	req := httptest.NewRequest("PUT", "/api/admin/email/templates/app.test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesUpsert_TooLarge(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	fake.upsertErr = fmt.Errorf("%w: html exceeds 256000 characters", emailtemplates.ErrTooLarge)
	router := emailTemplateRouter(fake)

	body := `{"subjectTemplate":"Hello","htmlTemplate":"<p>Large</p>"}`
	req := httptest.NewRequest("PUT", "/api/admin/email/templates/app.test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesDelete(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	fake.templates["app.welcome"] = &emailtemplates.Template{TemplateKey: "app.welcome"}
	router := emailTemplateRouter(fake)

	req := httptest.NewRequest("DELETE", "/api/admin/email/templates/app.welcome", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNoContent, rec.Code)
}

func TestEmailTemplatesDelete_NotFound(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	req := httptest.NewRequest("DELETE", "/api/admin/email/templates/nonexistent.key", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEmailTemplatesDelete_InvalidKey(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	req := httptest.NewRequest("DELETE", "/api/admin/email/templates/INVALID", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesPatch_Toggle(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	fake.templates["app.welcome"] = &emailtemplates.Template{
		TemplateKey: "app.welcome",
		Enabled:     true,
	}
	router := emailTemplateRouter(fake)

	body := `{"enabled":false}`
	req := httptest.NewRequest("PATCH", "/api/admin/email/templates/app.welcome", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.True(t, !fake.templates["app.welcome"].Enabled, "should be disabled")
}

func TestEmailTemplatesPatch_NotFound(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"enabled":false}`
	req := httptest.NewRequest("PATCH", "/api/admin/email/templates/nonexistent.key", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEmailTemplatesPatch_InvalidKey(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"enabled":true}`
	req := httptest.NewRequest("PATCH", "/api/admin/email/templates/INVALID", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesPreview_PassesArgsCorrectly(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"subjectTemplate":"Hi {{.Name}}","htmlTemplate":"<p>Hello {{.Name}}</p>","variables":{"Name":"Alice"}}`
	req := httptest.NewRequest("POST", "/api/admin/email/templates/app.test/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)

	// Verify the handler passed the correct arguments to Preview.
	testutil.Equal(t, "app.test", fake.previewKey)
	testutil.Equal(t, "Hi {{.Name}}", fake.previewSubject)
	testutil.Equal(t, "<p>Hello {{.Name}}</p>", fake.previewHTML)
	testutil.Equal(t, "Alice", fake.previewVars["Name"])

	// Verify the response contains the rendered output.
	var resp previewEmailTemplateResponse
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	testutil.Equal(t, "Hi Alice", resp.Subject)
	testutil.True(t, strings.Contains(resp.HTML, "Hello Alice"),
		"HTML should contain rendered Name, got: %s", resp.HTML)
}

func TestEmailTemplatesPreview_NilVarsInitialized(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	// No "variables" field in request body.
	body := `{"subjectTemplate":"Static subject","htmlTemplate":"<p>Static</p>"}`
	req := httptest.NewRequest("POST", "/api/admin/email/templates/app.test/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
	// Verify vars was initialized to empty map, not nil.
	testutil.True(t, fake.previewVars != nil, "vars should be initialized to empty map, not nil")
}

func TestEmailTemplatesPreview_MissingFields(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	// Missing htmlTemplate.
	body := `{"subjectTemplate":"Hello"}`
	req := httptest.NewRequest("POST", "/api/admin/email/templates/app.test/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusBadRequest, rec.Code)

	// Missing subjectTemplate.
	body = `{"htmlTemplate":"<p>Hello</p>"}`
	req = httptest.NewRequest("POST", "/api/admin/email/templates/app.test/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesPreview_InvalidKey(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"subjectTemplate":"Static subject","htmlTemplate":"<p>Static</p>"}`
	req := httptest.NewRequest("POST", "/api/admin/email/templates/INVALID/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesPreview_MissingVariableErrorMappedToBadRequest(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	fake.previewErr = fmt.Errorf("%w: missing variable ActionURL", emailtemplates.ErrRenderFailed)
	router := emailTemplateRouter(fake)

	body := `{"subjectTemplate":"Hi {{.AppName}}","htmlTemplate":"<p>{{.ActionURL}}</p>","variables":{"AppName":"Sigil"}}`
	req := httptest.NewRequest("POST", "/api/admin/email/templates/auth.password_reset/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesSend_PassesArgsCorrectly(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"templateKey":"app.welcome","to":"user@example.com","variables":{"Name":"Bob"}}`
	req := httptest.NewRequest("POST", "/api/admin/email/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.True(t, fake.sendCalled, "send should have been called")
	testutil.Equal(t, "app.welcome", fake.sendKey)
	testutil.Equal(t, "user@example.com", fake.sendTo)
	testutil.Equal(t, "Bob", fake.sendVars["Name"])
}

func TestEmailTemplatesSend_MissingTemplateKey(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"to":"user@example.com","variables":{"Name":"Bob"}}`
	req := httptest.NewRequest("POST", "/api/admin/email/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
	testutil.True(t, !fake.sendCalled, "send should NOT have been called for missing key")
}

func TestEmailTemplatesSend_MissingTo(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"templateKey":"app.welcome","variables":{"Name":"Bob"}}`
	req := httptest.NewRequest("POST", "/api/admin/email/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesSend_InvalidEmail(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"templateKey":"app.welcome","to":"not-an-email"}`
	req := httptest.NewRequest("POST", "/api/admin/email/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
	testutil.True(t, !fake.sendCalled, "send should NOT have been called for invalid email")
}

func TestEmailTemplatesSend_InvalidKey(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	router := emailTemplateRouter(fake)

	body := `{"templateKey":"INVALID","to":"user@example.com"}`
	req := httptest.NewRequest("POST", "/api/admin/email/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
	testutil.True(t, !fake.sendCalled, "send should NOT have been called for invalid key")
}

func TestEmailTemplatesSend_RenderValidationErrorMappedToBadRequest(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	fake.sendErr = fmt.Errorf("%w: missing variable ActionURL", emailtemplates.ErrRenderFailed)
	router := emailTemplateRouter(fake)

	body := `{"templateKey":"app.welcome","to":"user@example.com","variables":{"AppName":"Sigil"}}`
	req := httptest.NewRequest("POST", "/api/admin/email/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailTemplatesSend_TemplateNotFoundMappedToNotFound(t *testing.T) {
	t.Parallel()
	fake := newFakeEmailTemplateAdmin()
	fake.sendErr = fmt.Errorf("%w: app.missing", emailtemplates.ErrNoTemplate)
	router := emailTemplateRouter(fake)

	body := `{"templateKey":"app.missing","to":"user@example.com","variables":{"Name":"Alice"}}`
	req := httptest.NewRequest("POST", "/api/admin/email/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIsValidEmailAddress(t *testing.T) {
	t.Parallel()

	valid := []string{"user@example.com", "a@b.co", "test+tag@domain.org"}
	for _, email := range valid {
		testutil.True(t, isValidEmailAddress(email), "%q should be valid", email)
	}

	invalid := []string{
		"",
		"nodomain",
		"@example.com",
		"user@",
		"user@nodot",
		"user@example.com\r\nBcc:evil@example.com",
		"Display Name <user@example.com>",
	}
	for _, email := range invalid {
		testutil.True(t, !isValidEmailAddress(email), "%q should be invalid", email)
	}
}
