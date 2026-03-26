// Package server Admin HTTP handlers for edge function deployment, management, logging, and test execution.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// edgeFuncAdmin is the interface for edge function management (admin + public trigger).
// edgefunc.Service satisfies this interface.
type edgeFuncAdmin interface {
	Deploy(ctx context.Context, name, source string, opts edgefunc.DeployOptions) (*edgefunc.EdgeFunction, error)
	Get(ctx context.Context, id uuid.UUID) (*edgefunc.EdgeFunction, error)
	GetByName(ctx context.Context, name string) (*edgefunc.EdgeFunction, error)
	List(ctx context.Context, page, perPage int) ([]*edgefunc.EdgeFunction, error)
	Update(ctx context.Context, id uuid.UUID, source string, opts edgefunc.DeployOptions) (*edgefunc.EdgeFunction, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Invoke(ctx context.Context, name string, req edgefunc.Request) (edgefunc.Response, error)
	ListLogs(ctx context.Context, functionID uuid.UUID, opts edgefunc.LogListOptions) ([]*edgefunc.LogEntry, error)
}

// --- Deploy request/response ---

type adminDeployFunctionRequest struct {
	Name       string            `json:"name"`
	Source     string            `json:"source"`
	EntryPoint string            `json:"entry_point"`
	TimeoutMs  int               `json:"timeout_ms"`
	EnvVars    map[string]string `json:"env_vars"`
	Public     bool              `json:"public"`
}

// --- Update request ---

type adminUpdateFunctionRequest struct {
	Source     string            `json:"source"`
	EntryPoint string            `json:"entry_point"`
	TimeoutMs  int               `json:"timeout_ms"`
	EnvVars    map[string]string `json:"env_vars"`
	Public     bool              `json:"public"`
}

// --- Admin invoke request ---

type adminInvokeFunctionRequest struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body"`
}

// --- Admin invoke response ---

type adminInvokeFunctionResponse struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       string              `json:"body,omitempty"`
}

// --- Handlers ---

// handleAdminListFunctions returns a paginated list of all edge functions.
func handleAdminListFunctions(svc edgeFuncAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))

		functions, err := svc.List(r.Context(), page, perPage)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list functions")
			return
		}

		// Never return null — return empty array.
		if functions == nil {
			functions = []*edgefunc.EdgeFunction{}
		}

		httputil.WriteJSON(w, http.StatusOK, functions)
	}
}

// handleAdminDeployFunction deploys a new edge function.
func handleAdminDeployFunction(svc edgeFuncAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req adminDeployFunctionRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Source == "" {
			httputil.WriteError(w, http.StatusBadRequest, "source is required")
			return
		}

		fn, err := svc.Deploy(r.Context(), req.Name, req.Source, edgefunc.DeployOptions{
			EntryPoint: req.EntryPoint,
			TimeoutMs:  req.TimeoutMs,
			EnvVars:    req.EnvVars,
			Public:     req.Public,
		})
		if err != nil {
			if errors.Is(err, edgefunc.ErrFunctionNameConflict) {
				httputil.WriteError(w, http.StatusConflict, "function name already exists")
				return
			}
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, fn)
	}
}

// handleAdminGetFunction returns a function by ID, including source.
func handleAdminGetFunction(svc edgeFuncAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParamWithLabel(w, r, "id", "function id")
		if !ok {
			return
		}

		fn, err := svc.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, edgefunc.ErrFunctionNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "function not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get function")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, fn)
	}
}

// handleAdminGetFunctionByName returns a function by name, including source.
func handleAdminGetFunctionByName(svc edgeFuncAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}

		fn, err := svc.GetByName(r.Context(), name)
		if err != nil {
			if errors.Is(err, edgefunc.ErrFunctionNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "function not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get function")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, fn)
	}
}

