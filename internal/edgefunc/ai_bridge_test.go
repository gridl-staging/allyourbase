package edgefunc

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// --- ayb.ai.generateText ---

func TestAIGenerateTextSuccess(t *testing.T) {
	called := false
	pool := NewPool(1, WithPoolAIGenerate(func(ctx context.Context, messages []map[string]any, opts map[string]any) (string, error) {
		called = true
		if len(messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(messages))
		}
		if messages[0]["role"] != "user" {
			t.Errorf("expected role=user, got %v", messages[0]["role"])
		}
		if messages[0]["content"] != "hello" {
			t.Errorf("expected content=hello, got %v", messages[0]["content"])
		}
		if opts["model"] != "gpt-4o" {
			t.Errorf("expected model=gpt-4o, got %v", opts["model"])
		}
		return "world", nil
	}))
	defer pool.Close()

	code := `
function handler(req) {
	var result = ayb.ai.generateText({
		messages: [{role: "user", content: "hello"}],
		model: "gpt-4o"
	});
	return { statusCode: 200, body: result };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !called {
		t.Error("expected AIGenerateFunc to be called")
	}
	if string(resp.Body) != "world" {
		t.Errorf("expected body=world, got %q", resp.Body)
	}
}

func TestAIGenerateTextError(t *testing.T) {
	pool := NewPool(1, WithPoolAIGenerate(func(_ context.Context, _ []map[string]any, _ map[string]any) (string, error) {
		return "", errors.New("provider unavailable")
	}))
	defer pool.Close()

	code := `
