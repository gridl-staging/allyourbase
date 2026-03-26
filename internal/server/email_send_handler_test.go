package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/emailtemplates"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

// --- Fakes ---

// mockMailer records Send calls and returns configurable errors per recipient.
type mockMailer struct {
	mu        sync.Mutex
	calls     []*mailer.Message
	failAddrs map[string]error // per-recipient errors
	err       error            // global error for all calls
}

func (m *mockMailer) Send(_ context.Context, msg *mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, msg)
	if m.err != nil {
		return m.err
	}
	if m.failAddrs != nil {
		if err, ok := m.failAddrs[msg.To]; ok {
			return err
		}
	}
	return nil
}

func (m *mockMailer) sentMessages() []*mailer.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*mailer.Message, len(m.calls))
	copy(cp, m.calls)
	return cp
}

// mockEmailTplSvc implements emailTemplateAdmin for template send tests.
type mockEmailTplSvc struct {
	fakeEmailTemplateAdmin
	renderResult *emailtemplates.RenderedEmail
	renderErr    error
	renderKey    string
	renderVars   map[string]string
}

func (m *mockEmailTplSvc) Render(_ context.Context, key string, vars map[string]string) (*emailtemplates.RenderedEmail, error) {
	m.renderKey = key
	m.renderVars = vars
	if m.renderErr != nil {
		return nil, m.renderErr
	}
	if m.renderResult != nil {
		return m.renderResult, nil
	}
	return &emailtemplates.RenderedEmail{
		Subject: "Rendered Subject",
		HTML:    "<p>Rendered</p>",
		Text:    "Rendered",
	}, nil
}

// --- Test helpers ---

func emailTestServer(opts ...func(*Server)) *Server {
	s := &Server{
		cfg: &config.Config{
			Email: config.EmailConfig{
				From:     "default@example.com",
				FromName: "Test",
			},
		},
		mailer: &mockMailer{},
		logger: testutil.DiscardLogger(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func emailReq(t *testing.T, body string, claims *auth.Claims) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/email/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if claims != nil {
		ctx := auth.ContextWithClaims(req.Context(), claims)
		req = req.WithContext(ctx)
	}
	return req
}

func emailWriteClaims() *auth.Claims {
	return &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
		Email:            "test@example.com",
		APIKeyID:         "key-1",
		APIKeyScope:      "readwrite",
	}
}

func emailReadonlyClaims() *auth.Claims {
	return &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
		Email:            "test@example.com",
		APIKeyScope:      "readonly",
	}
}

func decodeEmailResp(t *testing.T, w *httptest.ResponseRecorder) emailSendResponse {
	t.Helper()
	var resp emailSendResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// ── Item 20: Validation tests ───────────────────────────────────────────

func TestPublicEmailSend_NoAuth(t *testing.T) {
	t.Parallel()
	srv := emailTestServer()
	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"a@b.com","subject":"Hi","html":"<p>Hi</p>"}`, nil)
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPublicEmailSend_ReadOnlyScope(t *testing.T) {
	t.Parallel()
	srv := emailTestServer()
	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"a@b.com","subject":"Hi","html":"<p>Hi</p>"}`, emailReadonlyClaims())
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
}

