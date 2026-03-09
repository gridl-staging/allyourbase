package emailtemplates

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

type stubTemplateStore struct {
	upsertFn     func(ctx context.Context, key, subjectTpl, htmlTpl string) (*Template, error)
	getFn        func(ctx context.Context, key string) (*Template, error)
	listFn       func(ctx context.Context) ([]*Template, error)
	deleteFn     func(ctx context.Context, key string) error
	setEnabledFn func(ctx context.Context, key string, enabled bool) error
}

func (s *stubTemplateStore) Upsert(ctx context.Context, key, subjectTpl, htmlTpl string) (*Template, error) {
	if s.upsertFn != nil {
		return s.upsertFn(ctx, key, subjectTpl, htmlTpl)
	}
	return nil, errors.New("unexpected Upsert call")
}

func (s *stubTemplateStore) Get(ctx context.Context, key string) (*Template, error) {
	if s.getFn != nil {
		return s.getFn(ctx, key)
	}
	return nil, ErrNotFound
}

func (s *stubTemplateStore) List(ctx context.Context) ([]*Template, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return nil, nil
}

func (s *stubTemplateStore) Delete(ctx context.Context, key string) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, key)
	}
	return ErrNotFound
}

func (s *stubTemplateStore) SetEnabled(ctx context.Context, key string, enabled bool) error {
	if s.setEnabledFn != nil {
		return s.setEnabledFn(ctx, key, enabled)
	}
	return ErrNotFound
}

func TestValidateKey(t *testing.T) {
	t.Parallel()

	valid := []string{
		"auth.password_reset",
		"auth.email_verification",
		"auth.magic_link",
		"app.club_invite",
		"app.event_reminder",
		"notify.event_reminder",
		"a.b",
		"a1.b2",
		"foo.bar_baz",
		"x.y.z",
	}
	for _, key := range valid {
		if err := ValidateKey(key); err != nil {
			t.Errorf("key %q should be valid: %v", key, err)
		}
	}

	invalid := []string{
		"",
		"singleword",
		".leading.dot",
		"UPPER.case",
		"has space.key",
		"1starts.num",
		"a.",
		".a",
		"a..b",
		"a.B",
		"a.b-c",
	}
	for _, key := range invalid {
		err := ValidateKey(key)
		testutil.True(t, err != nil, "key %q should be invalid", key)
		testutil.True(t, errors.Is(err, ErrInvalidKey), "should be ErrInvalidKey for %q", key)
	}
}

func TestRenderTemplates_Basic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rendered, err := renderTemplates(ctx, "test.basic",
		"Hello {{.Name}}",
		"<p>Welcome {{.Name}} to {{.AppName}}</p>",
		map[string]string{"Name": "Alice", "AppName": "TestApp"},
	)
	testutil.NoError(t, err)
	testutil.Equal(t, "Hello Alice", rendered.Subject)
	testutil.True(t, strings.Contains(rendered.HTML, "Welcome Alice to TestApp"),
		"HTML should contain rendered values")
	testutil.True(t, strings.Contains(rendered.Text, "Welcome Alice to TestApp"),
		"Text should contain rendered values (tags stripped)")
	testutil.True(t, !strings.Contains(rendered.Text, "<p>"),
		"Text should not contain HTML tags")
}

func TestRenderTemplates_MissingVariable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, err := renderTemplates(ctx, "test.missing",
		"Hello {{.Name}}",
		"<p>Hello</p>",
		map[string]string{}, // Name is missing
	)
	testutil.True(t, err != nil, "missing variable should cause error")
	testutil.True(t, errors.Is(err, ErrRenderFailed),
		"should be ErrRenderFailed")
}

func TestRenderTemplates_MissingHTMLVariable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, err := renderTemplates(ctx, "test.missing_html",
		"Subject",
		"<p>Hello {{.Name}}</p>",
		map[string]string{}, // Name is missing
	)
	testutil.True(t, err != nil, "missing HTML variable should cause error")
	testutil.True(t, errors.Is(err, ErrRenderFailed),
		"should be ErrRenderFailed")
}

