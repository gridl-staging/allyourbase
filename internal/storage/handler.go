// Package storage Handler serves HTTP endpoints for file storage operations including upload, download, deletion, signing, and listing, with support for resumable uploads via TUS protocol, image transformations, and tenant quota enforcement.
package storage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

// Handler serves storage HTTP endpoints.
type Handler struct {
	svc           *Service
	isAdmin       func(*http.Request) bool
	logger        *slog.Logger
	maxFileSize   int64
	cdnURL        string
	uploadTimeout time.Duration

	mutations           handlerMutations
	cdnPurgeCoordinator *cdnPurgeCoordinator

	tenantQuotaReader      tenantQuotaReader
	tenantQuotaChecker     tenant.QuotaChecker
	tenantUsageAccumulator *tenant.UsageAccumulator
	tenantQuotaMetrics     tenantQuotaMetricsRecorder
	tenantQuotaAudit       tenantQuotaAuditEmitter
}

const (
	headerTenantQuotaWarning = "X-Tenant-Quota-Warning"

	tusResumableVersion     = "1.0.0"
	tusResumableExtension   = "creation"
	tusResumableHeader      = "Tus-Resumable"
	tusVersionHeader        = "Tus-Version"
	tusExtensionHeader      = "Tus-Extension"
	tusMaxSizeHeader        = "Tus-Max-Size"
	tusUploadLengthHeader   = "Upload-Length"
	tusUploadOffsetHeader   = "Upload-Offset"
	tusUploadMetadataHeader = "Upload-Metadata"
	tusOffsetContentType    = "application/offset+octet-stream"

	defaultUploadTimeout = 5 * time.Minute
)

// NewHandler creates a new storage handler.
func NewHandler(svc *Service, logger *slog.Logger, maxFileSize int64, cdnURL string, isAdmin ...func(*http.Request) bool) *Handler {
	var isAdminFn func(*http.Request) bool
	if len(isAdmin) > 0 {
		isAdminFn = isAdmin[0]
	}
	return &Handler{
		svc:           svc,
		isAdmin:       isAdminFn,
		logger:        logger,
		maxFileSize:   maxFileSize,
		cdnURL:        strings.TrimSpace(cdnURL),
		uploadTimeout: defaultUploadTimeout,
		mutations:     newHandlerMutations(svc),
		cdnPurgeCoordinator: newCDNPurgeCoordinator(
			NopCDNProvider{},
			logger,
			defaultCDNPurgeTimeout,
		),
	}
}

// SetUploadTimeout overrides the server-side upload timeout used for backend
// writes. A non-positive value disables the handler-level timeout.
func (h *Handler) SetUploadTimeout(d time.Duration) {
	h.uploadTimeout = d
}

// Routes returns a chi.Router with storage endpoints mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Route("/upload/resumable", func(r chi.Router) {
		r.Options("/", h.HandleResumableOptions)
		r.Post("/", h.HandleResumableCreate)
		r.Head("/{id}", h.HandleResumableHead)
		r.Patch("/{id}", h.HandleResumablePatch)
	})
	r.Get("/{bucket}", h.HandleList)
	r.Post("/{bucket}", h.HandleUpload)
	r.Get("/{bucket}/*", h.HandleServe)
	r.Delete("/{bucket}/*", h.HandleDelete)
	r.Post("/{bucket}/{name}/sign", h.HandleSign)
	return r
}

type listResponse struct {
	Items      []listItemResponse `json:"items"`
	TotalItems int                `json:"totalItems"`
}

type listItemResponse struct {
	Object
	URL string `json:"url,omitempty"`
}

type uploadResponse struct {
	Object
	URL string `json:"url,omitempty"`
}

type uploadRequestInput struct {
	bucket      string
	name        string
	contentType string
	userID      *string
	trackedUser bool
	file        multipart.File
	size        int64
}

// HandleList returns a paginated list of objects in a bucket with optional prefix filtering. Each item includes a public URL if the bucket is public, or an empty URL if access is restricted.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	prefix := r.URL.Query().Get("prefix")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	objects, total, err := h.svc.ListObjects(r.Context(), bucket, prefix, limit, offset)
	if err != nil {
		if errors.Is(err, ErrInvalidBucket) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, ErrPermissionDenied) {
			httputil.WriteError(w, http.StatusForbidden, "insufficient storage permissions")
			return
		}
		h.logger.Error("list error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if objects == nil {
		objects = []Object{}
	}

	isPublic, err := h.isBucketPublic(r.Context(), bucket)
	if err != nil {
		h.logger.Error("checking bucket access", "error", err)
		isPublic = false
	}

	items := make([]listItemResponse, 0, len(objects))
	for _, obj := range objects {
		item := listItemResponse{Object: obj}
		item.URL = h.publicObjectResponseURL(r, obj, isPublic)
		items = append(items, item)
	}
	httputil.WriteJSON(w, http.StatusOK, listResponse{Items: items, TotalItems: total})
}

