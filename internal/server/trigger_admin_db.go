package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type createDBTriggerRequest struct {
	TableName     string                    `json:"table_name"`
	Schema        string                    `json:"schema"`
	Events        []edgefunc.DBTriggerEvent `json:"events"`
	FilterColumns []string                  `json:"filter_columns"`
}

var dbTriggerMessages = triggerAdminMessages{
	notFound:      "db trigger not found",
	getFailed:     "failed to get db trigger",
	listFailed:    "failed to list db triggers",
	deleteFailed:  "failed to delete db trigger",
	enableFailed:  "failed to enable db trigger",
	disableFailed: "failed to disable db trigger",
}

func dbTriggerOwnerFunctionID(trigger *edgefunc.DBTrigger) string {
	return trigger.FunctionID
}

func isDBTriggerNotFound(err error) bool {
	return errors.Is(err, edgefunc.ErrDBTriggerNotFound)
}

// returns an HTTP handler that creates a database trigger for a function, validating that the table name and events are provided and returning the created trigger or an HTTP error response.
func handleCreateDBTrigger(svc dbTriggerAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		functionID := chi.URLParam(r, "id")

		var req createDBTriggerRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.TableName == "" {
			httputil.WriteError(w, http.StatusBadRequest, "table_name is required")
			return
		}
		if len(req.Events) == 0 {
			httputil.WriteError(w, http.StatusBadRequest, "events is required")
			return
		}

		trigger, err := svc.Create(r.Context(), edgefunc.CreateDBTriggerInput{
			FunctionID:    functionID,
			TableName:     req.TableName,
			Schema:        req.Schema,
			Events:        req.Events,
			FilterColumns: req.FilterColumns,
		})
		if err != nil {
			writeDBTriggerError(w, err)
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, trigger)
	}
}

func handleListDBTriggers(svc dbTriggerAdmin) http.HandlerFunc {
	return handleListTriggers(func(ctx context.Context, functionID string) ([]*edgefunc.DBTrigger, error) {
		return svc.List(ctx, functionID)
	}, dbTriggerMessages)
}

func handleGetDBTrigger(svc dbTriggerAdmin) http.HandlerFunc {
	return handleGetTrigger(func(ctx context.Context, triggerID string) (*edgefunc.DBTrigger, error) {
		return svc.Get(ctx, triggerID)
	}, dbTriggerOwnerFunctionID, isDBTriggerNotFound, dbTriggerMessages)
}

// returns an HTTP handler that deletes a database trigger after verifying ownership, returning a 204 status on success or an HTTP error response on failure.
func handleDeleteDBTrigger(svc dbTriggerAdmin) http.HandlerFunc {
	return handleDeleteTrigger(
		func(ctx context.Context, triggerID string) (*edgefunc.DBTrigger, error) {
			return svc.Get(ctx, triggerID)
		},
		func(ctx context.Context, triggerID string) error {
			return svc.Delete(ctx, triggerID)
		},
		dbTriggerOwnerFunctionID,
		isDBTriggerNotFound,
		dbTriggerMessages,
	)
}

// returns an HTTP handler that enables a database trigger, verifying ownership before updating and returning the modified trigger or an HTTP error response.
func handleEnableDBTrigger(svc dbTriggerAdmin) http.HandlerFunc {
	return handleSetTriggerEnabled(
		func(ctx context.Context, triggerID string) (*edgefunc.DBTrigger, error) {
			return svc.Get(ctx, triggerID)
		},
		func(ctx context.Context, triggerID string, enabled bool) (*edgefunc.DBTrigger, error) {
			return svc.SetEnabled(ctx, triggerID, enabled)
		},
		dbTriggerOwnerFunctionID,
		true,
		isDBTriggerNotFound,
		dbTriggerMessages,
	)
}

// returns an HTTP handler that disables a database trigger, verifying ownership before updating and returning the modified trigger or an HTTP error response.
func handleDisableDBTrigger(svc dbTriggerAdmin) http.HandlerFunc {
	return handleSetTriggerEnabled(
		func(ctx context.Context, triggerID string) (*edgefunc.DBTrigger, error) {
			return svc.Get(ctx, triggerID)
		},
		func(ctx context.Context, triggerID string, enabled bool) (*edgefunc.DBTrigger, error) {
			return svc.SetEnabled(ctx, triggerID, enabled)
		},
		dbTriggerOwnerFunctionID,
		false,
		isDBTriggerNotFound,
		dbTriggerMessages,
	)
}

// converts database trigger-related errors into appropriate HTTP error responses, mapping specific error types to status codes and messages.
func writeDBTriggerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, edgefunc.ErrDBTriggerNotFound):
		httputil.WriteError(w, http.StatusNotFound, "db trigger not found")
	case errors.Is(err, edgefunc.ErrDBTriggerDuplicate):
		httputil.WriteError(w, http.StatusConflict, "db trigger already exists for this function and table")
	case errors.Is(err, edgefunc.ErrTableNameRequired),
		errors.Is(err, edgefunc.ErrDBEventsRequired),
		errors.Is(err, edgefunc.ErrInvalidDBEvent),
		errors.Is(err, edgefunc.ErrInvalidIdentifier),
		errors.Is(err, edgefunc.ErrFunctionIDRequired):
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httputil.WriteError(w, http.StatusInternalServerError, "failed to process db trigger")
	}
}

func (s *Server) handleDBTriggerList(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.dbTriggerSvc != nil, handleListDBTriggers(s.dbTriggerSvc))
}

func (s *Server) handleDBTriggerCreate(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.dbTriggerSvc != nil, handleCreateDBTrigger(s.dbTriggerSvc))
}

func (s *Server) handleDBTriggerGet(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.dbTriggerSvc != nil, handleGetDBTrigger(s.dbTriggerSvc))
}

func (s *Server) handleDBTriggerDelete(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.dbTriggerSvc != nil, handleDeleteDBTrigger(s.dbTriggerSvc))
}

func (s *Server) handleDBTriggerEnable(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.dbTriggerSvc != nil, handleEnableDBTrigger(s.dbTriggerSvc))
}

func (s *Server) handleDBTriggerDisable(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.dbTriggerSvc != nil, handleDisableDBTrigger(s.dbTriggerSvc))
}
