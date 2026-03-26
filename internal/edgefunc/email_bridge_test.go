package edgefunc

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEmailBridge_NilCallback(t *testing.T) {
	// Pool without email function — ayb.email should be absent.
	pool := NewPool(1)
	defer pool.Close()

	code := `
function handler(req) {
	if (typeof ayb.email === "undefined") {
		return { statusCode: 200, body: "no-email" };
	}
	return { statusCode: 200, body: "has-email" };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if string(resp.Body) != "no-email" {
		t.Errorf("expected no-email, got %q", resp.Body)
	}
}

func TestEmailBridge_DirectSend(t *testing.T) {
	var gotTo []string
	var gotSubject, gotHTML, gotText, gotFrom string

	pool := NewPool(1, WithPoolEmailSend(func(_ context.Context, to []string, subject, html, text, templateKey string, variables map[string]string, from string) (int, error) {
		gotTo = to
		gotSubject = subject
		gotHTML = html
		gotText = text
		gotFrom = from
		return len(to), nil
	}))
	defer pool.Close()

	code := `
function handler(req) {
	var result = ayb.email.send({
		to: "user@example.com",
		subject: "Hello",
		html: "<p>World</p>",
		text: "World",
		from: "noreply@example.com"
	});
	return { statusCode: 200, body: JSON.stringify(result) };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if len(gotTo) != 1 || gotTo[0] != "user@example.com" {
		t.Errorf("expected to=[user@example.com], got %v", gotTo)
	}
	if gotSubject != "Hello" {
		t.Errorf("expected subject=Hello, got %q", gotSubject)
	}
	if gotHTML != "<p>World</p>" {
		t.Errorf("expected html=<p>World</p>, got %q", gotHTML)
	}
	if gotText != "World" {
		t.Errorf("expected text=World, got %q", gotText)
	}
	if gotFrom != "noreply@example.com" {
		t.Errorf("expected from=noreply@example.com, got %q", gotFrom)
	}
	if !strings.Contains(string(resp.Body), `"sent":1`) {
		t.Errorf("expected {sent:1} in body, got %q", resp.Body)
	}
}

func TestEmailBridge_TemplateSend(t *testing.T) {
	var gotTemplateKey string
	var gotVars map[string]string

	pool := NewPool(1, WithPoolEmailSend(func(_ context.Context, to []string, subject, html, text, templateKey string, variables map[string]string, from string) (int, error) {
		gotTemplateKey = templateKey
		gotVars = variables
		return len(to), nil
	}))
	defer pool.Close()

	code := `
function handler(req) {
	var result = ayb.email.send({
		to: "user@example.com",
		templateKey: "welcome.email",
		variables: { name: "Alice", plan: "pro" }
	});
	return { statusCode: 200, body: JSON.stringify(result) };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if gotTemplateKey != "welcome.email" {
		t.Errorf("expected templateKey=welcome.email, got %q", gotTemplateKey)
	}
	if gotVars["name"] != "Alice" || gotVars["plan"] != "pro" {
		t.Errorf("expected vars={name:Alice, plan:pro}, got %v", gotVars)
	}
}

func TestEmailBridge_ArrayTo(t *testing.T) {
	var gotTo []string

	pool := NewPool(1, WithPoolEmailSend(func(_ context.Context, to []string, subject, html, text, templateKey string, variables map[string]string, from string) (int, error) {
		gotTo = to
		return len(to), nil
	}))
	defer pool.Close()

	code := `
function handler(req) {
	var result = ayb.email.send({
		to: ["a@b.com", "c@d.com", "e@f.com"],
		subject: "Hi",
		html: "<p>Hi</p>"
	});
	return { statusCode: 200, body: JSON.stringify(result) };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(gotTo) != 3 {
		t.Errorf("expected 3 recipients, got %d", len(gotTo))
	}
	if !strings.Contains(string(resp.Body), `"sent":3`) {
		t.Errorf("expected {sent:3}, got %q", resp.Body)
	}
}

func TestEmailBridge_MissingTo(t *testing.T) {
	pool := NewPool(1, WithPoolEmailSend(func(_ context.Context, to []string, subject, html, text, templateKey string, variables map[string]string, from string) (int, error) {
		return 0, nil
	}))
	defer pool.Close()

	code := `
function handler(req) {
	try {
		ayb.email.send({ subject: "Hi", html: "<p>Hi</p>" });
		return { statusCode: 200, body: "ok" };
	} catch(e) {
		return { statusCode: 400, body: e.message };
	}
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "'to' is required") {
		t.Errorf("expected 'to' required error, got %q", resp.Body)
	}
}

func TestEmailBridge_SendError(t *testing.T) {
	pool := NewPool(1, WithPoolEmailSend(func(_ context.Context, to []string, subject, html, text, templateKey string, variables map[string]string, from string) (int, error) {
		return 0, errors.New("SMTP connection refused")
	}))
	defer pool.Close()

	code := `
function handler(req) {
	try {
		ayb.email.send({ to: "a@b.com", subject: "Hi", html: "<p>Hi</p>" });
		return { statusCode: 200, body: "ok" };
	} catch(e) {
		return { statusCode: 500, body: e.message };
	}
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "SMTP connection refused") {
		t.Errorf("expected error message in body, got %q", resp.Body)
	}
}

func TestSetEmailSendOnExistingPool(t *testing.T) {
	pool := NewPool(1)
	defer pool.Close()

	pool.SetEmailSend(func(_ context.Context, to []string, subject, html, text, templateKey string, variables map[string]string, from string) (int, error) {
		return 42, nil
	})

	code := `
function handler(req) {
	var result = ayb.email.send({ to: "a@b.com", subject: "Hi", html: "<p>Hi</p>" });
	return { statusCode: 200, body: JSON.stringify(result) };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(string(resp.Body), `"sent":42`) {
		t.Errorf("expected {sent:42}, got %q", resp.Body)
	}
}
