// Package server Provides HTTP handlers for administering database, cron, and storage triggers attached to edge functions.
package server

import (
	"context"
	"net/http"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// --- Admin interfaces for trigger services ---

type dbTriggerAdmin interface {
	Create(ctx context.Context, input edgefunc.CreateDBTriggerInput) (*edgefunc.DBTrigger, error)
	Get(ctx context.Context, id string) (*edgefunc.DBTrigger, error)
	List(ctx context.Context, functionID string) ([]*edgefunc.DBTrigger, error)
	Delete(ctx context.Context, id string) error
	SetEnabled(ctx context.Context, id string, enabled bool) (*edgefunc.DBTrigger, error)
}

type cronTriggerAdmin interface {
	Create(ctx context.Context, input edgefunc.CreateCronTriggerInput) (*edgefunc.CronTrigger, error)
	Get(ctx context.Context, id string) (*edgefunc.CronTrigger, error)
	List(ctx context.Context, functionID string) ([]*edgefunc.CronTrigger, error)
	Delete(ctx context.Context, id string) error
	SetEnabled(ctx context.Context, id string, enabled bool) (*edgefunc.CronTrigger, error)
}

type storageTriggerAdmin interface {
	Create(ctx context.Context, input edgefunc.CreateStorageTriggerInput) (*edgefunc.StorageTrigger, error)
	Get(ctx context.Context, id string) (*edgefunc.StorageTrigger, error)
	List(ctx context.Context, functionID string) ([]*edgefunc.StorageTrigger, error)
	Delete(ctx context.Context, id string) error
	SetEnabled(ctx context.Context, id string, enabled bool) (*edgefunc.StorageTrigger, error)
}

type triggerAdminMessages struct {
	notFound      string
	getFailed     string
	listFailed    string
	deleteFailed  string
	enableFailed  string
	disableFailed string
}

// getOwnedTrigger retrieves and validates ownership of a trigger, returning nil and writing an error response if the trigger is not found or not owned by the specified function.
func getOwnedTrigger[T any](
	w http.ResponseWriter,
	r *http.Request,
	lookup func(context.Context, string) (*T, error),
	ownerFunctionID func(*T) string,
	isNotFound func(error) bool,
	messages triggerAdminMessages,
) *T {
	functionID := chi.URLParam(r, "id")
	triggerID := chi.URLParam(r, "triggerId")

	trigger, err := lookup(r.Context(), triggerID)
	if err != nil {
		if isNotFound(err) {
			httputil.WriteError(w, http.StatusNotFound, messages.notFound)
			return nil
		}
		httputil.WriteError(w, http.StatusInternalServerError, messages.getFailed)
		return nil
	}
	if ownerFunctionID(trigger) != functionID {
		httputil.WriteError(w, http.StatusNotFound, messages.notFound)
		return nil
	}
	return trigger
}

func handleListTriggers[T any](list func(context.Context, string) ([]*T, error), messages triggerAdminMessages) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		triggers, err := list(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, messages.listFailed)
			return
		}
		if triggers == nil {
			triggers = []*T{}
		}
		httputil.WriteJSON(w, http.StatusOK, triggers)
	}
}

func handleGetTrigger[T any](
	lookup func(context.Context, string) (*T, error),
	ownerFunctionID func(*T) string,
	isNotFound func(error) bool,
	messages triggerAdminMessages,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trigger := getOwnedTrigger(w, r, lookup, ownerFunctionID, isNotFound, messages)
		if trigger == nil {
			return
		}
		httputil.WriteJSON(w, http.StatusOK, trigger)
	}
}

// handleDeleteTrigger returns an HTTP handler that verifies trigger ownership and deletes the trigger, returning 204 No Content on success.
func handleDeleteTrigger[T any](
	lookup func(context.Context, string) (*T, error),
	deleteTrigger func(context.Context, string) error,
	ownerFunctionID func(*T) string,
	isNotFound func(error) bool,
	messages triggerAdminMessages,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trigger := getOwnedTrigger(w, r, lookup, ownerFunctionID, isNotFound, messages)
		if trigger == nil {
			return
		}

		triggerID := chi.URLParam(r, "triggerId")
		if err := deleteTrigger(r.Context(), triggerID); err != nil {
			if isNotFound(err) {
				httputil.WriteError(w, http.StatusNotFound, messages.notFound)
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, messages.deleteFailed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleSetTriggerEnabled returns an HTTP handler that verifies trigger ownership and updates the trigger's enabled status, returning the updated trigger.
func handleSetTriggerEnabled[T any](
	lookup func(context.Context, string) (*T, error),
	setEnabled func(context.Context, string, bool) (*T, error),
	ownerFunctionID func(*T) string,
	enabled bool,
	isNotFound func(error) bool,
	messages triggerAdminMessages,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trigger := getOwnedTrigger(w, r, lookup, ownerFunctionID, isNotFound, messages)
		if trigger == nil {
			return
		}

		updatedTrigger, err := setEnabled(r.Context(), chi.URLParam(r, "triggerId"), enabled)
		if err != nil {
			if isNotFound(err) {
				httputil.WriteError(w, http.StatusNotFound, messages.notFound)
				return
			}
			statusMessage := messages.disableFailed
			if enabled {
				statusMessage = messages.enableFailed
			}
			httputil.WriteError(w, http.StatusInternalServerError, statusMessage)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, updatedTrigger)
	}
}

func serveTriggerAdminRequest(w http.ResponseWriter, r *http.Request, available bool, handler http.Handler) {
	if !available {
		serviceUnavailable(w, serviceUnavailableEdgeFunctions)
		return
	}
	handler.ServeHTTP(w, r)
}
