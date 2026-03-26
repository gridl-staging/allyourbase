package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type fakeAssistantService struct {
	executeResp  ai.AssistantResponse
	executeErr   error
	streamResp   ai.AssistantResponse
	streamErr    error
	streamChunks []string
	historyRows  []ai.AssistantHistoryEntry
	historyTotal int
	historyErr   error
}

func (f *fakeAssistantService) Execute(_ context.Context, _ ai.AssistantRequest) (ai.AssistantResponse, error) {
	if f.executeErr != nil {
		return ai.AssistantResponse{}, f.executeErr
	}
	return f.executeResp, nil
}

func (f *fakeAssistantService) ExecuteStream(_ context.Context, _ ai.AssistantRequest, onChunk func(string) error) (ai.AssistantResponse, error) {
	for _, chunk := range f.streamChunks {
		if err := onChunk(chunk); err != nil {
			return ai.AssistantResponse{}, err
		}
	}
	if f.streamErr != nil {
		return ai.AssistantResponse{}, f.streamErr
	}
	return f.streamResp, nil
}

func (f *fakeAssistantService) ListHistory(_ context.Context, _ ai.AssistantHistoryFilter) ([]ai.AssistantHistoryEntry, int, error) {
	if f.historyErr != nil {
		return nil, 0, f.historyErr
	}
	return f.historyRows, f.historyTotal, nil
}

func assistantHandlerServer(svc assistantService) *Server {
	s := &Server{assistantSvc: svc}
	r := chi.NewRouter()
	r.Route("/admin/ai", func(r chi.Router) {
		r.Post("/assistant", s.handleAdminAIAssistant)
		r.Post("/assistant/stream", s.handleAdminAIAssistantStream)
		r.Get("/assistant/history", s.handleAdminAIAssistantHistory)
	})
	s.router = r
	return s
}

func assistantHandlerServerWithAdminRoutes(svc assistantService, rl *auth.RateLimiter, limit int) *Server {
	s := &Server{
		assistantSvc:       svc,
		assistantRL:        rl,
		assistantRateLimit: limit,
		adminAuth:          newAdminAuth("test-password"),
	}
	r := chi.NewRouter()
	s.registerAdminAIRoutes(r)
	s.router = r
	return s
}

func TestAdminAIAssistant_Success(t *testing.T) {
	svc := &fakeAssistantService{executeResp: ai.AssistantResponse{
		HistoryID:    uuid.New(),
		Mode:         ai.AssistantModeSQL,
		Text:         "```sql\nSELECT 1;\n```",
		SQL:          "SELECT 1;",
		Explanation:  "returns one row",
		Warning:      "",
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		DurationMs:   12,
		InputTokens:  5,
		OutputTokens: 6,
	}}
	s := assistantHandlerServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/admin/ai/assistant", strings.NewReader(`{"query":"show me sql","mode":"sql"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", w.Code, w.Body.String())
	}
	var resp ai.AssistantResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SQL != "SELECT 1;" {
		t.Fatalf("sql = %q", resp.SQL)
	}
}