func TestRenderTemplates_InvalidTemplateSyntax(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Invalid subject template syntax.
	_, err := renderTemplates(ctx, "test.bad_subject",
		"Hello {{.Name",
		"<p>OK</p>",
		map[string]string{"Name": "Alice"},
	)
	testutil.True(t, err != nil, "invalid subject syntax should cause error")
	testutil.True(t, errors.Is(err, ErrParseFailed),
		"should be ErrParseFailed for bad subject")

	// Invalid HTML template syntax.
	_, err = renderTemplates(ctx, "test.bad_html",
		"OK",
		"<p>Hello {{.Name</p>",
		map[string]string{"Name": "Alice"},
	)
	testutil.True(t, err != nil, "invalid HTML syntax should cause error")
	testutil.True(t, errors.Is(err, ErrParseFailed),
		"should be ErrParseFailed for bad HTML")
}

func TestRenderTemplates_HTMLAutoEscaping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rendered, err := renderTemplates(ctx, "test.escape",
		"Subject",
		"<p>Hello {{.Name}}</p>",
		map[string]string{"Name": `<script>alert("xss")</script>`},
	)
	testutil.NoError(t, err)
	testutil.True(t, !strings.Contains(rendered.HTML, "<script>"),
		"HTML auto-escaping should prevent script injection")
	testutil.True(t, strings.Contains(rendered.HTML, "&lt;script&gt;"),
		"script tag should be escaped in HTML")
}

func TestRenderTemplates_HTMLAutoEscapingDecodedInPlaintext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Verify that html/template auto-escaped entities are decoded in the
	// plaintext fallback (e.g. O&#39;Brien → O'Brien, not raw entities).
	rendered, err := renderTemplates(ctx, "test.entity",
		"Subject",
		"<p>Hello {{.Name}}</p>",
		map[string]string{"Name": "O'Brien & Co"},
	)
	testutil.NoError(t, err)
	// HTML should have entities escaped.
	testutil.True(t, strings.Contains(rendered.HTML, "O&#39;Brien"),
		"HTML should escape apostrophe, got: %s", rendered.HTML)
	testutil.True(t, strings.Contains(rendered.HTML, "&amp;"),
		"HTML should escape ampersand, got: %s", rendered.HTML)
	// Plaintext should have entities decoded back.
	testutil.True(t, strings.Contains(rendered.Text, "O'Brien"),
		"Text should decode &#39; to apostrophe, got: %s", rendered.Text)
	testutil.True(t, strings.Contains(rendered.Text, "& Co"),
		"Text should decode &amp; to &, got: %s", rendered.Text)
}

func TestRenderTemplates_SSTIPrevention(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// map[string]string values are plain strings with no callable methods.
	_, err := renderTemplates(ctx, "test.ssti",
		"{{.Name.Method}}",
		"<p>Safe</p>",
		map[string]string{"Name": "Alice"},
	)
	testutil.True(t, err != nil, "method call on string should fail")
}

func TestStripHTML(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple paragraph",
			input: "<p>Hello World</p>",
			want:  "Hello World",
		},
		{
			name:  "nested tags",
			input: "<div><h1>Title</h1>\n<p>Body</p></div>",
			want:  "Title\nBody",
		},
		{
			name:  "link with attributes",
			input: `<a href="https://example.com" style="color:blue">Click here</a>`,
			want:  "Click here",
		},
		{
			name:  "no tags",
			input: "Plain text",
			want:  "Plain text",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace collapse",
			input: "<p>  Hello  </p>\n\n\n<p>  World  </p>",
			want:  "Hello\nWorld",
		},
		{
			name:  "html entities decoded",
			input: "<p>O&#39;Brien &amp; Co &lt;team&gt;</p>",
			want:  "O'Brien & Co <team>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripHTML(tc.input)
			testutil.Equal(t, tc.want, got)
		})
	}
}

func TestServiceRender_BuiltinFallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	builtins := map[string]BuiltinTemplate{
		"auth.password_reset": {
			SubjectTemplate: "Reset your password",
			HTMLTemplate:    "<p>Reset link for {{.AppName}}: {{.ActionURL}}</p>",
			Variables:       []string{"AppName", "ActionURL"},
		},
	}

	svc := NewService(nil, builtins)

	// Render should use builtin when no store exists.
	rendered, err := svc.Render(ctx, "auth.password_reset",
		map[string]string{"AppName": "TestApp", "ActionURL": "https://example.com/reset"})
	testutil.NoError(t, err)
	testutil.Equal(t, "Reset your password", rendered.Subject)
	testutil.True(t, strings.Contains(rendered.HTML, "TestApp"),
		"builtin render should include AppName")
	testutil.True(t, strings.Contains(rendered.HTML, "https://example.com/reset"),
		"builtin render should include ActionURL")
}

