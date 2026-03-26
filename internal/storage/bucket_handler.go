// Package storage provides HTTP request handlers for managing storage buckets including create, list, update, and delete operations. It includes error mapping to translate service layer errors into appropriate HTTP response codes.
package storage

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type bucketCreateRequest struct {
	Name   string `json:"name"`
	Public bool   `json:"public"`
}

type bucketUpdateRequest struct {
	Public bool `json:"public"`
}

type bucketListResponse struct {
	Items []Bucket `json:"items"`
}

// HandleBucketCreate decodes and validates a bucket creation request from HTTP POST, creates the bucket via the storage service, and writes the result as JSON with 201 status or an error response.
func (h *Handler) HandleBucketCreate(w http.ResponseWriter, r *http.Request) {
	var req bucketCreateRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}

	bucket, err := h.svc.CreateBucket(r.Context(), req.Name, req.Public)
	if err != nil {
		h.writeBucketError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, bucket)
}

func (h *Handler) HandleBucketList(w http.ResponseWriter, r *http.Request) {
	buckets, err := h.svc.ListBuckets(r.Context())
	if err != nil {
		h.logger.Error("list buckets failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if buckets == nil {
		buckets = []Bucket{}
	}
	httputil.WriteJSON(w, http.StatusOK, bucketListResponse{Items: buckets})
}

// HandleBucketUpdate decodes and validates a bucket update request from HTTP PATCH, updates the bucket via the storage service, and writes the result as JSON or an error response.
func (h *Handler) HandleBucketUpdate(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req bucketUpdateRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	bucket, err := h.svc.UpdateBucket(r.Context(), name, req.Public)
	if err != nil {
		h.writeBucketError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, bucket)
}

// HandleBucketDelete parses the bucket name and optional force parameter from the HTTP DELETE request, deletes the bucket via the storage service, and writes 204 No Content or an error response.
func (h *Handler) HandleBucketDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	force := false
	if forceParam := r.URL.Query().Get("force"); forceParam != "" {
		var err error
		force, err = strconv.ParseBool(forceParam)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid force value")
			return
		}
	}

	if err := h.svc.DeleteBucket(r.Context(), name, force); err != nil {
		h.writeBucketError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeBucketError maps bucket service errors to HTTP response codes: 400 for invalid names, 404 for not found, 409 for conflicts including duplicates and non-empty buckets, and 500 for unexpected errors.
func (h *Handler) writeBucketError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidBucket):
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, ErrAlreadyExists):
		httputil.WriteError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, ErrBucketNotFound):
		httputil.WriteError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrBucketNotEmpty):
		httputil.WriteError(w, http.StatusConflict, "bucket has objects; use force=true to delete")
		return
	}

	h.logger.Error("bucket operation failed", "error", err)
	httputil.WriteError(w, http.StatusInternalServerError, "internal error")
}