// TODO: Document Handler.HandleUpload.
func (h *Handler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	input, ok := h.parseUploadRequest(w, r)
	if !ok {
		return
	}
	defer input.file.Close()

	tenantID := tenant.TenantFromContext(r.Context())
	if tenantID != "" {
		softWarning, currentUsage, limit, err := h.applyTenantQuotaChecks(r.Context(), tenantID, input.size)
		if err != nil {
			if errors.Is(err, ErrQuotaExceeded) {
				h.emitTenantStorageQuotaViolation(r, tenantID, currentUsage, limit)
				httputil.WriteError(w, http.StatusRequestEntityTooLarge, "tenant storage quota exceeded")
			} else {
				h.logger.Error("tenant storage quota check failed", "tenant_id", tenantID, "error", err)
				httputil.WriteError(w, http.StatusInternalServerError, "tenant storage quota check is temporarily unavailable")
			}
			return
		}
		if softWarning {
			w.Header().Set(headerTenantQuotaWarning, "storage")
		}
	}

	// Reserve usage atomically before upload so concurrent requests cannot
	// oversubscribe quota and later race on accounting writes.
	reservedBytes := int64(0)
	if input.trackedUser {
		if err := h.mutations.reserveQuota(r.Context(), *input.userID, input.size); err != nil {
			if errors.Is(err, ErrQuotaExceeded) {
				httputil.WriteError(w, http.StatusRequestEntityTooLarge, "storage quota exceeded")
				return
			}
			h.logger.Error("quota reservation error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		reservedBytes = input.size
	}

	obj, err, timedOut := h.uploadWithTimeout(
		r.Context(),
		input.bucket,
		input.name,
		input.contentType,
		input.userID,
		input.file,
	)
	if err != nil {
		h.rollbackReservedQuota(r.Context(), input.userID, reservedBytes, input.trackedUser)
		if timedOut {
			h.logger.Warn("upload timed out", "timeout", h.uploadTimeout, "bucket", input.bucket, "name", input.name)
			httputil.WriteError(w, http.StatusGatewayTimeout, "upload timed out")
			return
		}
		if errors.Is(err, ErrInvalidBucket) || errors.Is(err, ErrInvalidName) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, ErrPermissionDenied) {
			httputil.WriteError(w, http.StatusForbidden, "insufficient storage permissions")
			return
		}
		h.logger.Error("upload error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if tenantID != "" && h.tenantUsageAccumulator != nil {
		h.tenantUsageAccumulator.Record(tenantID, tenant.ResourceTypeDBSizeBytes, obj.Size)
	}

	isPublic, publicErr := h.isBucketPublic(r.Context(), input.bucket)
	if publicErr != nil {
		h.logger.Error("checking bucket access", "error", publicErr)
		isPublic = false
	}
	if objectWasOverwritten(obj) {
		h.enqueueObjectURLPurge(r, obj.Bucket, obj.Name)
	}

	resp := uploadResponse{Object: *obj}
	resp.URL = h.publicObjectResponseURL(r, *obj, isPublic)
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) parseUploadRequest(w http.ResponseWriter, r *http.Request) (*uploadRequestInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize)
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid multipart form or file too large",
			"https://allyourbase.io/guide/file-storage")
		return nil, false
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "missing \"file\" field in multipart form",
			"https://allyourbase.io/guide/file-storage")
		return nil, false
	}

	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}
	if name == "" {
		file.Close()
		httputil.WriteError(w, http.StatusBadRequest, "file name is required")
		return nil, false
	}

	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = header.Header.Get("Content-Type")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var userID *string
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		userID = &claims.Subject
	}

	return &uploadRequestInput{
		bucket:      chi.URLParam(r, "bucket"),
		name:        name,
		contentType: contentType,
		userID:      userID,
		trackedUser: userID != nil && !h.isAdminToken(r),
		file:        file,
		size:        header.Size,
	}, true
}

func (h *Handler) uploadWithTimeout(
	requestCtx context.Context,
	bucket, name, contentType string,
	userID *string,
	file io.Reader,
) (*Object, error, bool) {
	uploadCtx := requestCtx
	cancel := func() {}
	if h.uploadTimeout > 0 {
		uploadCtx, cancel = context.WithTimeout(requestCtx, h.uploadTimeout)
	}
	defer cancel()

	obj, err := h.mutations.upload(uploadCtx, bucket, name, contentType, userID, file)
	timedOut := err != nil &&
		errors.Is(err, context.DeadlineExceeded) &&
		requestCtx.Err() == nil &&
		uploadCtx.Err() == context.DeadlineExceeded
	return obj, err, timedOut
}