func TestServiceGetEffective_BuiltinSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	builtins := map[string]BuiltinTemplate{
		"auth.password_reset": {
			SubjectTemplate: "Reset your password",
			HTMLTemplate:    "<p>Reset</p>",
			Variables:       []string{"AppName", "ActionURL"},
		},
	}

	svc := NewService(nil, builtins)

	eff, err := svc.GetEffective(ctx, "auth.password_reset")
	testutil.NoError(t, err)
	testutil.Equal(t, "builtin", eff.Source)
	testutil.Equal(t, "Reset your password", eff.SubjectTemplate)
	testutil.Equal(t, 2, len(eff.Variables))
}

func TestServiceGetEffective_NoTemplate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc := NewService(nil, map[string]BuiltinTemplate{})

	_, err := svc.GetEffective(ctx, "nonexistent.key")
	testutil.True(t, err != nil, "should error for nonexistent key")
	testutil.True(t, errors.Is(err, ErrNoTemplate),
		"should be ErrNoTemplate for nonexistent key")
}

func TestServicePreview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc := NewService(nil, nil)
	rendered, err := svc.Preview(ctx, "test.preview",
		"Welcome {{.Name}}",
		"<h1>Hi {{.Name}}</h1>",
		map[string]string{"Name": "Bob"},
	)
	testutil.NoError(t, err)
	testutil.Equal(t, "Welcome Bob", rendered.Subject)
	testutil.True(t, strings.Contains(rendered.HTML, "Hi Bob"),
		"preview should render HTML")
}

func TestServiceRender_NoTemplate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc := NewService(nil, map[string]BuiltinTemplate{})
	_, err := svc.Render(ctx, "nonexistent.key", map[string]string{})
	testutil.True(t, err != nil, "should error for nonexistent key")
	testutil.True(t, errors.Is(err, ErrNoTemplate),
		"should be ErrNoTemplate")
}

func TestServiceRender_CustomOnlyMissingVariableReturnsRenderError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := &stubTemplateStore{
		getFn: func(_ context.Context, key string) (*Template, error) {
			if key != "app.club_invite" {
				return nil, ErrNotFound
			}
			return &Template{
				TemplateKey:     key,
				SubjectTemplate: "Invite {{.Name}}",
				HTMLTemplate:    "<p>Join {{.ClubName}}</p>",
				Enabled:         true,
			}, nil
		},
	}
	svc := NewService(store, map[string]BuiltinTemplate{})

	_, err := svc.Render(ctx, "app.club_invite", map[string]string{"Name": "Alice"})
	testutil.True(t, err != nil, "missing ClubName should cause render error")
	testutil.True(t, errors.Is(err, ErrRenderFailed), "should be ErrRenderFailed")
}

func TestServiceGetEffective_StoreError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := &stubTemplateStore{
		getFn: func(_ context.Context, _ string) (*Template, error) {
			return nil, errors.New("db temporarily unavailable")
		},
	}
	svc := NewService(store, map[string]BuiltinTemplate{
		"auth.password_reset": {
			SubjectTemplate: "Reset your password",
			HTMLTemplate:    "<p>Reset</p>",
		},
	})

	_, err := svc.GetEffective(ctx, "auth.password_reset")
	testutil.True(t, err != nil, "store error should be returned")
}

func TestSystemKeys_DeterministicOrder(t *testing.T) {
	t.Parallel()

	builtins := map[string]BuiltinTemplate{
		"auth.password_reset":     {SubjectTemplate: "Reset"},
		"auth.email_verification": {SubjectTemplate: "Verify"},
		"auth.magic_link":         {SubjectTemplate: "Link"},
	}

	svc := NewService(nil, builtins)

	// Call multiple times and verify order is always the same.
	for i := 0; i < 10; i++ {
		keys := svc.SystemKeys()
		testutil.Equal(t, 3, len(keys))
		testutil.Equal(t, "auth.email_verification", keys[0].TemplateKey)
		testutil.Equal(t, "auth.magic_link", keys[1].TemplateKey)
		testutil.Equal(t, "auth.password_reset", keys[2].TemplateKey)
	}
}

