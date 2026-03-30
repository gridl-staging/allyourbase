package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// MaxEdgeFuncBodySize is the default maximum request body size for edge function invocations (1MB).
const MaxEdgeFuncBodySize int64 = 1 << 20

var errInvalidEdgeFuncStatusCode = errors.New("invalid function response status code")

// edgeFuncInvoker is the interface for looking up and invoking edge functions.
// edgefunc.Service satisfies this interface.
type edgeFuncInvoker interface {
	GetByName(ctx context.Context, name string) (*edgefunc.EdgeFunction, error)
	Invoke(ctx context.Context, name string, req edgefunc.Request) (edgefunc.Response, error)
}

type edgeFuncTokenValidator func(ctx context.Context, token string) error
type edgeFuncInvocationRecorder func(ctx context.Context, name, status string)

func handleEdgeFuncInvoke(svc edgeFuncInvoker, maxBodyBytes int64, validateToken edgeFuncTokenValidator, recordInvocation edgeFuncInvocationRecorder) http.HandlerFunc {
	if maxBodyBytes <= 0 {
		maxBodyBytes = MaxEdgeFuncBodySize
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// Public function endpoints are browser-facing; always include CORS on
		// success and error responses so callers can read failures.
		w.Header().Set("Access-Control-Allow-Origin", "*")

		name := chi.URLParam(r, "name")
		if name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "function name is required")
			return
		}

		// CORS preflight: respond immediately for OPTIONS requests.
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-Id")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		efReq, ok := resolveEdgeFuncRequest(w, r, svc, maxBodyBytes, validateToken, name)
		if !ok {
			return
		}

		ctx := edgefunc.WithTriggerMeta(r.Context(), edgefunc.TriggerHTTP, "")

		resp, err := svc.Invoke(ctx, name, *efReq)
		if err != nil {
			if recordInvocation != nil {
				recordInvocation(ctx, name, "error")
			}
			if errors.Is(err, edgefunc.ErrFunctionNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "function not found")
				return
			}
			if errors.Is(err, edgefunc.ErrConcurrencyLimitExceeded) {
				w.Header().Set("Retry-After", "1")
				httputil.WriteError(w, http.StatusTooManyRequests, "function concurrency limit exceeded")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "function execution failed")
			return
		}

		if recordInvocation != nil {
			recordInvocation(ctx, name, "ok")
		}

		if err := writeEdgeFuncResponse(w, resp); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
}

// resolveEdgeFuncRequest looks up the named edge function, enforces private-function
// auth, reads the bounded request body, and builds the edgefunc.Request. Returns nil
// and false if an error response was already written.
func resolveEdgeFuncRequest(w http.ResponseWriter, r *http.Request, svc edgeFuncInvoker, maxBodyBytes int64, validateToken edgeFuncTokenValidator, name string) (*edgefunc.Request, bool) {
	fn, err := svc.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, edgefunc.ErrFunctionNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "function not found")
			return nil, false
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load function")
		return nil, false
	}

	if !fn.Public {
		token, ok := httputil.ExtractBearerToken(r)
		if !ok || token == "" || validateToken == nil {
			httputil.WriteError(w, http.StatusUnauthorized, "authentication required")
			return nil, false
		}
		if err := validateToken(r.Context(), token); err != nil {
			httputil.WriteError(w, http.StatusUnauthorized, "authentication required")
			return nil, false
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body []byte
	if r.Body != nil {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			if isMaxBytesError(err) {
				httputil.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large")
				return nil, false
			}
			httputil.WriteError(w, http.StatusBadRequest, "failed to read request body")
			return nil, false
		}
	}

	subPath := chi.URLParam(r, "*")
	if subPath != "" && !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}

	return &edgefunc.Request{
		Method:  r.Method,
		Path:    "/" + name + subPath,
		Query:   r.URL.RawQuery,
		Headers: r.Header,
		Body:    body,
	}, true
}

// writeEdgeFuncResponse writes the edge function response to the HTTP response writer.
func writeEdgeFuncResponse(w http.ResponseWriter, resp edgefunc.Response) error {
	status, err := validatedEdgeFuncStatus(resp.StatusCode)
	if err != nil {
		return err
	}

	// Set response headers from the function.
	for k, vals := range resp.Headers {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(status)

	if len(resp.Body) > 0 {
		// Response is committed after WriteHeader; write errors mean the
		// connection is broken and the caller can't send a different status.
		// Discard the error, matching WriteJSON's pattern.
		_, _ = w.Write(resp.Body)
	}

	return nil
}

func validatedEdgeFuncStatus(status int) (int, error) {
	if status == 0 {
		return http.StatusOK, nil
	}
	if status < 100 || status > 599 {
		return 0, errInvalidEdgeFuncStatusCode
	}
	return status, nil
}

// isMaxBytesError checks if the error is from http.MaxBytesReader.
func isMaxBytesError(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

// --- Server delegation methods (nil-check + dispatch) ---

func (s *Server) handleEdgeFuncInvokeProxy(w http.ResponseWriter, r *http.Request) {
	if s.edgeFuncSvc == nil {
		serviceUnavailable(w, serviceUnavailableEdgeFunctions)
		return
	}
	var recorder edgeFuncInvocationRecorder
	if s.infraMetrics != nil {
		recorder = s.infraMetrics.RecordEdgeFuncInvocation
	}
	handleEdgeFuncInvoke(s.edgeFuncSvc, s.cfg.EdgeFunctions.MaxRequestBodyBytes, s.validateEdgeFuncBearerToken, recorder).ServeHTTP(w, r)
}

// validateEdgeFuncBearerToken validates a bearer token for edge function invocation,
// accepting OAuth tokens, API keys, or JWTs.
func (s *Server) validateEdgeFuncBearerToken(ctx context.Context, token string) error {
	if s.authSvc == nil {
		return fmt.Errorf("auth service not configured")
	}
	if auth.IsOAuthAccessToken(token) {
		_, err := s.authSvc.ValidateOAuthToken(ctx, token)
		return err
	}
	if auth.IsAPIKey(token) {
		_, err := s.authSvc.ValidateAPIKey(ctx, token)
		return err
	}
	claims, err := s.authSvc.ValidateToken(token)
	if err != nil {
		return err
	}
	if claims != nil && claims.MFAPending {
		return fmt.Errorf("mfa verification required")
	}
	return nil
}
