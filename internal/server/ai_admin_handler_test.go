package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// --- Fake stores ---

type fakeAILogStore struct {
	logs    []ai.CallLog
	listErr error
}

func (f *fakeAILogStore) List(_ context.Context, filter ai.ListFilter) ([]ai.CallLog, int, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return f.logs, len(f.logs), nil
}

func (f *fakeAILogStore) UsageSummary(_ context.Context, from, to time.Time) (ai.UsageSummary, error) {
	return ai.UsageSummary{
		TotalCalls:        len(f.logs),
		TotalInputTokens:  100,
		TotalOutputTokens: 50,
		ByProvider:        map[string]ai.ProviderUsage{"openai": {Calls: len(f.logs)}},
	}, nil
}

func (f *fakeAILogStore) DailyUsage(_ context.Context, from, to time.Time) ([]ai.DailyUsage, error) {
	return []ai.DailyUsage{
		{
			Day:          time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
			Provider:     "openai",
			Model:        "gpt-4o",
			Calls:        2,
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
			TotalCostUSD: 0.01,
		},
	}, nil
}

type fakePromptStore struct {
	prompts map[uuid.UUID]ai.Prompt
	byName  map[string]uuid.UUID
}

func newFakePromptStore() *fakePromptStore {
	return &fakePromptStore{
		prompts: make(map[uuid.UUID]ai.Prompt),
		byName:  make(map[string]uuid.UUID),
	}
}

func (f *fakePromptStore) Create(_ context.Context, req ai.CreatePromptRequest) (ai.Prompt, error) {
	id := uuid.New()
	p := ai.Prompt{
		ID:       id,
		Name:     req.Name,
		Version:  1,
		Template: req.Template,
	}
	f.prompts[id] = p
	f.byName[req.Name] = id
	return p, nil
}

func (f *fakePromptStore) Get(_ context.Context, id uuid.UUID) (ai.Prompt, error) {
	p, ok := f.prompts[id]
	if !ok {
		return ai.Prompt{}, errNotFound
	}
	return p, nil
}

func (f *fakePromptStore) List(_ context.Context, page, perPage int) ([]ai.Prompt, int, error) {
	var out []ai.Prompt
	for _, p := range f.prompts {
		out = append(out, p)
	}
	return out, len(out), nil
}

func (f *fakePromptStore) Update(_ context.Context, id uuid.UUID, req ai.UpdatePromptRequest) (ai.Prompt, error) {
	p, ok := f.prompts[id]
	if !ok {
		return ai.Prompt{}, errNotFound
	}
	if req.Template != nil {
		p.Template = *req.Template
	}
	f.prompts[id] = p
	return p, nil
}

func (f *fakePromptStore) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := f.prompts[id]; !ok {
		return errNotFound
	}
	delete(f.prompts, id)
	return nil
}

func (f *fakePromptStore) ListVersions(_ context.Context, promptID uuid.UUID) ([]ai.PromptVersion, error) {
	return []ai.PromptVersion{}, nil
}

// --- Helpers ---

func aiTestServer(logStore aiLogStore, ps promptStore) *Server {
	s := &Server{
		aiLogStore:  logStore,
		promptStore: ps,
	}
	r := chi.NewRouter()
	r.Route("/admin/ai", func(r chi.Router) {
		r.Get("/logs", s.handleAdminAILogs)
		r.Get("/usage", s.handleAdminAIUsage)
		r.Get("/usage/daily", s.handleAdminAIUsageDaily)
		r.Route("/prompts", func(r chi.Router) {
			r.Get("/", s.handleAdminPromptList)
			r.Post("/", s.handleAdminPromptCreate)
			r.Get("/{id}", s.handleAdminPromptGet)
			r.Put("/{id}", s.handleAdminPromptUpdate)
			r.Delete("/{id}", s.handleAdminPromptDelete)
			r.Get("/{id}/versions", s.handleAdminPromptVersions)
			r.Post("/{id}/render", s.handleAdminPromptRender)
		})
	})
	s.router = r
	return s
}

// --- AI Log Tests ---

func TestAdminAILogsEmpty(t *testing.T) {
	s := aiTestServer(&fakeAILogStore{}, nil)
	req := httptest.NewRequest("GET", "/admin/ai/logs", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"] != float64(0) {
		t.Errorf("total = %v; want 0", resp["total"])
	}
}