func TestDefaultBuiltins(t *testing.T) {
	t.Parallel()

	builtins := DefaultBuiltins()

	// Must have all three system template keys.
	expectedKeys := []string{"auth.password_reset", "auth.email_verification", "auth.magic_link"}
	for _, key := range expectedKeys {
		b, ok := builtins[key]
		testutil.True(t, ok, "DefaultBuiltins should contain %q", key)
		testutil.True(t, b.SubjectTemplate != "", "SubjectTemplate for %q should not be empty", key)
		testutil.True(t, b.HTMLTemplate != "", "HTMLTemplate for %q should not be empty", key)
		testutil.True(t, len(b.Variables) == 2, "Variables for %q should have 2 items (AppName, ActionURL), got %d", key, len(b.Variables))
		testutil.True(t, b.Variables[0] == "AppName" || b.Variables[1] == "AppName",
			"Variables for %q should contain AppName", key)
		testutil.True(t, b.Variables[0] == "ActionURL" || b.Variables[1] == "ActionURL",
			"Variables for %q should contain ActionURL", key)
	}

	// Verify subjects match mailer defaults.
	testutil.Equal(t, "Reset your password", builtins["auth.password_reset"].SubjectTemplate)
	testutil.Equal(t, "Verify your email", builtins["auth.email_verification"].SubjectTemplate)
	testutil.Equal(t, "Your login link", builtins["auth.magic_link"].SubjectTemplate)

	// Templates should be parseable.
	for key, b := range builtins {
		_, err := parseSubject(key, b.SubjectTemplate)
		testutil.NoError(t, err)
		_, err = parseHTML(key, b.HTMLTemplate)
		testutil.NoError(t, err)
	}

	// Templates should render with system variables.
	ctx := context.Background()
	vars := map[string]string{"AppName": "TestApp", "ActionURL": "https://example.com/action"}
	for key, b := range builtins {
		rendered, err := renderTemplates(ctx, key, b.SubjectTemplate, b.HTMLTemplate, vars)
		testutil.NoError(t, err)
		testutil.True(t, rendered.Subject != "", "rendered subject for %q should not be empty", key)
		testutil.True(t, strings.Contains(rendered.HTML, "TestApp"),
			"rendered HTML for %q should contain AppName", key)
		testutil.True(t, strings.Contains(rendered.HTML, "https://example.com/action"),
			"rendered HTML for %q should contain ActionURL", key)
	}
}

func TestSizeLimits(t *testing.T) {
	t.Parallel()

	// MaxSubjectLen exceeded.
	longSubject := strings.Repeat("a", MaxSubjectLen+1)
	err := ValidateKey("test.key") // key is valid
	testutil.NoError(t, err)

	// We test the Store.Upsert size checks directly since they don't need a DB.
	// Subject too long — should produce ErrTooLarge before any DB or parse call.
	store := &Store{pool: nil} // nil pool - won't reach DB
	_, err = store.Upsert(context.Background(), "test.key", longSubject, "<p>ok</p>")
	testutil.True(t, err != nil, "oversized subject should be rejected")
	testutil.True(t, errors.Is(err, ErrTooLarge), "should be ErrTooLarge, got: %v", err)

	// HTML too long.
	longHTML := strings.Repeat("x", MaxHTMLLen+1)
	_, err = store.Upsert(context.Background(), "test.key", "ok", longHTML)
	testutil.True(t, err != nil, "oversized HTML should be rejected")
	testutil.True(t, errors.Is(err, ErrTooLarge), "should be ErrTooLarge, got: %v", err)
}

func TestRenderTemplates_CancelledContext(t *testing.T) {
	t.Parallel()

	// A cancelled context should cause a render error, simulating timeout behavior.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := renderTemplates(ctx, "test.timeout",
		"Subject",
		"<p>Body</p>",
		map[string]string{},
	)
	testutil.True(t, err != nil, "cancelled context should cause render error")
	testutil.True(t, errors.Is(err, ErrRenderFailed),
		"should be ErrRenderFailed for cancelled context, got: %v", err)
}

func TestServiceRender_Timeout(t *testing.T) {
	t.Parallel()

	builtins := map[string]BuiltinTemplate{
		"auth.password_reset": {
			SubjectTemplate: "Reset",
			HTMLTemplate:    "<p>Reset {{.ActionURL}}</p>",
			Variables:       []string{"AppName", "ActionURL"},
		},
	}
	svc := NewService(nil, builtins)

	// Use an already-expired context to simulate timeout.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	_, err := svc.Render(ctx, "auth.password_reset",
		map[string]string{"AppName": "Test", "ActionURL": "https://example.com"})
	testutil.True(t, err != nil, "expired context should cause render error")
}