// handleAdminUpdateFunction updates a function (re-transpiles + re-compiles on source change).
func handleAdminUpdateFunction(svc edgeFuncAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParamWithLabel(w, r, "id", "function id")
		if !ok {
			return
		}

		var req adminUpdateFunctionRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.Source == "" {
			httputil.WriteError(w, http.StatusBadRequest, "source is required")
			return
		}

		fn, err := svc.Update(r.Context(), id, req.Source, edgefunc.DeployOptions{
			EntryPoint: req.EntryPoint,
			TimeoutMs:  req.TimeoutMs,
			EnvVars:    req.EnvVars,
			Public:     req.Public,
		})
		if err != nil {
			if errors.Is(err, edgefunc.ErrFunctionNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "function not found")
				return
			}
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		httputil.WriteJSON(w, http.StatusOK, fn)
	}
}

// handleAdminDeleteFunction deletes a function and its logs.
func handleAdminDeleteFunction(svc edgeFuncAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParamWithLabel(w, r, "id", "function id")
		if !ok {
			return
		}

		err := svc.Delete(r.Context(), id)
		if err != nil {
			if errors.Is(err, edgefunc.ErrFunctionNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "function not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to delete function")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleAdminFunctionLogs returns execution logs for a function (paginated, newest first).
func handleAdminFunctionLogs(svc edgeFuncAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParamWithLabel(w, r, "id", "function id")
		if !ok {
			return
		}

		logOpts, err := parseAdminFunctionLogListOptions(r)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		logs, err := svc.ListLogs(r.Context(), id, logOpts)
		if err != nil {
			if errors.Is(err, edgefunc.ErrFunctionNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "function not found")
				return
			}
			if errors.Is(err, edgefunc.ErrInvalidLogFilter) {
				httputil.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list logs")
			return
		}

		if logs == nil {
			logs = []*edgefunc.LogEntry{}
		}

		httputil.WriteJSON(w, http.StatusOK, logs)
	}
}

// parseAdminFunctionLogListOptions extracts and parses query parameters into a LogListOptions struct. It accepts optional page, perPage, limit, status, trigger_type, since, and until parameters. If limit is provided, it takes precedence over page and perPage. Timestamps must be RFC3339-formatted. Returns an error for invalid parameters.
func parseAdminFunctionLogListOptions(r *http.Request) (edgefunc.LogListOptions, error) {
	var opts edgefunc.LogListOptions
	query := r.URL.Query()

	page, err := parseOptionalIntQuery(query.Get("page"), "page")
	if err != nil {
		return edgefunc.LogListOptions{}, err
	}
	perPage, err := parseOptionalIntQuery(query.Get("perPage"), "perPage")
	if err != nil {
		return edgefunc.LogListOptions{}, err
	}
	limit, err := parseOptionalIntQuery(query.Get("limit"), "limit")
	if err != nil {
		return edgefunc.LogListOptions{}, err
	}

	if limit != 0 {
		opts.Page = 1
		opts.PerPage = limit
	} else {
		opts.Page = page
		opts.PerPage = perPage
	}

	opts.Status = query.Get("status")
	opts.TriggerType = query.Get("trigger_type")

	sinceRaw := query.Get("since")
	if sinceRaw != "" {
		since, err := time.Parse(time.RFC3339, sinceRaw)
		if err != nil {
			return edgefunc.LogListOptions{}, fmt.Errorf("%w: invalid since timestamp", edgefunc.ErrInvalidLogFilter)
		}
		opts.Since = &since
	}

	untilRaw := query.Get("until")
	if untilRaw != "" {
		until, err := time.Parse(time.RFC3339, untilRaw)
		if err != nil {
			return edgefunc.LogListOptions{}, fmt.Errorf("%w: invalid until timestamp", edgefunc.ErrInvalidLogFilter)
		}
		opts.Until = &until
	}

	normalized, err := edgefunc.NormalizeLogListOptions(opts)
	if err != nil {
		return edgefunc.LogListOptions{}, err
	}
	return normalized, nil
}

func parseOptionalIntQuery(raw, field string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid %s query param", edgefunc.ErrInvalidLogFilter, field)
	}
	return n, nil
}