func TestAdminAIAssistant_Validation(t *testing.T) {
	s := assistantHandlerServer(&fakeAssistantService{})
	cases := []struct {
		name string
		body string
	}{
		{name: "empty query", body: `{"query":""}`},
		{name: "too long", body: `{"query":"` + strings.Repeat("a", 2001) + `"}`},
		{name: "invalid mode", body: `{"query":"ok","mode":"bad"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/admin/ai/assistant", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.router.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d; want 400; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAdminAIAssistant_AllowsUnicodeAtCharacterLimit(t *testing.T) {
	s := assistantHandlerServer(&fakeAssistantService{})
	unicodeQuery := strings.Repeat("界", assistantMaxQueryLength)

	req := httptest.NewRequest(http.MethodPost, "/admin/ai/assistant", strings.NewReader(`{"query":"`+unicodeQuery+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestAdminAIAssistant_RateLimited(t *testing.T) {
	rl := auth.NewRateLimiter(1, time.Hour)
	defer rl.Stop()

	s := assistantHandlerServerWithAdminRoutes(&fakeAssistantService{}, rl, 1)
	token := s.adminAuth.token()

	newRequest := func(ip string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/admin/ai/assistant", strings.NewReader(`{"query":"show me sql"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = ip
		return req
	}

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, newRequest("198.51.100.10:1234"))
	if w.Code != http.StatusOK {
		t.Fatalf("first status = %d; want 200; body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, newRequest("198.51.100.10:9876"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d; want 429; body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatalf("Retry-After header must be set on rate limit responses")
	}

	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, newRequest("203.0.113.20:1111"))
	if w.Code != http.StatusOK {
		t.Fatalf("third status = %d; want 200 for distinct client; body=%s", w.Code, w.Body.String())
	}
}

func TestAdminAIAssistant_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{name: "disabled", err: ai.ErrAssistantDisabled, status: http.StatusNotFound},
		{name: "not configured", err: ai.ErrAssistantNotConfigured, status: http.StatusServiceUnavailable},
		{name: "schema not ready", err: ai.ErrAssistantSchemaCacheNotReady, status: http.StatusServiceUnavailable},
		{name: "other", err: errors.New("boom"), status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := assistantHandlerServer(&fakeAssistantService{executeErr: tc.err})
			req := httptest.NewRequest(http.MethodPost, "/admin/ai/assistant", strings.NewReader(`{"query":"q"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.router.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status = %d; want %d; body=%s", w.Code, tc.status, w.Body.String())
			}
		})
	}
}

func TestAdminAIAssistantStream_HappyPath(t *testing.T) {
	svc := &fakeAssistantService{
		streamChunks: []string{"hello", " world"},
		streamResp: ai.AssistantResponse{
			HistoryID:   uuid.New(),
			Mode:        ai.AssistantModeGeneral,
			Text:        "hello world",
			Provider:    "openai",
			Model:       "gpt-4o-mini",
			DurationMs:  15,
			CreatedAt:   time.Now().UTC(),
			FinishedAt:  time.Now().UTC(),
			Status:      ai.AssistantStatusSuccess,
			Explanation: "done",
		},
	}
	s := assistantHandlerServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/admin/ai/assistant/stream", strings.NewReader(`{"query":"say hi"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: start") {
		t.Fatalf("missing start event: %s", body)
	}
	if !strings.Contains(body, "event: chunk") {
		t.Fatalf("missing chunk event: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("missing done event: %s", body)
	}
}

func TestAdminAIAssistantStream_ProviderFailure(t *testing.T) {
	s := assistantHandlerServer(&fakeAssistantService{streamErr: errors.New("provider down")})
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/assistant/stream", strings.NewReader(`{"query":"q"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "event: error") {
		t.Fatalf("expected SSE error event, got: %s", w.Body.String())
	}
}

func TestAdminAIAssistantStream_ClientCancellationDoesNotEmitErrorEvent(t *testing.T) {
	s := assistantHandlerServer(&fakeAssistantService{streamChunks: []string{"chunk"}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/assistant/stream", strings.NewReader(`{"query":"q"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}

	body := w.Body.String()
	if strings.Contains(body, "event: error") {
		t.Fatalf("unexpected SSE error event for canceled client stream: %s", body)
	}
	if strings.Contains(body, "event: done") {
		t.Fatalf("unexpected SSE done event for canceled client stream: %s", body)
	}
}

func TestAdminAIAssistantStream_BackendTimeoutEmitsErrorEvent(t *testing.T) {
	s := assistantHandlerServer(&fakeAssistantService{streamErr: context.DeadlineExceeded})
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/assistant/stream", strings.NewReader(`{"query":"q"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("expected SSE error event for backend timeout, got: %s", body)
	}
	if strings.Contains(body, "event: done") {
		t.Fatalf("unexpected SSE done event for backend timeout: %s", body)
	}
}

func TestAdminAIAssistantHistory_Success(t *testing.T) {
	svc := &fakeAssistantService{
		historyRows: []ai.AssistantHistoryEntry{{
			ID:        uuid.New(),
			Mode:      ai.AssistantModeSQL,
			QueryText: "show users",
			CreatedAt: time.Now().UTC(),
		}},
		historyTotal: 1,
	}
	s := assistantHandlerServer(svc)

	req := httptest.NewRequest(http.MethodGet, "/admin/ai/assistant/history?mode=sql&page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var payload struct {
		History []ai.AssistantHistoryEntry `json:"history"`
		Total   int                        `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Total != 1 || len(payload.History) != 1 {
		t.Fatalf("unexpected history payload: %+v", payload)
	}
}