func TestServiceRenderWithFallback_StoreErrorFallsBackToBuiltin(t *testing.T) {
	t.Parallel()

	builtins := map[string]BuiltinTemplate{
		"auth.password_reset": {
			SubjectTemplate: "Reset your password",
			HTMLTemplate:    "<p>Reset link: {{.ActionURL}}</p>",
			Variables:       []string{"AppName", "ActionURL"},
		},
	}

	// Store returns a transient DB error — RenderWithFallback should
	// log and gracefully fall through to the built-in default.
	store := &stubTemplateStore{
		getFn: func(_ context.Context, _ string) (*Template, error) {
			return nil, errors.New("db connection refused")
		},
	}
	svc := NewService(store, builtins)

	subject, html, text, err := svc.RenderWithFallback(context.Background(), "auth.password_reset",
		map[string]string{"AppName": "TestApp", "ActionURL": "https://example.com/reset"})
	testutil.NoError(t, err)
	testutil.Equal(t, "Reset your password", subject)
	testutil.True(t, strings.Contains(html, "https://example.com/reset"), "should contain ActionURL")
	testutil.True(t, len(text) > 0, "plaintext should not be empty")
}

func TestServiceRenderWithFallback_NoTemplate(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, map[string]BuiltinTemplate{})

	_, _, _, err := svc.RenderWithFallback(context.Background(), "nonexistent.key", map[string]string{})
	testutil.True(t, err != nil, "should error for nonexistent key")
	testutil.True(t, errors.Is(err, ErrNoTemplate), "should be ErrNoTemplate")
}

func TestServiceRender_CustomTemplateSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := &stubTemplateStore{
		getFn: func(_ context.Context, key string) (*Template, error) {
			if key != "auth.password_reset" {
				return nil, ErrNotFound
			}
			return &Template{
				TemplateKey:     key,
				SubjectTemplate: "Custom Reset for {{.AppName}}",
				HTMLTemplate:    "<p>Custom reset: {{.ActionURL}}</p>",
				Enabled:         true,
			}, nil
		},
	}

	builtins := map[string]BuiltinTemplate{
		"auth.password_reset": {
			SubjectTemplate: "Reset your password",
			HTMLTemplate:    "<p>Builtin reset: {{.ActionURL}}</p>",
			Variables:       []string{"AppName", "ActionURL"},
		},
	}

	svc := NewService(store, builtins)
	rendered, err := svc.Render(ctx, "auth.password_reset",
		map[string]string{"AppName": "TestApp", "ActionURL": "https://example.com/reset"})
	testutil.NoError(t, err)
	testutil.Equal(t, "Custom Reset for TestApp", rendered.Subject)
	testutil.True(t, strings.Contains(rendered.HTML, "Custom reset:"),
		"should use custom template, not builtin")
}

func TestServiceRender_DisabledCustomFallsBackToBuiltin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := &stubTemplateStore{
		getFn: func(_ context.Context, key string) (*Template, error) {
			return &Template{
				TemplateKey:     key,
				SubjectTemplate: "Custom subject",
				HTMLTemplate:    "<p>Custom</p>",
				Enabled:         false, // disabled
			}, nil
		},
	}

	builtins := map[string]BuiltinTemplate{
		"auth.password_reset": {
			SubjectTemplate: "Builtin subject",
			HTMLTemplate:    "<p>Builtin: {{.ActionURL}}</p>",
			Variables:       []string{"AppName", "ActionURL"},
		},
	}

	svc := NewService(store, builtins)
	rendered, err := svc.Render(ctx, "auth.password_reset",
		map[string]string{"AppName": "TestApp", "ActionURL": "https://example.com/reset"})
	testutil.NoError(t, err)
	testutil.Equal(t, "Builtin subject", rendered.Subject)
	testutil.True(t, strings.Contains(rendered.HTML, "Builtin:"),
		"disabled custom should fall back to builtin")
}