// handleAdminInvokeFunction test-invokes a function from the admin dashboard.
func handleAdminInvokeFunction(svc edgeFuncAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParamWithLabel(w, r, "id", "function id")
		if !ok {
			return
		}

		// Look up function to get name.
		fn, err := svc.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, edgefunc.ErrFunctionNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "function not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get function")
			return
		}

		var req adminInvokeFunctionRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		method := req.Method
		if method == "" {
			method = "GET"
		}
		path := req.Path
		if path == "" {
			path = "/" + fn.Name
		}

		efReq := edgefunc.Request{
			Method:  method,
			Path:    path,
			Headers: req.Headers,
			Body:    []byte(req.Body),
		}

		ctx := edgefunc.WithTriggerMeta(r.Context(), edgefunc.TriggerHTTP, "")
		resp, err := svc.Invoke(ctx, fn.Name, efReq)
		if err != nil {
			if errors.Is(err, edgefunc.ErrFunctionNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "function not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "function execution failed")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, adminInvokeFunctionResponse{
			StatusCode: resp.StatusCode,
			Headers:    resp.Headers,
			Body:       string(resp.Body),
		})
	}
}

// --- Server delegation methods (nil-check + dispatch) ---

func (s *Server) handleEdgeFuncAdminList(w http.ResponseWriter, r *http.Request) {
	if s.edgeFuncSvc == nil {
		serviceUnavailable(w, serviceUnavailableEdgeFunctions)
		return
	}
	handleAdminListFunctions(s.edgeFuncSvc).ServeHTTP(w, r)
}

func (s *Server) handleEdgeFuncAdminDeploy(w http.ResponseWriter, r *http.Request) {
	if s.edgeFuncSvc == nil {
		serviceUnavailable(w, serviceUnavailableEdgeFunctions)
		return
	}
	handleAdminDeployFunction(s.edgeFuncSvc).ServeHTTP(w, r)
}

func (s *Server) handleEdgeFuncAdminGet(w http.ResponseWriter, r *http.Request) {
	if s.edgeFuncSvc == nil {
		serviceUnavailable(w, serviceUnavailableEdgeFunctions)
		return
	}
	handleAdminGetFunction(s.edgeFuncSvc).ServeHTTP(w, r)
}

func (s *Server) handleEdgeFuncAdminGetByName(w http.ResponseWriter, r *http.Request) {
	if s.edgeFuncSvc == nil {
		serviceUnavailable(w, serviceUnavailableEdgeFunctions)
		return
	}
	handleAdminGetFunctionByName(s.edgeFuncSvc).ServeHTTP(w, r)
}

func (s *Server) handleEdgeFuncAdminUpdate(w http.ResponseWriter, r *http.Request) {
	if s.edgeFuncSvc == nil {
		serviceUnavailable(w, serviceUnavailableEdgeFunctions)
		return
	}
	handleAdminUpdateFunction(s.edgeFuncSvc).ServeHTTP(w, r)
}

func (s *Server) handleEdgeFuncAdminDelete(w http.ResponseWriter, r *http.Request) {
	if s.edgeFuncSvc == nil {
		serviceUnavailable(w, serviceUnavailableEdgeFunctions)
		return
	}
	handleAdminDeleteFunction(s.edgeFuncSvc).ServeHTTP(w, r)
}

func (s *Server) handleEdgeFuncAdminLogs(w http.ResponseWriter, r *http.Request) {
	if s.edgeFuncSvc == nil {
		serviceUnavailable(w, serviceUnavailableEdgeFunctions)
		return
	}
	handleAdminFunctionLogs(s.edgeFuncSvc).ServeHTTP(w, r)
}

func (s *Server) handleEdgeFuncAdminInvoke(w http.ResponseWriter, r *http.Request) {
	if s.edgeFuncSvc == nil {
		serviceUnavailable(w, serviceUnavailableEdgeFunctions)
		return
	}
	handleAdminInvokeFunction(s.edgeFuncSvc).ServeHTTP(w, r)
}
