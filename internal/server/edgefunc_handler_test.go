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

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// --- Fake edgeFuncInvoker ---

type fakeEdgeFuncInvoker struct {
	functions map[string]*edgefunc.EdgeFunction
	invokeErr error
	response  edgefunc.Response
	lastReq   *edgefunc.Request // captures the last request passed to Invoke
	lastCtx   context.Context   // captures the last context passed to Invoke
	invoked   bool              // whether Invoke was called
}

func newFakeEdgeFuncInvoker() *fakeEdgeFuncInvoker {
	return &fakeEdgeFuncInvoker{
		functions: make(map[string]*edgefunc.EdgeFunction),
	}
}

func (f *fakeEdgeFuncInvoker) GetByName(_ context.Context, name string) (*edgefunc.EdgeFunction, error) {
	fn, ok := f.functions[name]
	if !ok {
		return nil, edgefunc.ErrFunctionNotFound
	}
	return fn, nil
}

func (f *fakeEdgeFuncInvoker) Invoke(ctx context.Context, name string, req edgefunc.Request) (edgefunc.Response, error) {
	f.lastReq = &req
	f.lastCtx = ctx
	f.invoked = true
	if _, ok := f.functions[name]; !ok {
		return edgefunc.Response{}, edgefunc.ErrFunctionNotFound
	}
	if f.invokeErr != nil {
		return edgefunc.Response{}, f.invokeErr
	}
	return f.response, nil
}

func addFakeFunction(f *fakeEdgeFuncInvoker, name string, public bool) {
	f.functions[name] = &edgefunc.EdgeFunction{
		ID:         uuid.New(),
		Name:       name,
		EntryPoint: "handler",
		Source:     "export default function handler(req) { return new Response('ok'); }",
		Public:     public,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

// --- Test helpers ---

func edgefuncRouter(invoker edgeFuncInvoker) *chi.Mux {
	return edgefuncRouterWithValidator(invoker, nil)
}

func edgefuncRouterWithValidator(invoker edgeFuncInvoker, validator edgeFuncTokenValidator) *chi.Mux {
	r := chi.NewRouter()
	r.HandleFunc("/functions/v1/{name}", handleEdgeFuncInvoke(invoker, MaxEdgeFuncBodySize, validator, nil))
	r.HandleFunc("/functions/v1/{name}/*", handleEdgeFuncInvoke(invoker, MaxEdgeFuncBodySize, validator, nil))
	return r
}

func decodeErrorBody(t *testing.T, w *httptest.ResponseRecorder) httputil.ErrorResponse {
	t.Helper()
	var body httputil.ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&body)
	testutil.NoError(t, err)
	return body
}

// --- Tests ---

func TestEdgeFuncInvoke_GET_Public(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "hello", true)
	invoker.response = edgefunc.Response{
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"application/json"}},
		Body:       []byte(`{"message":"hello"}`),
	}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/hello", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 200, w.Code)
	testutil.Equal(t, "application/json", w.Header().Get("Content-Type"))
	testutil.Equal(t, `{"message":"hello"}`, w.Body.String())
}