func (h *Handler) rollbackReservedQuota(
	ctx context.Context,
	userID *string,
	reservedBytes int64,
	trackedUser bool,
) {
	if !trackedUser || reservedBytes <= 0 || userID == nil {
		return
	}
	if rollbackErr := h.mutations.decrementUsage(ctx, *userID, reservedBytes); rollbackErr != nil {
		h.logger.Error("quota rollback error", "error", rollbackErr)
	}
}

// HandleServe serves a file from storage, first checking for a valid signed URL signature. If present and valid, the file is served without authentication. Otherwise, the bucket's public status is checked; private buckets require authentication while public buckets are accessible to all.
func (h *Handler) HandleServe(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	name := chi.URLParam(r, "*")

	// Check for signed URL params.
	if sig := r.URL.Query().Get("sig"); sig != "" {
		exp := r.URL.Query().Get("exp")
		if !h.svc.ValidateSignedURL(bucket, name, exp, sig) {
			httputil.WriteErrorWithDocURL(w, http.StatusForbidden, "invalid or expired signed URL",
				"https://allyourbase.io/guide/file-storage")
			return
		}
		// Signed URL is valid — serve the file without further auth checks.
		// Treat signed URLs as private to avoid cache leakage.
		h.serveFile(w, r, bucket, name, false)
		return
	}

	isPublic, err := h.isBucketPublic(r.Context(), bucket)
	if err != nil {
		h.logger.Error("checking bucket access", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !isPublic {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil && !h.isAdminToken(r) {
			httputil.WriteError(w, http.StatusUnauthorized, "missing auth token")
			return
		}
	}

	h.serveFile(w, r, bucket, name, isPublic)
}

// isBucketPublic determines whether a bucket allows public access. Without a database pool it returns true for backward compatibility. If a bucket has no metadata record, it is treated as implicitly public.
func (h *Handler) isBucketPublic(ctx context.Context, bucket string) (bool, error) {
	// Without a DB pool, preserve backward compatibility by allowing access
	// and keeping historical behavior (public by default).
	if h.svc.pool == nil {
		return true, nil
	}

	b, err := h.svc.GetBucket(ctx, bucket)
	if err != nil {
		if errors.Is(err, ErrBucketNotFound) {
			// Buckets without metadata are treated as implicitly public.
			return true, nil
		}
		return false, err
	}

	return b.Public, nil
}

func (h *Handler) isAdminToken(r *http.Request) bool {
	if h.isAdmin == nil {
		return false
	}
	return h.isAdmin(r)
}

// callerUserID returns the authenticated user's ID for ownership checks.
// Returns nil for admin requests (bypasses ownership) or unauthenticated requests.
func (h *Handler) callerUserID(r *http.Request) *string {
	if h.isAdminToken(r) {
		return nil // admin bypass
	}
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		return &claims.Subject
	}
	return nil
}

// serveFile downloads a file from storage and streams it to the response writer with appropriate cache headers and ETag validation. If the request contains image transform parameters, the image is processed and served in the requested format; otherwise the raw file is served as-is.
func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, bucket, name string, isPublic bool) {
	reader, obj, err := h.svc.Download(r.Context(), bucket, name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		if errors.Is(err, ErrPermissionDenied) {
			httputil.WriteError(w, http.StatusForbidden, "insufficient storage permissions")
			return
		}
		h.logger.Error("download error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer reader.Close()

	// If image transform query params are present, process and serve transformed image.
	if hasTransformParams(r) {
		h.serveTransformed(w, r, reader, obj, isPublic)
		return
	}

	if applyConditionalRawETag(w, r, obj) {
		return
	}

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	w.Header().Set("Cache-Control", cacheControlRaw(isPublic))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

// hasTransformParams returns true if the request contains image transform query parameters.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	name := chi.URLParam(r, "*")
	tenantID := tenant.TenantFromContext(r.Context())

	// Fetch object metadata before deletion for usage accounting.
	obj, err := h.mutations.getObject(r.Context(), bucket, name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		if errors.Is(err, ErrPermissionDenied) {
			httputil.WriteError(w, http.StatusForbidden, "insufficient storage permissions")
			return
		}
		h.logger.Error("get object for delete error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.mutations.deleteObject(r.Context(), bucket, name); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		if errors.Is(err, ErrPermissionDenied) {
			httputil.WriteError(w, http.StatusForbidden, "insufficient storage permissions")
			return
		}
		h.logger.Error("delete error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Reclaim quota after successful deletion.
	if obj.UserID != nil {
		if err := h.mutations.decrementUsage(r.Context(), *obj.UserID, obj.Size); err != nil {
			h.logger.Error("decrement usage after delete", "error", err)
			// Non-fatal: file deleted successfully; log the accounting failure and continue.
		}
	}
	if tenantID != "" && h.tenantUsageAccumulator != nil {
		h.tenantUsageAccumulator.Record(tenantID, tenant.ResourceTypeDBSizeBytes, -obj.Size)
	}
	h.enqueueObjectURLPurge(r, bucket, name)

	w.WriteHeader(http.StatusNoContent)
}