function handler(req) {
	try {
		ayb.ai.generateText({ messages: [{role: "user", content: "test"}] });
		return { statusCode: 200, body: "should not reach" };
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
	if !strings.Contains(string(resp.Body), "provider unavailable") {
		t.Errorf("expected error message in body, got %q", resp.Body)
	}
}

func TestAIGenerateTextNotConfigured(t *testing.T) {
	// Pool without AI function — ayb.ai should be absent
	pool := NewPool(1)
	defer pool.Close()

	code := `
function handler(req) {
	if (typeof ayb.ai === "undefined") {
		return { statusCode: 200, body: "no-ai" };
	}
	return { statusCode: 200, body: "has-ai" };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if string(resp.Body) != "no-ai" {
		t.Errorf("expected no-ai, got %q", resp.Body)
	}
}

func TestAIGenerateTextRequiresArg(t *testing.T) {
	pool := NewPool(1, WithPoolAIGenerate(func(_ context.Context, _ []map[string]any, _ map[string]any) (string, error) {
		return "ok", nil
	}))
	defer pool.Close()

	code := `
function handler(req) {
	try {
		ayb.ai.generateText();
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
}

// --- ayb.ai.renderPrompt ---

func TestAIRenderPromptSuccess(t *testing.T) {
	called := false
	pool := NewPool(1, WithPoolAIRenderPrompt(func(ctx context.Context, name string, vars map[string]any) (string, error) {
		called = true
		if name != "greeting" {
			t.Errorf("expected name=greeting, got %q", name)
		}
		if vars["user"] != "Alice" {
			t.Errorf("expected vars.user=Alice, got %v", vars["user"])
		}
		return "Hello Alice!", nil
	}))
	defer pool.Close()

	code := `
function handler(req) {
	var text = ayb.ai.renderPrompt("greeting", { user: "Alice" });
	return { statusCode: 200, body: text };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !called {
		t.Error("expected AIRenderPromptFunc to be called")
	}
	if string(resp.Body) != "Hello Alice!" {
		t.Errorf("expected 'Hello Alice!', got %q", resp.Body)
	}
}

func TestAIRenderPromptError(t *testing.T) {
	pool := NewPool(1, WithPoolAIRenderPrompt(func(_ context.Context, _ string, _ map[string]any) (string, error) {
		return "", errors.New("prompt not found")
	}))
	defer pool.Close()

	code := `
function handler(req) {
	try {
		ayb.ai.renderPrompt("missing-prompt", {});
		return { statusCode: 200, body: "ok" };
	} catch(e) {
		return { statusCode: 404, body: e.message };
	}
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestAIRenderPromptRequiresName(t *testing.T) {
	pool := NewPool(1, WithPoolAIRenderPrompt(func(_ context.Context, _ string, _ map[string]any) (string, error) {
		return "ok", nil
	}))
	defer pool.Close()

	code := `
function handler(req) {
	try {
		ayb.ai.renderPrompt();
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
}

// --- ayb.ai.parseDocument ---

func TestAIParseDocumentSuccess(t *testing.T) {
	called := false
	pool := NewPool(1, WithPoolAIParseDocument(func(ctx context.Context, url string, opts map[string]any) (map[string]any, error) {
		called = true
		if url != "https://example.com/invoice.pdf" {
			t.Errorf("expected url, got %q", url)
		}
		return map[string]any{"amount": 42.0, "vendor": "ACME"}, nil
	}))
	defer pool.Close()

	code := `
function handler(req) {
	var result = ayb.ai.parseDocument({ url: "https://example.com/invoice.pdf" });
	return { statusCode: 200, body: JSON.stringify(result) };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !called {
		t.Error("expected AIParseDocumentFunc to be called")
	}
	if !strings.Contains(string(resp.Body), "ACME") {
		t.Errorf("expected ACME in response, got %q", resp.Body)
	}
}

func TestAIParseDocumentError(t *testing.T) {
	pool := NewPool(1, WithPoolAIParseDocument(func(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return nil, errors.New("fetch failed")
	}))
	defer pool.Close()

	code := `
function handler(req) {
	try {
		ayb.ai.parseDocument({ url: "https://bad.example" });
		return { statusCode: 200, body: "ok" };
	} catch(e) {
		return { statusCode: 502, body: e.message };
	}
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.StatusCode != 502 {
		t.Errorf("expected status 502, got %d", resp.StatusCode)
	}
}

func TestAIParseDocumentRequiresArg(t *testing.T) {
	pool := NewPool(1, WithPoolAIParseDocument(func(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{}, nil
	}))
	defer pool.Close()

	code := `
function handler(req) {
	try {
		ayb.ai.parseDocument();
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
}

// --- coexistence with other bridges ---

func TestAIBridgeCoexistsWithDB(t *testing.T) {
	pool := NewPool(1, WithPoolAIGenerate(func(_ context.Context, _ []map[string]any, _ map[string]any) (string, error) {
		return "ai-result", nil
	}))
	defer pool.Close()

	// Both ayb.db and ayb.ai should be available simultaneously.
	// ayb.db.from is not configured (nil qe), so ayb.db won't exist.
	// We just verify that ayb.ai doesn't wipe other ayb properties.
	code := `
function handler(req) {
	var hasEnv = typeof ayb.env !== "undefined";
	var hasAI  = typeof ayb.ai  !== "undefined";
	return { statusCode: 200, body: JSON.stringify({ hasEnv: hasEnv, hasAI: hasAI }) };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(string(resp.Body), `"hasEnv":true`) {
		t.Errorf("ayb.env missing: %q", resp.Body)
	}
	if !strings.Contains(string(resp.Body), `"hasAI":true`) {
		t.Errorf("ayb.ai missing: %q", resp.Body)
	}
}

func TestSetAIGenerateOnExistingPool(t *testing.T) {
	pool := NewPool(1)
	defer pool.Close()

	pool.SetAIGenerate(func(_ context.Context, _ []map[string]any, _ map[string]any) (string, error) {
		return "set-after", nil
	})

	code := `
function handler(req) {
	var text = ayb.ai.generateText({ messages: [{role: "user", content: "hi"}] });
	return { statusCode: 200, body: text };
}
`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if string(resp.Body) != "set-after" {
		t.Errorf("expected set-after, got %q", resp.Body)
	}
}
