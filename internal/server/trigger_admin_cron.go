// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_3_webhook_and_trigger_proof/allyourbase_dev/internal/server/trigger_admin_cron.go.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type createCronTriggerRequest struct {
	CronExpr string          `json:"cron_expr"`
	Timezone string          `json:"timezone"`
	Payload  json.RawMessage `json:"payload"`
}

var cronTriggerMessages = triggerAdminMessages{
	notFound:      "cron trigger not found",
	getFailed:     "failed to get cron trigger",
	listFailed:    "failed to list cron triggers",
	deleteFailed:  "failed to delete cron trigger",
	enableFailed:  "failed to enable cron trigger",
	disableFailed: "failed to disable cron trigger",
}

func cronTriggerOwnerFunctionID(trigger *edgefunc.CronTrigger) string {
	return trigger.FunctionID
}

func isCronTriggerNotFound(err error) bool {
	return errors.Is(err, edgefunc.ErrCronTriggerNotFound)
}

// returns an HTTP handler that creates a cron trigger for a function, validating that the cron expression is provided and returning the created trigger or an HTTP error response.
func handleCreateCronTrigger(svc cronTriggerAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		functionID := chi.URLParam(r, "id")

		var req createCronTriggerRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.CronExpr == "" {
			httputil.WriteError(w, http.StatusBadRequest, "cron_expr is required")
			return
		}

		trigger, err := svc.Create(r.Context(), edgefunc.CreateCronTriggerInput{
			FunctionID: functionID,
			CronExpr:   req.CronExpr,
			Timezone:   req.Timezone,
			Payload:    req.Payload,
		})
		if err != nil {
			writeCronTriggerError(w, err)
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, trigger)
	}
}

func handleListCronTriggers(svc cronTriggerAdmin) http.HandlerFunc {
	return handleListTriggers(func(ctx context.Context, functionID string) ([]*edgefunc.CronTrigger, error) {
		return svc.List(ctx, functionID)
	}, cronTriggerMessages)
}

func handleGetCronTrigger(svc cronTriggerAdmin) http.HandlerFunc {
	return handleGetTrigger(func(ctx context.Context, triggerID string) (*edgefunc.CronTrigger, error) {
		return svc.Get(ctx, triggerID)
	}, cronTriggerOwnerFunctionID, isCronTriggerNotFound, cronTriggerMessages)
}

// returns an HTTP handler that deletes a cron trigger after verifying ownership, returning a 204 status on success or an HTTP error response on failure.
func handleDeleteCronTrigger(svc cronTriggerAdmin) http.HandlerFunc {
	return handleDeleteTrigger(
		func(ctx context.Context, triggerID string) (*edgefunc.CronTrigger, error) {
			return svc.Get(ctx, triggerID)
		},
		func(ctx context.Context, triggerID string) error {
			return svc.Delete(ctx, triggerID)
		},
		cronTriggerOwnerFunctionID,
		isCronTriggerNotFound,
		cronTriggerMessages,
	)
}

// returns an HTTP handler that enables a cron trigger, verifying ownership before updating and returning the modified trigger or an HTTP error response.
func handleEnableCronTrigger(svc cronTriggerAdmin) http.HandlerFunc {
	return handleSetTriggerEnabled(
		func(ctx context.Context, triggerID string) (*edgefunc.CronTrigger, error) {
			return svc.Get(ctx, triggerID)
		},
		func(ctx context.Context, triggerID string, enabled bool) (*edgefunc.CronTrigger, error) {
			return svc.SetEnabled(ctx, triggerID, enabled)
		},
		cronTriggerOwnerFunctionID,
		true,
		isCronTriggerNotFound,
		cronTriggerMessages,
	)
}

// returns an HTTP handler that disables a cron trigger, verifying ownership before updating and returning the modified trigger or an HTTP error response.
func handleDisableCronTrigger(svc cronTriggerAdmin) http.HandlerFunc {
	return handleSetTriggerEnabled(
		func(ctx context.Context, triggerID string) (*edgefunc.CronTrigger, error) {
			return svc.Get(ctx, triggerID)
		},
		func(ctx context.Context, triggerID string, enabled bool) (*edgefunc.CronTrigger, error) {
			return svc.SetEnabled(ctx, triggerID, enabled)
		},
		cronTriggerOwnerFunctionID,
		false,
		isCronTriggerNotFound,
		cronTriggerMessages,
	)
}

// TODO: Document handleManualRunCronTrigger.
func handleManualRunCronTrigger(svc cronTriggerAdmin, invoker edgefunc.FunctionInvoker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trigger := getOwnedTrigger(w, r, func(ctx context.Context, triggerID string) (*edgefunc.CronTrigger, error) {
			return svc.Get(ctx, triggerID)
		}, cronTriggerOwnerFunctionID, isCronTriggerNotFound, cronTriggerMessages)
		if trigger == nil {
			return
		}
		if !trigger.Enabled {
			httputil.WriteError(w, http.StatusConflict, "cron trigger is disabled")
			return
		}

		req := edgefunc.Request{
			Method: "POST",
			Path:   "/cron",
			Body:   trigger.Payload,
		}

		ctx := edgefunc.WithTriggerMeta(r.Context(), edgefunc.TriggerCron, trigger.ID)
		resp, err := invoker.InvokeByID(ctx, trigger.FunctionID, req)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "function invocation failed")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"statusCode": resp.StatusCode,
			"body":       string(resp.Body),
		})
	}
}

func writeCronTriggerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, edgefunc.ErrCronTriggerNotFound):
		httputil.WriteError(w, http.StatusNotFound, "cron trigger not found")
	case errors.Is(err, edgefunc.ErrInvalidCronExpr),
		errors.Is(err, edgefunc.ErrInvalidTimezone),
		errors.Is(err, edgefunc.ErrCronExprRequired),
		errors.Is(err, edgefunc.ErrFunctionIDRequired):
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httputil.WriteError(w, http.StatusInternalServerError, "failed to process cron trigger")
	}
}

func (s *Server) handleCronTriggerList(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.cronTriggerSvc != nil, handleListCronTriggers(s.cronTriggerSvc))
}

func (s *Server) handleCronTriggerCreate(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.cronTriggerSvc != nil, handleCreateCronTrigger(s.cronTriggerSvc))
}

func (s *Server) handleCronTriggerGet(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.cronTriggerSvc != nil, handleGetCronTrigger(s.cronTriggerSvc))
}

func (s *Server) handleCronTriggerDelete(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.cronTriggerSvc != nil, handleDeleteCronTrigger(s.cronTriggerSvc))
}

func (s *Server) handleCronTriggerEnable(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.cronTriggerSvc != nil, handleEnableCronTrigger(s.cronTriggerSvc))
}

func (s *Server) handleCronTriggerDisable(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.cronTriggerSvc != nil, handleDisableCronTrigger(s.cronTriggerSvc))
}

func (s *Server) handleCronTriggerManualRun(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.cronTriggerSvc != nil && s.funcInvoker != nil, handleManualRunCronTrigger(s.cronTriggerSvc, s.funcInvoker))
}
