package storage

import (
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type resumableCreateResponse struct {
	ID         string `json:"id"`
	Bucket     string `json:"bucket"`
	Name       string `json:"name"`
	TotalSize  int64  `json:"totalSize"`
	Status     string `json:"status"`
	ExpiresAt  int64  `json:"expiresAt"`
	UploadType string `json:"uploadType"`
}

// HandleResumableCreate creates a new TUS upload session.
func (h *Handler) HandleResumableCreate(w http.ResponseWriter, r *http.Request) {
	if !requireTusVersion(w, r) {
		return
	}

	length, err := parseTusIntHeader(w, r, tusUploadLengthHeader)
	if err != nil {
		return
	}
	if length <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Upload-Length must be greater than 0")
		return
	}

	bucket := r.URL.Query().Get("bucket")
	name := r.URL.Query().Get("name")
	metadata := parseTusMetadata(r.Header.Get(tusUploadMetadataHeader))
	if name == "" {
		name = metadata["name"]
	}
	if bucket == "" || name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "bucket and name query params are required")
		return
	}

	contentType := metadata["contentType"]
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if length > h.maxFileSize {
		httputil.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds maximum file size")
		return
	}

	var userID *string
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		userID = &claims.Subject
	}

	// Enforce quota upfront based on declared Upload-Length (skip for admin and anonymous).
	if userID != nil && !h.isAdminToken(r) {
		if err := h.svc.CheckQuota(r.Context(), *userID, length); err != nil {
			if errors.Is(err, ErrQuotaExceeded) {
				httputil.WriteError(w, http.StatusRequestEntityTooLarge, "storage quota exceeded")
				return
			}
			h.logger.Error("quota check error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	upload, err := h.svc.CreateResumableUpload(r.Context(), bucket, name, contentType, userID, length)
	if err != nil {
		if errors.Is(err, ErrInvalidBucket) || errors.Is(err, ErrInvalidName) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error("create resumable upload error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	location := "/api/storage/upload/resumable/" + upload.ID
	w.Header().Set("Location", location)
	setTusHeaders(w)
	httputil.WriteJSON(w, http.StatusCreated, resumableCreateResponse{
		ID:         upload.ID,
		Bucket:     upload.Bucket,
		Name:       upload.Name,
		TotalSize:  upload.TotalSize,
		Status:     upload.Status,
		ExpiresAt:  upload.ExpiresAt.Unix(),
		UploadType: tusResumableVersion,
	})
}

// HandleResumablePatch appends chunk data to an active resumable upload session.
func (h *Handler) HandleResumablePatch(w http.ResponseWriter, r *http.Request) {
	if !requireTusVersion(w, r) {
		return
	}

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != tusOffsetContentType {
		httputil.WriteError(w, http.StatusBadRequest, "Content-Type must be application/offset+octet-stream")
		return
	}

	offset, err := parseTusIntHeader(w, r, tusUploadOffsetHeader)
	if err != nil {
		return
	}

	callerUserID := h.callerUserID(r)
	id := chi.URLParam(r, "id")
	upload, finalize, err := h.mutations.appendResumableUpload(r.Context(), id, offset, callerUserID, r.Body)
	if err != nil {
		serveResumableError(w, err)
		return
	}

	if finalize {
		obj, err := h.mutations.finalizeResumableUpload(r.Context(), upload.ID, callerUserID)
		if err != nil {
			serveResumableError(w, err)
			return
		}
		// Track usage after successful finalization.
		if upload.UserID != nil {
			if err := h.mutations.incrementUsage(r.Context(), *upload.UserID, upload.TotalSize); err != nil {
				h.logger.Error("increment usage after TUS finalize", "error", err)
				// Non-fatal: upload succeeded; log the accounting failure and continue.
			}
		}
		if objectWasOverwritten(obj) {
			h.enqueueObjectURLPurge(r, obj.Bucket, obj.Name)
		}
	}

	w.Header().Set(tusUploadOffsetHeader, strconv.FormatInt(upload.UploadedSize, 10))
	w.Header().Set(tusUploadLengthHeader, strconv.FormatInt(upload.TotalSize, 10))
	setTusHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

// HandleResumableHead returns resumable upload metadata for a session.
func (h *Handler) HandleResumableHead(w http.ResponseWriter, r *http.Request) {
	if !requireTusVersion(w, r) {
		return
	}

	id := chi.URLParam(r, "id")
	upload, err := h.svc.GetResumableUpload(r.Context(), id, h.callerUserID(r))
	if err != nil {
		serveResumableError(w, err)
		return
	}

	w.Header().Set(tusUploadOffsetHeader, strconv.FormatInt(upload.UploadedSize, 10))
	w.Header().Set(tusUploadLengthHeader, strconv.FormatInt(upload.TotalSize, 10))
	setTusHeaders(w)
	w.WriteHeader(http.StatusOK)
}

// HandleResumableOptions exposes TUS protocol metadata for resumable endpoints.
func (h *Handler) HandleResumableOptions(w http.ResponseWriter, r *http.Request) {
	setTusHeaders(w)
	w.Header().Set(tusVersionHeader, tusResumableVersion)
	w.Header().Set(tusExtensionHeader, tusResumableExtension)
	w.Header().Set(tusMaxSizeHeader, strconv.FormatInt(h.maxFileSize, 10))
	w.WriteHeader(http.StatusNoContent)
}

func setTusHeaders(w http.ResponseWriter) {
	w.Header().Set(tusResumableHeader, tusResumableVersion)
}

func requireTusVersion(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get(tusResumableHeader) != tusResumableVersion {
		httputil.WriteError(w, http.StatusPreconditionFailed, "invalid Tus-Resumable header")
		return false
	}
	return true
}

func parseTusIntHeader(w http.ResponseWriter, r *http.Request, key string) (int64, error) {
	raw := r.Header.Get(key)
	if raw == "" {
		httputil.WriteError(w, http.StatusBadRequest, key+" header is required")
		return 0, fmt.Errorf("%s required", key)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid "+key+" header")
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return value, nil
}

// serveResumableError converts TUS-specific upload errors to appropriate HTTP status codes, mapping ErrResumableUploadNotFound to 404, ErrResumableUploadForbidden to 403, ErrResumableUploadExpired to 410, ErrResumableUploadOffsetMismatch to 409, and validation errors to 400.
func serveResumableError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrResumableUploadNotFound):
		httputil.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrResumableUploadForbidden):
		httputil.WriteError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrResumableUploadExpired):
		httputil.WriteError(w, http.StatusGone, err.Error())
	case errors.Is(err, ErrResumableUploadOffsetMismatch):
		httputil.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrResumableUploadChunkTooLarge):
		httputil.WriteError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, ErrResumableUploadInvalidState):
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}

// parseTusMetadata decodes the TUS Upload-Metadata header, extracting comma-separated key-value pairs where values are base64-encoded. Returns an empty map if the header is absent or contains no valid pairs.
func parseTusMetadata(raw string) map[string]string {
	if raw == "" {
		return map[string]string{}
	}
	pairs := strings.Split(raw, ",")
	out := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		parts := strings.SplitN(pair, " ", 2)
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		key := parts[0]
		if len(parts) == 1 {
			out[key] = "" // Key-only (no value per TUS spec).
			continue
		}
		value, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			continue
		}
		out[key] = string(value)
	}
	return out
}