func TestServiceRender_CustomRenderFailsFallsBackToBuiltin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Custom template has a missing variable — render fails, should fall back to builtin.
	store := &stubTemplateStore{
		getFn: func(_ context.Context, key string) (*Template, error) {
			return &Template{
				TemplateKey:     key,
				SubjectTemplate: "Custom {{.MissingVar}}",
				HTMLTemplate:    "<p>Custom</p>",
				Enabled:         true,
			}, nil
		},
	}

	builtins := map[string]BuiltinTemplate{
		"auth.password_reset": {
			SubjectTemplate: "Builtin subject",
			HTMLTemplate:    "<p>Builtin: {{.ActionURL}}</p>",
			Variables:       []string{"AppName", "ActionURL"},
		},
	}

	svc := NewService(store, builtins)
	rendered, err := svc.Render(ctx, "auth.password_reset",
		map[string]string{"AppName": "TestApp", "ActionURL": "https://example.com/reset"})
	testutil.NoError(t, err)
	testutil.Equal(t, "Builtin subject", rendered.Subject)
	testutil.True(t, strings.Contains(rendered.HTML, "Builtin:"),
		"failed custom should fall back to builtin")
}

func TestServiceRenderWithFallback_CustomRenderFailsFallsBackToBuiltin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Same graceful degradation path tested through the RenderWithFallback API
	// (which auth.renderAuthEmail calls).
	store := &stubTemplateStore{
		getFn: func(_ context.Context, key string) (*Template, error) {
			return &Template{
				TemplateKey:     key,
				SubjectTemplate: "Custom {{.MissingVar}}",
				HTMLTemplate:    "<p>Custom</p>",
				Enabled:         true,
			}, nil
		},
	}

	builtins := map[string]BuiltinTemplate{
		"auth.password_reset": {
			SubjectTemplate: "Builtin subject",
			HTMLTemplate:    "<p>Builtin: {{.ActionURL}}</p>",
			Variables:       []string{"AppName", "ActionURL"},
		},
	}

	svc := NewService(store, builtins)
	subject, html, text, err := svc.RenderWithFallback(ctx, "auth.password_reset",
		map[string]string{"AppName": "TestApp", "ActionURL": "https://example.com/reset"})
	testutil.NoError(t, err)
	testutil.Equal(t, "Builtin subject", subject)
	testutil.True(t, strings.Contains(html, "Builtin:"),
		"failed custom should fall back to builtin via RenderWithFallback")
	testutil.True(t, len(text) > 0, "plaintext should not be empty")
}

func TestServiceGetEffective_DisabledCustomFallsBackToBuiltin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// When a custom template exists but is disabled, GetEffective should
	// return the builtin as the effective template.
	store := &stubTemplateStore{
		getFn: func(_ context.Context, key string) (*Template, error) {
			if key == "auth.password_reset" {
				return &Template{
					TemplateKey:     key,
					SubjectTemplate: "Custom disabled subject",
					HTMLTemplate:    "<p>Custom disabled</p>",
					Enabled:         false,
				}, nil
			}
			return nil, ErrNotFound
		},
	}
	builtins := map[string]BuiltinTemplate{
		"auth.password_reset": {
			SubjectTemplate: "Reset your password",
			HTMLTemplate:    "<p>Default Reset</p>",
			Variables:       []string{"AppName", "ActionURL"},
		},
	}

	svc := NewService(store, builtins)
	eff, err := svc.GetEffective(ctx, "auth.password_reset")
	testutil.NoError(t, err)
	testutil.Equal(t, "builtin", eff.Source)
	testutil.Equal(t, true, eff.Enabled)
	testutil.Equal(t, "Reset your password", eff.SubjectTemplate)
	testutil.Equal(t, "<p>Default Reset</p>", eff.HTMLTemplate)
	testutil.Equal(t, 2, len(eff.Variables))
}

func TestServiceGetEffective_DisabledCustomNoBuiltinReturnsCustom(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// When a custom template is disabled and no builtin exists for the key,
	// GetEffective should return the disabled custom template.
	store := &stubTemplateStore{
		getFn: func(_ context.Context, key string) (*Template, error) {
			if key == "app.club_invite" {
				return &Template{
					TemplateKey:     key,
					SubjectTemplate: "Invite {{.Name}}",
					HTMLTemplate:    "<p>Join {{.ClubName}}</p>",
					Enabled:         false,
				}, nil
			}
			return nil, ErrNotFound
		},
	}

	svc := NewService(store, map[string]BuiltinTemplate{})
	eff, err := svc.GetEffective(ctx, "app.club_invite")
	testutil.NoError(t, err)
	testutil.Equal(t, "custom", eff.Source)
	testutil.Equal(t, false, eff.Enabled)
	testutil.Equal(t, "Invite {{.Name}}", eff.SubjectTemplate)
}
