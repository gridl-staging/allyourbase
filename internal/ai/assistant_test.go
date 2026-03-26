package ai

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/google/uuid"
)

type assistantTestProvider struct {
	resp GenerateTextResponse
	err  error
	req  GenerateTextRequest
}

func (p *assistantTestProvider) GenerateText(_ context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
	p.req = req
	if p.err != nil {
		return GenerateTextResponse{}, p.err
	}
	return p.resp, nil
}

type assistantTestStreamingProvider struct {
	assistantTestProvider
	streamText string
	streamErr  error
}

func (p *assistantTestStreamingProvider) GenerateTextStream(_ context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	p.req = req
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	return io.NopCloser(strings.NewReader(p.streamText)), nil
}

type assistantTestHistoryStore struct {
	entries       []AssistantHistoryEntry
	insertCtxErrs []error
	list          []AssistantHistoryEntry
}

func (s *assistantTestHistoryStore) Insert(ctx context.Context, entry AssistantHistoryEntry) (AssistantHistoryEntry, error) {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	s.insertCtxErrs = append(s.insertCtxErrs, ctx.Err())
	s.entries = append(s.entries, entry)
	return entry, nil
}

func (s *assistantTestHistoryStore) List(_ context.Context, _ AssistantHistoryFilter) ([]AssistantHistoryEntry, int, error) {
	return s.list, len(s.list), nil
}

func testAssistantSchemaHolder() *schema.CacheHolder {
	h := schema.NewCacheHolder(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetForTesting(&schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				Kind:       "table",
				PrimaryKey: []string{"id"},
				Columns: []*schema.Column{
					{Name: "id", TypeName: "uuid", IsPrimaryKey: true},
					{Name: "email", TypeName: "text"},
				},
				Indexes:    []*schema.Index{{Name: "idx_users_email", Method: "btree", IsUnique: true}},
				RLSEnabled: true,
				RLSPolicies: []*schema.RLSPolicy{{
					Name:    "users_select_self",
					Command: "SELECT",
				}},
			},
		},
		Functions: map[string]*schema.Function{
			"public.search_users": {
				Schema:     "public",
				Name:       "search_users",
				ReturnType: "SETOF users",
			},
		},
		Schemas:     []string{"public"},
		HasPgVector: true,
		HasPostGIS:  true,
		BuiltAt:     time.Now().UTC(),
	})
	return h
}

func TestPromptForMode(t *testing.T) {
	modes := []AssistantMode{AssistantModeSQL, AssistantModeRLS, AssistantModeMigration, AssistantModeGeneral}
	for _, mode := range modes {
		tpl := PromptForMode(mode)
		if tpl.Mode != mode {
			t.Fatalf("mode = %q; want %q", tpl.Mode, mode)
		}
		if !strings.Contains(tpl.SystemPrompt, "DROP DATABASE") {
			t.Fatalf("prompt for %s missing safety rule", mode)
		}
	}
}

func TestPromptForModeUnknownFallsBackGeneral(t *testing.T) {
	tpl := PromptForMode("unknown")
	if tpl.Mode != AssistantModeGeneral {
		t.Fatalf("mode = %q; want %q", tpl.Mode, AssistantModeGeneral)
	}
}

func TestCompactSchemaContextIncludesCoreSignalsAndBudget(t *testing.T) {
	summary := CompactSchemaContext(testAssistantSchemaHolder().Get(), 300)
	if !strings.Contains(summary, "public.users") {
		t.Fatalf("summary missing table name: %q", summary)
	}
	if !strings.Contains(summary, "RLS") {
		t.Fatalf("summary missing RLS: %q", summary)
	}
	if !strings.Contains(summary, "pgvector") {
		t.Fatalf("summary missing pgvector capability: %q", summary)
	}
	if len(summary) > 300 {
		t.Fatalf("summary length = %d; want <= 300", len(summary))
	}
}