func TestAdminAILogsNilStore(t *testing.T) {
	s := aiTestServer(nil, nil)
	req := httptest.NewRequest("GET", "/admin/ai/logs", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
}

func TestAdminAIUsage(t *testing.T) {
	store := &fakeAILogStore{logs: []ai.CallLog{{Provider: "openai"}}}
	s := aiTestServer(store, nil)
	req := httptest.NewRequest("GET", "/admin/ai/usage", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp ai.UsageSummary
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d; want 1", resp.TotalCalls)
	}
}

func TestAdminAIUsageDaily(t *testing.T) {
	store := &fakeAILogStore{logs: []ai.CallLog{{Provider: "openai"}}}
	s := aiTestServer(store, nil)
	req := httptest.NewRequest("GET", "/admin/ai/usage/daily", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp []ai.DailyUsage
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 1 {
		t.Fatalf("rows = %d; want 1", len(resp))
	}
	if resp[0].Provider != "openai" {
		t.Fatalf("provider = %q; want openai", resp[0].Provider)
	}
}

// --- Prompt CRUD Tests ---

func TestAdminPromptCreateAndGet(t *testing.T) {
	ps := newFakePromptStore()
	s := aiTestServer(nil, ps)

	// Create
	body := `{"name":"greeting","template":"Hello {{name}}"}`
	req := httptest.NewRequest("POST", "/admin/ai/prompts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d; want 201; body = %s", w.Code, w.Body.String())
	}
	var created ai.Prompt
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.Name != "greeting" {
		t.Errorf("name = %q", created.Name)
	}

	// Get
	req = httptest.NewRequest("GET", "/admin/ai/prompts/"+created.ID.String(), nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d; want 200", w.Code)
	}
}

func TestAdminPromptCreateMissingFields(t *testing.T) {
	ps := newFakePromptStore()
	s := aiTestServer(nil, ps)

	body := `{"name":"","template":""}`
	req := httptest.NewRequest("POST", "/admin/ai/prompts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
}

func TestAdminPromptList(t *testing.T) {
	ps := newFakePromptStore()
	s := aiTestServer(nil, ps)

	// Seed a prompt
	ps.Create(context.Background(), ai.CreatePromptRequest{Name: "test", Template: "hi"})

	req := httptest.NewRequest("GET", "/admin/ai/prompts", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"] != float64(1) {
		t.Errorf("total = %v; want 1", resp["total"])
	}
}

func TestAdminPromptUpdate(t *testing.T) {
	ps := newFakePromptStore()
	s := aiTestServer(nil, ps)

	created, _ := ps.Create(context.Background(), ai.CreatePromptRequest{Name: "test", Template: "old"})

	body := `{"template":"new"}`
	req := httptest.NewRequest("PUT", "/admin/ai/prompts/"+created.ID.String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
	var updated ai.Prompt
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Template != "new" {
		t.Errorf("template = %q; want 'new'", updated.Template)
	}
}

func TestAdminPromptDelete(t *testing.T) {
	ps := newFakePromptStore()
	s := aiTestServer(nil, ps)

	created, _ := ps.Create(context.Background(), ai.CreatePromptRequest{Name: "test", Template: "hi"})

	req := httptest.NewRequest("DELETE", "/admin/ai/prompts/"+created.ID.String(), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want 204", w.Code)
	}
}

func TestAdminPromptDeleteNotFound(t *testing.T) {
	ps := newFakePromptStore()
	s := aiTestServer(nil, ps)

	req := httptest.NewRequest("DELETE", "/admin/ai/prompts/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", w.Code)
	}
}

func TestAdminPromptGetInvalidID(t *testing.T) {
	s := aiTestServer(nil, newFakePromptStore())

	req := httptest.NewRequest("GET", "/admin/ai/prompts/not-a-uuid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid prompt id format") {
		t.Fatalf("body = %s; want invalid prompt id format", w.Body.String())
	}
}

func TestAdminPromptRender(t *testing.T) {
	ps := newFakePromptStore()
	s := aiTestServer(nil, ps)

	created, _ := ps.Create(context.Background(), ai.CreatePromptRequest{
		Name:     "greeting",
		Template: "Hello {{name}}!",
	})

	body := `{"variables":{"name":"World"}}`
	req := httptest.NewRequest("POST", "/admin/ai/prompts/"+created.ID.String()+"/render", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["rendered"] != "Hello World!" {
		t.Errorf("rendered = %v", resp["rendered"])
	}
}

func TestAdminPromptVersions(t *testing.T) {
	ps := newFakePromptStore()
	s := aiTestServer(nil, ps)

	created, _ := ps.Create(context.Background(), ai.CreatePromptRequest{Name: "test", Template: "hi"})

	req := httptest.NewRequest("GET", "/admin/ai/prompts/"+created.ID.String()+"/versions", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
}

func TestAdminPromptNilStore(t *testing.T) {
	s := aiTestServer(nil, nil) // both nil

	// List should return empty
	req := httptest.NewRequest("GET", "/admin/ai/prompts", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d; want 200", w.Code)
	}

	// Create should return 503
	body := `{"name":"test","template":"hi"}`
	req = httptest.NewRequest("POST", "/admin/ai/prompts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("create status = %d; want 503", w.Code)
	}
}
