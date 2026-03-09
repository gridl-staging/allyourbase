package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/matview"
	"github.com/go-chi/chi/v5"
)

// matviewAdmin is the interface for matview admin operations.
// matview.Store + matview.Service together satisfy this.
type matviewAdmin interface {
	List(ctx context.Context) ([]matview.Registration, error)
	Get(ctx context.Context, id string) (*matview.Registration, error)
	Register(ctx context.Context, schemaName, viewName string, mode matview.RefreshMode) (*matview.Registration, error)
	Update(ctx context.Context, id string, mode matview.RefreshMode) (*matview.Registration, error)
	Delete(ctx context.Context, id string) error
	RefreshNow(ctx context.Context, id string) (*matview.RefreshResult, error)
}

type matviewListResponse struct {
	Items []matview.Registration `json:"items"`
	Count int                    `json:"count"`
}

type registerMatviewRequest struct {
	Schema      string             `json:"schema"`
	ViewName    string             `json:"viewName"`
	RefreshMode matview.RefreshMode `json:"refreshMode"`
}

type updateMatviewRequest struct {
	RefreshMode matview.RefreshMode `json:"refreshMode"`
}

func handleAdminListMatviews(svc matviewAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := svc.List(r.Context())
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list matviews")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, matviewListResponse{
			Items: items,
			Count: len(items),
		})
	}
}

func handleAdminGetMatview(svc matviewAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid matview id format")
			return
		}
		reg, err := svc.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, matview.ErrRegistrationNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "matview registration not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get matview")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, reg)
	}
}

func handleAdminRegisterMatview(svc matviewAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerMatviewRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.ViewName == "" {
			httputil.WriteError(w, http.StatusBadRequest, "viewName is required")
			return
		}
		if req.Schema == "" {
			req.Schema = "public"
		}
		if req.RefreshMode == "" {
			req.RefreshMode = matview.RefreshModeStandard
		}
		if req.RefreshMode != matview.RefreshModeStandard && req.RefreshMode != matview.RefreshModeConcurrent {
			httputil.WriteError(w, http.StatusBadRequest, "refreshMode must be 'standard' or 'concurrent'")
			return
		}

		reg, err := svc.Register(r.Context(), req.Schema, req.ViewName, req.RefreshMode)
		if err != nil {
			if errors.Is(err, matview.ErrNotMaterializedView) {
				httputil.WriteError(w, http.StatusNotFound, "materialized view not found in database")
				return
			}
			if errors.Is(err, matview.ErrInvalidIdentifier) {
				httputil.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			if errors.Is(err, matview.ErrDuplicateRegistration) {
				httputil.WriteError(w, http.StatusConflict, "materialized view already registered")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to register matview")
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, reg)
	}
}

func handleAdminUpdateMatview(svc matviewAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid matview id format")
			return
		}

		var req updateMatviewRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.RefreshMode != matview.RefreshModeStandard && req.RefreshMode != matview.RefreshModeConcurrent {
			httputil.WriteError(w, http.StatusBadRequest, "refreshMode must be 'standard' or 'concurrent'")
			return
		}

		reg, err := svc.Update(r.Context(), id, req.RefreshMode)
		if err != nil {
			if errors.Is(err, matview.ErrRegistrationNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "matview registration not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update matview")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, reg)
	}
}

func handleAdminDeleteMatview(svc matviewAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid matview id format")
			return
		}

		err := svc.Delete(r.Context(), id)
		if err != nil {
			if errors.Is(err, matview.ErrRegistrationNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "matview registration not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to delete matview")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleAdminRefreshMatview(svc matviewAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid matview id format")
			return
		}

		result, err := svc.RefreshNow(r.Context(), id)
		if err != nil {
			if errors.Is(err, matview.ErrRegistrationNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "matview registration not found")
				return
			}
			if errors.Is(err, matview.ErrRefreshInProgress) {
				httputil.WriteError(w, http.StatusConflict, "refresh already in progress")
				return
			}
			if errors.Is(err, matview.ErrConcurrentRefreshRequiresIndex) {
				httputil.WriteError(w, http.StatusConflict, "concurrent refresh requires a unique index on the materialized view")
				return
			}
			if errors.Is(err, matview.ErrConcurrentRefreshRequiresPopulated) {
				httputil.WriteError(w, http.StatusConflict, "concurrent refresh requires a populated materialized view")
				return
			}
			if errors.Is(err, matview.ErrNotMaterializedView) {
				httputil.WriteError(w, http.StatusNotFound, "materialized view no longer exists in database")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "refresh failed")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, result)
	}
}