func TestParseAssistantResponseTextExtractsSQLAndWarning(t *testing.T) {
	parsed := ParseAssistantResponseText("" +
		"Use this query:\n```sql\nDELETE FROM users;\n```\nThis is destructive.")
	if !strings.Contains(parsed.SQL, "DELETE FROM users") {
		t.Fatalf("sql extraction failed: %q", parsed.SQL)
	}
	if parsed.Warning == "" {
		t.Fatalf("warning must be populated for unqualified delete")
	}
}

func TestAssistantServiceExecuteSuccessWritesHistory(t *testing.T) {
	reg := NewRegistry()
	provider := &assistantTestProvider{resp: GenerateTextResponse{
		Text:  "```sql\nSELECT id FROM users WHERE email = $1;\n```",
		Model: "gpt-4o-mini",
		Usage: Usage{InputTokens: 11, OutputTokens: 7},
	}}
	reg.Register("openai", provider)
	history := &assistantTestHistoryStore{}
	svc := NewAssistantService(AssistantServiceConfig{
		Enabled:      true,
		AIConfig:     config.AIConfig{DefaultProvider: "openai", DefaultModel: "gpt-4o", Providers: map[string]config.ProviderConfig{"openai": {DefaultModel: "gpt-4o-mini"}}},
		Registry:     reg,
		Schema:       testAssistantSchemaHolder(),
		HistoryStore: history,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	resp, err := svc.Execute(context.Background(), AssistantRequest{Mode: AssistantModeSQL, Query: "find user by email"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(resp.SQL, "SELECT id FROM users") {
		t.Fatalf("unexpected SQL: %q", resp.SQL)
	}
	if resp.Provider != "openai" {
		t.Fatalf("provider = %q; want openai", resp.Provider)
	}
	if resp.HistoryID == uuid.Nil {
		t.Fatalf("HistoryID should be set")
	}
	if len(history.entries) != 1 {
		t.Fatalf("history entries = %d; want 1", len(history.entries))
	}
	if history.entries[0].Status != AssistantStatusSuccess {
		t.Fatalf("status = %q; want %q", history.entries[0].Status, AssistantStatusSuccess)
	}
}

func TestAssistantServiceExecuteTypedErrors(t *testing.T) {
	baseCfg := config.AIConfig{DefaultProvider: "openai", DefaultModel: "gpt-4o", Providers: map[string]config.ProviderConfig{"openai": {DefaultModel: "gpt-4o-mini"}}}
	reg := NewRegistry()
	reg.Register("openai", &assistantTestProvider{resp: GenerateTextResponse{Text: "ok", Model: "gpt-4o-mini"}})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("disabled", func(t *testing.T) {
		svc := NewAssistantService(AssistantServiceConfig{Enabled: false, AIConfig: baseCfg, Registry: reg, Schema: testAssistantSchemaHolder(), Logger: logger})
		_, err := svc.Execute(context.Background(), AssistantRequest{Query: "q"})
		if !errors.Is(err, ErrAssistantDisabled) {
			t.Fatalf("err = %v; want ErrAssistantDisabled", err)
		}
	})

	t.Run("not configured", func(t *testing.T) {
		svc := NewAssistantService(AssistantServiceConfig{Enabled: true, AIConfig: config.AIConfig{}, Registry: NewRegistry(), Schema: testAssistantSchemaHolder(), Logger: logger})
		_, err := svc.Execute(context.Background(), AssistantRequest{Query: "q"})
		if !errors.Is(err, ErrAssistantNotConfigured) {
			t.Fatalf("err = %v; want ErrAssistantNotConfigured", err)
		}
	})

	t.Run("schema not ready", func(t *testing.T) {
		svc := NewAssistantService(AssistantServiceConfig{Enabled: true, AIConfig: baseCfg, Registry: reg, Schema: schema.NewCacheHolder(nil, logger), Logger: logger})
		_, err := svc.Execute(context.Background(), AssistantRequest{Query: "q"})
		if !errors.Is(err, ErrAssistantSchemaCacheNotReady) {
			t.Fatalf("err = %v; want ErrAssistantSchemaCacheNotReady", err)
		}
	})
}

func TestAssistantServiceExecuteStreamFallback(t *testing.T) {
	reg := NewRegistry()
	reg.Register("openai", &assistantTestProvider{resp: GenerateTextResponse{Text: "hello world", Model: "gpt-4o-mini"}})
	history := &assistantTestHistoryStore{}
	svc := NewAssistantService(AssistantServiceConfig{
		Enabled:      true,
		AIConfig:     config.AIConfig{DefaultProvider: "openai", DefaultModel: "gpt-4o", Providers: map[string]config.ProviderConfig{"openai": {DefaultModel: "gpt-4o-mini"}}},
		Registry:     reg,
		Schema:       testAssistantSchemaHolder(),
		HistoryStore: history,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	var chunks []string
	resp, err := svc.ExecuteStream(context.Background(), AssistantRequest{Mode: AssistantModeGeneral, Query: "say hi"}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteStream: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected fallback to emit at least one chunk")
	}
	if resp.Text == "" {
		t.Fatalf("stream response text should be populated")
	}
	if history.entries[0].Status != AssistantStatusSuccess {
		t.Fatalf("status = %q; want %q", history.entries[0].Status, AssistantStatusSuccess)
	}
}

func TestAssistantServiceExecuteStreamProviderErrorWritesHistory(t *testing.T) {
	reg := NewRegistry()
	reg.Register("openai", &assistantTestStreamingProvider{streamErr: errors.New("stream failed")})
	history := &assistantTestHistoryStore{}
	svc := NewAssistantService(AssistantServiceConfig{
		Enabled:      true,
		AIConfig:     config.AIConfig{DefaultProvider: "openai", DefaultModel: "gpt-4o", Providers: map[string]config.ProviderConfig{"openai": {DefaultModel: "gpt-4o-mini"}}},
		Registry:     reg,
		Schema:       testAssistantSchemaHolder(),
		HistoryStore: history,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	_, err := svc.ExecuteStream(context.Background(), AssistantRequest{Mode: AssistantModeGeneral, Query: "say hi"}, func(chunk string) error {
		return nil
	})
	if err == nil {
		t.Fatalf("expected stream error")
	}
	if len(history.entries) != 1 {
		t.Fatalf("history entries = %d; want 1", len(history.entries))
	}
	if history.entries[0].Status != AssistantStatusError {
		t.Fatalf("status = %q; want %q", history.entries[0].Status, AssistantStatusError)
	}
}

func TestAssistantServiceExecuteStreamCancelledWritesHistoryWithDetachedContext(t *testing.T) {
	reg := NewRegistry()
	reg.Register("openai", &assistantTestStreamingProvider{streamText: "hello"})
	history := &assistantTestHistoryStore{}
	svc := NewAssistantService(AssistantServiceConfig{
		Enabled:      true,
		AIConfig:     config.AIConfig{DefaultProvider: "openai", DefaultModel: "gpt-4o", Providers: map[string]config.ProviderConfig{"openai": {DefaultModel: "gpt-4o-mini"}}},
		Registry:     reg,
		Schema:       testAssistantSchemaHolder(),
		HistoryStore: history,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	_, err := svc.ExecuteStream(ctx, AssistantRequest{Mode: AssistantModeGeneral, Query: "say hi"}, func(string) error {
		cancel()
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want context.Canceled", err)
	}
	if len(history.entries) != 1 {
		t.Fatalf("history entries = %d; want 1", len(history.entries))
	}
	if history.entries[0].Status != AssistantStatusCancelled {
		t.Fatalf("status = %q; want %q", history.entries[0].Status, AssistantStatusCancelled)
	}
	if len(history.insertCtxErrs) != 1 {
		t.Fatalf("insert ctx count = %d; want 1", len(history.insertCtxErrs))
	}
	if history.insertCtxErrs[0] != nil {
		t.Fatalf("history insert context err = %v; want nil detached context", history.insertCtxErrs[0])
	}
}
