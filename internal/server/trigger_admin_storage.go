package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type createStorageTriggerRequest struct {
	Bucket       string   `json:"bucket"`
	EventTypes   []string `json:"event_types"`
	PrefixFilter string   `json:"prefix_filter"`
	SuffixFilter string   `json:"suffix_filter"`
}

var storageTriggerMessages = triggerAdminMessages{
	notFound:      "storage trigger not found",
	getFailed:     "failed to get storage trigger",
	listFailed:    "failed to list storage triggers",
	deleteFailed:  "failed to delete storage trigger",
	enableFailed:  "failed to enable storage trigger",
	disableFailed: "failed to disable storage trigger",
}

func storageTriggerOwnerFunctionID(trigger *edgefunc.StorageTrigger) string {
	return trigger.FunctionID
}

func isStorageTriggerNotFound(err error) bool {
	return errors.Is(err, edgefunc.ErrStorageTriggerNotFound)
}

// returns an HTTP handler that creates a storage trigger for a function, validating that the bucket and event types are provided and returning the created trigger or an HTTP error response.
func handleCreateStorageTrigger(svc storageTriggerAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		functionID := chi.URLParam(r, "id")

		var req createStorageTriggerRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.Bucket == "" {
			httputil.WriteError(w, http.StatusBadRequest, "bucket is required")
			return
		}
		if len(req.EventTypes) == 0 {
			httputil.WriteError(w, http.StatusBadRequest, "event_types is required")
			return
		}

		trigger, err := svc.Create(r.Context(), edgefunc.CreateStorageTriggerInput{
			FunctionID:   functionID,
			Bucket:       req.Bucket,
			EventTypes:   req.EventTypes,
			PrefixFilter: req.PrefixFilter,
			SuffixFilter: req.SuffixFilter,
		})
		if err != nil {
			writeStorageTriggerError(w, err)
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, trigger)
	}
}

func handleListStorageTriggers(svc storageTriggerAdmin) http.HandlerFunc {
	return handleListTriggers(func(ctx context.Context, functionID string) ([]*edgefunc.StorageTrigger, error) {
		return svc.List(ctx, functionID)
	}, storageTriggerMessages)
}

func handleGetStorageTrigger(svc storageTriggerAdmin) http.HandlerFunc {
	return handleGetTrigger(func(ctx context.Context, triggerID string) (*edgefunc.StorageTrigger, error) {
		return svc.Get(ctx, triggerID)
	}, storageTriggerOwnerFunctionID, isStorageTriggerNotFound, storageTriggerMessages)
}

// returns an HTTP handler that deletes a storage trigger after verifying ownership, returning a 204 status on success or an HTTP error response on failure.
func handleDeleteStorageTrigger(svc storageTriggerAdmin) http.HandlerFunc {
	return handleDeleteTrigger(
		func(ctx context.Context, triggerID string) (*edgefunc.StorageTrigger, error) {
			return svc.Get(ctx, triggerID)
		},
		func(ctx context.Context, triggerID string) error {
			return svc.Delete(ctx, triggerID)
		},
		storageTriggerOwnerFunctionID,
		isStorageTriggerNotFound,
		storageTriggerMessages,
	)
}

// returns an HTTP handler that enables a storage trigger, verifying ownership before updating and returning the modified trigger or an HTTP error response.
func handleEnableStorageTrigger(svc storageTriggerAdmin) http.HandlerFunc {
	return handleSetTriggerEnabled(
		func(ctx context.Context, triggerID string) (*edgefunc.StorageTrigger, error) {
			return svc.Get(ctx, triggerID)
		},
		func(ctx context.Context, triggerID string, enabled bool) (*edgefunc.StorageTrigger, error) {
			return svc.SetEnabled(ctx, triggerID, enabled)
		},
		storageTriggerOwnerFunctionID,
		true,
		isStorageTriggerNotFound,
		storageTriggerMessages,
	)
}

// returns an HTTP handler that disables a storage trigger, verifying ownership before updating and returning the modified trigger or an HTTP error response.
func handleDisableStorageTrigger(svc storageTriggerAdmin) http.HandlerFunc {
	return handleSetTriggerEnabled(
		func(ctx context.Context, triggerID string) (*edgefunc.StorageTrigger, error) {
			return svc.Get(ctx, triggerID)
		},
		func(ctx context.Context, triggerID string, enabled bool) (*edgefunc.StorageTrigger, error) {
			return svc.SetEnabled(ctx, triggerID, enabled)
		},
		storageTriggerOwnerFunctionID,
		false,
		isStorageTriggerNotFound,
		storageTriggerMessages,
	)
}

func writeStorageTriggerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, edgefunc.ErrStorageTriggerNotFound):
		httputil.WriteError(w, http.StatusNotFound, "storage trigger not found")
	case errors.Is(err, edgefunc.ErrBucketRequired),
		errors.Is(err, edgefunc.ErrEventTypesRequired),
		errors.Is(err, edgefunc.ErrInvalidEventType),
		errors.Is(err, edgefunc.ErrFunctionIDRequired):
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httputil.WriteError(w, http.StatusInternalServerError, "failed to process storage trigger")
	}
}

func (s *Server) handleStorageTriggerList(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.storageTriggerSvc != nil, handleListStorageTriggers(s.storageTriggerSvc))
}

func (s *Server) handleStorageTriggerCreate(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.storageTriggerSvc != nil, handleCreateStorageTrigger(s.storageTriggerSvc))
}

func (s *Server) handleStorageTriggerGet(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.storageTriggerSvc != nil, handleGetStorageTrigger(s.storageTriggerSvc))
}

func (s *Server) handleStorageTriggerDelete(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.storageTriggerSvc != nil, handleDeleteStorageTrigger(s.storageTriggerSvc))
}

func (s *Server) handleStorageTriggerEnable(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.storageTriggerSvc != nil, handleEnableStorageTrigger(s.storageTriggerSvc))
}

func (s *Server) handleStorageTriggerDisable(w http.ResponseWriter, r *http.Request) {
	serveTriggerAdminRequest(w, r, s.storageTriggerSvc != nil, handleDisableStorageTrigger(s.storageTriggerSvc))
}