func TestPublicEmailSend_MissingTo(t *testing.T) {
	t.Parallel()
	srv := emailTestServer()
	w := httptest.NewRecorder()
	req := emailReq(t, `{"subject":"Hi","html":"<p>Hi</p>"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "to is required")
}

func TestPublicEmailSend_MissingSubject(t *testing.T) {
	t.Parallel()
	srv := emailTestServer()
	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"a@b.com","html":"<p>Hi</p>"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "subject is required")
}

func TestPublicEmailSend_NoBodyOrTemplate(t *testing.T) {
	t.Parallel()
	srv := emailTestServer()
	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"a@b.com","subject":"Hi"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "must specify either html/text or templateKey")
}

func TestPublicEmailSend_MutualExclusion(t *testing.T) {
	t.Parallel()
	srv := emailTestServer()
	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"a@b.com","subject":"Hi","html":"<p>Hi</p>","templateKey":"auth.verify"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "cannot specify both")
}

func TestPublicEmailSend_InvalidEmail(t *testing.T) {
	t.Parallel()
	srv := emailTestServer()
	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"not-an-email","subject":"Hi","html":"<p>Hi</p>"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid email address")
}

func TestPublicEmailSend_FromNotAllowed(t *testing.T) {
	t.Parallel()
	srv := emailTestServer()
	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"a@b.com","subject":"Hi","html":"<p>Hi</p>","from":"evil@attacker.com"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "from address not allowed")
}

func TestPublicEmailSend_TooManyRecipients(t *testing.T) {
	t.Parallel()
	srv := emailTestServer(func(s *Server) {
		s.cfg.Email.Policy.MaxRecipientsPerRequest = 2
	})
	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":["a@b.com","c@d.com","e@f.com"],"subject":"Hi","html":"<p>Hi</p>"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "too many recipients")
}

func TestPublicEmailSend_FromNotAllowedWinsBeforeRecipientLimit(t *testing.T) {
	t.Parallel()
	srv := emailTestServer(func(s *Server) {
		s.cfg.Email.Policy.MaxRecipientsPerRequest = 1
	})
	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":["a@b.com","c@d.com"],"subject":"Hi","html":"<p>Hi</p>","from":"evil@attacker.com"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "from address not allowed")
}

func TestPublicEmailSend_NoMailerConfigured(t *testing.T) {
	t.Parallel()
	srv := emailTestServer(func(s *Server) {
		s.mailer = nil
	})
	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"a@b.com","subject":"Hi","html":"<p>Hi</p>"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)
	testutil.Equal(t, http.StatusNotImplemented, w.Code)
}

// ── Item 21: Policy and flow tests ──────────────────────────────────────

func TestPublicEmailSend_RateLimitExceeded(t *testing.T) {
	t.Parallel()
	rl := auth.NewRateLimiter(1, time.Hour)
	defer rl.Stop()
	srv := emailTestServer(func(s *Server) {
		s.emailRL = rl
	})

	// First call should succeed.
	w1 := httptest.NewRecorder()
	req1 := emailReq(t, `{"to":"a@b.com","subject":"Hi","html":"<p>Hi</p>"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w1, req1)
	testutil.Equal(t, http.StatusOK, w1.Code)

	// Second call should be rate-limited.
	w2 := httptest.NewRecorder()
	req2 := emailReq(t, `{"to":"a@b.com","subject":"Hi","html":"<p>Hi</p>"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w2, req2)
	testutil.Equal(t, http.StatusTooManyRequests, w2.Code)
	testutil.True(t, w2.Header().Get("Retry-After") != "", "should set Retry-After header")
}

func TestPublicEmailSend_DirectSendSuccess(t *testing.T) {
	t.Parallel()
	ml := &mockMailer{}
	srv := emailTestServer(func(s *Server) { s.mailer = ml })

	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"user@example.com","subject":"Hello","html":"<p>World</p>","text":"World"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	resp := decodeEmailResp(t, w)
	testutil.Equal(t, 1, resp.Sent)
	testutil.Equal(t, 0, resp.Failed)

	msgs := ml.sentMessages()
	testutil.SliceLen(t, msgs, 1)
	testutil.Equal(t, "user@example.com", msgs[0].To)
	testutil.Equal(t, "Hello", msgs[0].Subject)
	testutil.Equal(t, "<p>World</p>", msgs[0].HTML)
	testutil.Equal(t, "World", msgs[0].Text)
	testutil.Equal(t, "default@example.com", msgs[0].From)
}

func TestPublicEmailSend_TemplateSendSuccess(t *testing.T) {
	t.Parallel()
	ml := &mockMailer{}
	tplSvc := &mockEmailTplSvc{
		renderResult: &emailtemplates.RenderedEmail{
			Subject: "Welcome!",
			HTML:    "<h1>Welcome</h1>",
			Text:    "Welcome",
		},
	}
	srv := emailTestServer(func(s *Server) {
		s.mailer = ml
		s.emailTplSvc = tplSvc
	})

	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"user@example.com","templateKey":"onboarding.welcome","variables":{"name":"Alice"}}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	resp := decodeEmailResp(t, w)
	testutil.Equal(t, 1, resp.Sent)

	// Verify template was rendered with correct key and vars.
	testutil.Equal(t, "onboarding.welcome", tplSvc.renderKey)
	testutil.Equal(t, "Alice", tplSvc.renderVars["name"])

	// Verify mailer received rendered content.
	msgs := ml.sentMessages()
	testutil.SliceLen(t, msgs, 1)
	testutil.Equal(t, "Welcome!", msgs[0].Subject)
	testutil.Equal(t, "<h1>Welcome</h1>", msgs[0].HTML)
}

func TestPublicEmailSend_TemplateNotFound(t *testing.T) {
	t.Parallel()
	tplSvc := &mockEmailTplSvc{
		renderErr: fmt.Errorf("%w: %q", emailtemplates.ErrNoTemplate, "missing.key"),
	}
	srv := emailTestServer(func(s *Server) {
		s.emailTplSvc = tplSvc
	})

	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"user@example.com","templateKey":"missing.key"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "template not found")
}

func TestPublicEmailSend_PartialFailure(t *testing.T) {
	t.Parallel()
	ml := &mockMailer{
		failAddrs: map[string]error{
			"fail@example.com": errors.New("SMTP timeout"),
		},
	}
	srv := emailTestServer(func(s *Server) { s.mailer = ml })

	w := httptest.NewRecorder()
	body := `{"to":["ok1@example.com","fail@example.com","ok2@example.com"],"subject":"Hi","html":"<p>Hi</p>"}`
	req := emailReq(t, body, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	resp := decodeEmailResp(t, w)
	testutil.Equal(t, 2, resp.Sent)
	testutil.Equal(t, 1, resp.Failed)
	testutil.SliceLen(t, resp.Errors, 1)
	testutil.Equal(t, "fail@example.com", resp.Errors[0].To)
	testutil.Contains(t, resp.Errors[0].Error, "SMTP timeout")
}

func TestPublicEmailSend_DefaultFrom(t *testing.T) {
	t.Parallel()
	ml := &mockMailer{}
	srv := emailTestServer(func(s *Server) { s.mailer = ml })

	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"user@example.com","subject":"Hi","html":"<p>Hi</p>"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	msgs := ml.sentMessages()
	testutil.SliceLen(t, msgs, 1)
	testutil.Equal(t, "default@example.com", msgs[0].From)
}

func TestPublicEmailSend_ExplicitFromAllowed(t *testing.T) {
	t.Parallel()
	ml := &mockMailer{}
	srv := emailTestServer(func(s *Server) {
		s.mailer = ml
		s.cfg.Email.Policy.AllowedFromAddresses = []string{"custom@example.com", "default@example.com"}
	})

	w := httptest.NewRecorder()
	req := emailReq(t, `{"to":"user@example.com","subject":"Hi","html":"<p>Hi</p>","from":"custom@example.com"}`, emailWriteClaims())
	srv.handlePublicEmailSend(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	msgs := ml.sentMessages()
	testutil.SliceLen(t, msgs, 1)
	testutil.Equal(t, "custom@example.com", msgs[0].From)
}

// ── parseRecipients unit tests ──────────────────────────────────────────

func TestParseRecipients_String(t *testing.T) {
	t.Parallel()
	r, err := parseRecipients(json.RawMessage(`"user@example.com"`))
	testutil.NoError(t, err)
	testutil.SliceLen(t, r, 1)
	testutil.Equal(t, "user@example.com", r[0])
}

func TestParseRecipients_Array(t *testing.T) {
	t.Parallel()
	r, err := parseRecipients(json.RawMessage(`["a@b.com","c@d.com"]`))
	testutil.NoError(t, err)
	testutil.SliceLen(t, r, 2)
}

func TestParseRecipients_Empty(t *testing.T) {
	t.Parallel()
	r, err := parseRecipients(nil)
	testutil.NoError(t, err)
	testutil.SliceLen(t, r, 0)
}

func TestParseRecipients_Invalid(t *testing.T) {
	t.Parallel()
	_, err := parseRecipients(json.RawMessage(`123`))
	testutil.ErrorContains(t, err, "to must be a string or array")
}