func TestEdgeFuncInvoke_POST_WithBody(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "echo", true)
	invoker.response = edgefunc.Response{
		StatusCode: 201,
		Body:       []byte(`created`),
	}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("POST", "/functions/v1/echo", strings.NewReader(`{"data":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 201, w.Code)
	testutil.Equal(t, "created", w.Body.String())

	// Verify request translation: method, body, and headers were forwarded.
	testutil.True(t, invoker.lastReq != nil, "expected Invoke to be called")
	testutil.Equal(t, "POST", invoker.lastReq.Method)
	testutil.Equal(t, `{"data":"test"}`, string(invoker.lastReq.Body))
	testutil.Equal(t, "application/json", http.Header(invoker.lastReq.Headers).Get("Content-Type"))
}

func TestEdgeFuncInvoke_AllHTTPMethods(t *testing.T) {
	t.Parallel()
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			invoker := newFakeEdgeFuncInvoker()
			addFakeFunction(invoker, "any", true)
			invoker.response = edgefunc.Response{StatusCode: 200, Body: []byte("ok")}

			r := edgefuncRouter(invoker)
			req := httptest.NewRequest(method, "/functions/v1/any", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			testutil.StatusCode(t, 200, w.Code)
			// Verify the HTTP method was actually forwarded to the edge function.
			testutil.True(t, invoker.lastReq != nil, "expected Invoke to be called")
			testutil.Equal(t, method, invoker.lastReq.Method)
		})
	}
}

func TestEdgeFuncInvoke_NotFound(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 404, w.Code)
	body := decodeErrorBody(t, w)
	testutil.Equal(t, "function not found", body.Message)
}

func TestHandleEdgeFuncInvoke_InvocationRecorderCapturesSuccessStatus(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "recorded", true)
	invoker.response = edgefunc.Response{StatusCode: 200, Body: []byte("ok")}

	var calls []struct {
		name   string
		status string
	}
	recordInvocation := func(_ context.Context, name, status string) {
		calls = append(calls, struct {
			name   string
			status string
		}{name: name, status: status})
	}

	r := chi.NewRouter()
	r.HandleFunc("/functions/v1/{name}", handleEdgeFuncInvoke(invoker, MaxEdgeFuncBodySize, nil, recordInvocation))

	req := httptest.NewRequest("GET", "/functions/v1/recorded", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 200, w.Code)
	testutil.Equal(t, 1, len(calls))
	testutil.Equal(t, "recorded", calls[0].name)
	testutil.Equal(t, "ok", calls[0].status)
}

func TestHandleEdgeFuncInvoke_InvocationRecorderCapturesOkEvenWhenResponseWriteFails(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "badfunc", true)
	// Status code 99 is invalid (< 100), so writeEdgeFuncResponse returns an error.
	// But the function itself was invoked successfully — recorder should still get "ok".
	invoker.response = edgefunc.Response{StatusCode: 99, Body: []byte("bad")}

	var calls []struct {
		name   string
		status string
	}
	recordInvocation := func(_ context.Context, name, status string) {
		calls = append(calls, struct {
			name   string
			status string
		}{name: name, status: status})
	}

	r := chi.NewRouter()
	r.HandleFunc("/functions/v1/{name}", handleEdgeFuncInvoke(invoker, MaxEdgeFuncBodySize, nil, recordInvocation))

	req := httptest.NewRequest("GET", "/functions/v1/badfunc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 500, w.Code)
	testutil.Equal(t, 1, len(calls))
	testutil.Equal(t, "badfunc", calls[0].name)
	testutil.Equal(t, "ok", calls[0].status)
}

func TestHandleEdgeFuncInvoke_InvocationRecorderCapturesErrorStatus(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "recorded", true)
	invoker.invokeErr = errors.New("function crashed")

	var calls []struct {
		name   string
		status string
	}
	recordInvocation := func(_ context.Context, name, status string) {
		calls = append(calls, struct {
			name   string
			status string
		}{name: name, status: status})
	}

	r := chi.NewRouter()
	r.HandleFunc("/functions/v1/{name}", handleEdgeFuncInvoke(invoker, MaxEdgeFuncBodySize, nil, recordInvocation))

	req := httptest.NewRequest("GET", "/functions/v1/recorded", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 500, w.Code)
	testutil.Equal(t, 1, len(calls))
	testutil.Equal(t, "recorded", calls[0].name)
	testutil.Equal(t, "error", calls[0].status)
}

func TestEdgeFuncInvoke_RuntimeError(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "crash", true)
	invoker.invokeErr = errors.New("ReferenceError: x is not defined")

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/crash", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 500, w.Code)
	body := decodeErrorBody(t, w)
	testutil.Equal(t, "function execution failed", body.Message)
}

func TestEdgeFuncInvoke_ConcurrencyLimitExceeded(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "crash", true)
	invoker.invokeErr = edgefunc.ErrConcurrencyLimitExceeded

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/crash", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusTooManyRequests, w.Code)
	testutil.Equal(t, "1", w.Header().Get("Retry-After"))
	body := decodeErrorBody(t, w)
	testutil.Equal(t, "function concurrency limit exceeded", body.Message)
}

func TestEdgeFuncInvoke_RuntimeError_IncludesCORSHeader(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "crash", true)
	invoker.invokeErr = errors.New("ReferenceError: x is not defined")

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/crash", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 500, w.Code)
	testutil.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestEdgeFuncInvoke_QueryString(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "search", true)
	invoker.response = edgefunc.Response{StatusCode: 200, Body: []byte("found")}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/search?q=test&limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 200, w.Code)
	// Verify query string was forwarded to the edge function request.
	testutil.True(t, invoker.lastReq != nil, "expected Invoke to be called")
	testutil.Equal(t, "q=test&limit=10", invoker.lastReq.Query)
}

func TestEdgeFuncInvoke_SubPath(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "api", true)
	invoker.response = edgefunc.Response{StatusCode: 200, Body: []byte("sub")}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/api/users/123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 200, w.Code)
	testutil.Equal(t, "sub", w.Body.String())
	// Verify path was translated with sub-path appended.
	testutil.True(t, invoker.lastReq != nil, "expected Invoke to be called")
	testutil.Equal(t, "/api/users/123", invoker.lastReq.Path)
}

func TestEdgeFuncInvoke_ResponseHeaders(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "headers", true)
	invoker.response = edgefunc.Response{
		StatusCode: 200,
		Headers: map[string][]string{
			"X-Custom":     {"value1"},
			"X-Multi":      {"a", "b"},
			"Content-Type": {"text/plain"},
		},
		Body: []byte("ok"),
	}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/headers", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 200, w.Code)
	testutil.Equal(t, "value1", w.Header().Get("X-Custom"))
	testutil.Equal(t, "text/plain", w.Header().Get("Content-Type"))
	testutil.SliceLen(t, w.Header().Values("X-Multi"), 2)
}

func TestEdgeFuncInvoke_DefaultStatus200(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "default", true)
	invoker.response = edgefunc.Response{
		StatusCode: 0, // function didn't set status
		Body:       []byte("ok"),
	}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/default", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 200, w.Code)
}

func TestEdgeFuncInvoke_PrivateFunction_NoAuth_Rejected(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "secret", false) // private function

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/secret", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 401, w.Code)
	body := decodeErrorBody(t, w)
	testutil.Equal(t, "authentication required", body.Message)
}

func TestEdgeFuncInvoke_PrivateFunction_WithBearerNoValidation_Rejected(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "secret", false) // private function

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/secret", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 401, w.Code)
	body := decodeErrorBody(t, w)
	testutil.Equal(t, "authentication required", body.Message)
}

func TestEdgeFuncInvoke_BodySizeLimit(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "big", true)
	invoker.response = edgefunc.Response{StatusCode: 200, Body: []byte("ok")}

	r := edgefuncRouter(invoker)
	// Create a body larger than 1MB
	bigBody := strings.Repeat("x", 1<<20+1)
	req := httptest.NewRequest("POST", "/functions/v1/big", strings.NewReader(bigBody))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 413, w.Code)
}

func TestEdgeFuncInvoke_BodySizeLimit_Configurable(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "tiny", true)
	invoker.response = edgefunc.Response{StatusCode: 200, Body: []byte("ok")}

	r := chi.NewRouter()
	r.HandleFunc("/functions/v1/{name}", handleEdgeFuncInvoke(invoker, 8, nil, nil))
	r.HandleFunc("/functions/v1/{name}/*", handleEdgeFuncInvoke(invoker, 8, nil, nil))

	req := httptest.NewRequest("POST", "/functions/v1/tiny", strings.NewReader("123456789"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 413, w.Code)
}

func TestEdgeFuncInvoke_CORS_Preflight(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "cors", true)

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("OPTIONS", "/functions/v1/cors", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 204, w.Code)
	testutil.True(t, w.Header().Get("Access-Control-Allow-Origin") != "", "expected ACAO header")
	testutil.True(t, w.Header().Get("Access-Control-Allow-Methods") != "", "expected ACAM header")
	// Verify the edge function was NOT invoked for preflight.
	testutil.False(t, invoker.invoked, "preflight should not invoke the edge function")
}

func TestEdgeFuncInvoke_CORS_ActualRequest(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "cors-get", true)
	invoker.response = edgefunc.Response{StatusCode: 200, Body: []byte("ok")}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/cors-get", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 200, w.Code)
	// Non-OPTIONS responses to public functions must include CORS header
	// so browsers allow reading the response.
	testutil.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestEdgeFuncInvoke_PrivateFunction_WithBearer_Succeeds(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "secret", false) // private function
	invoker.response = edgefunc.Response{StatusCode: 200, Body: []byte("secret data")}

	r := edgefuncRouterWithValidator(invoker, func(_ context.Context, token string) error {
		if token != "valid-token" {
			return errors.New("invalid token")
		}
		return nil
	})
	req := httptest.NewRequest("GET", "/functions/v1/secret", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 200, w.Code)
	testutil.Equal(t, "secret data", w.Body.String())
	testutil.True(t, invoker.invoked, "expected edge function to be invoked with bearer token")
}

func TestEdgeFuncInvokeProxy_PrivateFunction_ValidJWT_Succeeds(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	seedFunction(admin, "secret", "function handler(req) { return { statusCode: 200, body: 'ok' }; }", false)

	authSvc := auth.NewService(nil, "this-is-a-secret-that-is-at-least-32-characters-long", time.Minute, time.Hour, 8, testutil.DiscardLogger())
	jwt, err := authSvc.IssueTestToken("user-123", "u@example.com")
	testutil.NoError(t, err)

	srv := &Server{
		cfg:         config.Default(),
		edgeFuncSvc: admin,
		authSvc:     authSvc,
	}
	r := chi.NewRouter()
	r.HandleFunc("/functions/v1/{name}", srv.handleEdgeFuncInvokeProxy)

	req := httptest.NewRequest("GET", "/functions/v1/secret", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 200, w.Code)
	testutil.True(t, admin.lastInvokeReq != nil, "expected edge function to be invoked")
}

func TestEdgeFuncInvokeProxy_PrivateFunction_InvalidJWT_Rejected(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	seedFunction(admin, "secret", "function handler(req) { return { statusCode: 200, body: 'ok' }; }", false)

	authSvc := auth.NewService(nil, "this-is-a-secret-that-is-at-least-32-characters-long", time.Minute, time.Hour, 8, testutil.DiscardLogger())

	srv := &Server{
		cfg:         config.Default(),
		edgeFuncSvc: admin,
		authSvc:     authSvc,
	}
	r := chi.NewRouter()
	r.HandleFunc("/functions/v1/{name}", srv.handleEdgeFuncInvokeProxy)

	req := httptest.NewRequest("GET", "/functions/v1/secret", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 401, w.Code)
	testutil.True(t, admin.lastInvokeReq == nil, "invalid token should not invoke function")
}

func TestEdgeFuncInvoke_EmptyBody(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "empty", true)
	invoker.response = edgefunc.Response{StatusCode: 204}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("DELETE", "/functions/v1/empty", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, 204, w.Code)
	testutil.Equal(t, 0, w.Body.Len())
}

func TestEdgeFuncInvoke_InvalidResponseStatus_BelowRange(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "bad-status", true)
	invoker.response = edgefunc.Response{StatusCode: 99, Body: []byte("bad")}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/bad-status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusInternalServerError, w.Code)
	body := decodeErrorBody(t, w)
	testutil.Equal(t, "invalid function response status code", body.Message)
}

func TestEdgeFuncInvoke_InvalidResponseStatus_AboveRange(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "bad-status", true)
	invoker.response = edgefunc.Response{StatusCode: 600, Body: []byte("bad")}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/bad-status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusInternalServerError, w.Code)
	body := decodeErrorBody(t, w)
	testutil.Equal(t, "invalid function response status code", body.Message)
}

func TestEdgeFuncInvoke_SetsTriggerHTTPMeta(t *testing.T) {
	t.Parallel()
	invoker := newFakeEdgeFuncInvoker()
	addFakeFunction(invoker, "trigger-test", true)
	invoker.response = edgefunc.Response{StatusCode: 200, Body: []byte("ok")}

	r := edgefuncRouter(invoker)
	req := httptest.NewRequest("GET", "/functions/v1/trigger-test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.True(t, invoker.invoked, "expected Invoke to be called")

	meta, ok := edgefunc.GetTriggerMeta(invoker.lastCtx)
	testutil.True(t, ok, "expected trigger metadata in context")
	testutil.Equal(t, string(edgefunc.TriggerHTTP), string(meta.Type))
}
