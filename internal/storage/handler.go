package storage

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/imaging"
	"github.com/go-chi/chi/v5"
)

// Handler serves storage HTTP endpoints.
type Handler struct {
	svc         *Service
	logger      *slog.Logger
	maxFileSize int64
}

// NewHandler creates a new storage handler.
func NewHandler(svc *Service, logger *slog.Logger, maxFileSize int64) *Handler {
	return &Handler{
		svc:         svc,
		logger:      logger,
		maxFileSize: maxFileSize,
	}
}

// Routes returns a chi.Router with storage endpoints mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{bucket}", h.HandleList)
	r.Post("/{bucket}", h.HandleUpload)
	r.Get("/{bucket}/*", h.HandleServe)
	r.Delete("/{bucket}/*", h.HandleDelete)
	r.Post("/{bucket}/{name}/sign", h.HandleSign)
	return r
}

type listResponse struct {
	Items      []Object `json:"items"`
	TotalItems int      `json:"totalItems"`
}

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
		h.logger.Error("list error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if objects == nil {
		objects = []Object{}
	}
	httputil.WriteJSON(w, http.StatusOK, listResponse{Items: objects, TotalItems: total})
}

func (h *Handler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")

	// Limit request body size.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize)

	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid multipart form or file too large",
			"https://allyourbase.io/guide/file-storage")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "missing \"file\" field in multipart form",
			"https://allyourbase.io/guide/file-storage")
		return
	}
	defer file.Close()

	// Use provided name or fall back to uploaded filename.
	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}
	if name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "file name is required")
		return
	}

	// Detect content type from extension, fall back to header.
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

	obj, err := h.svc.Upload(r.Context(), bucket, name, contentType, userID, file)
	if err != nil {
		if errors.Is(err, ErrInvalidBucket) || errors.Is(err, ErrInvalidName) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error("upload error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, obj)
}

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
		h.serveFile(w, r, bucket, name)
		return
	}

	h.serveFile(w, r, bucket, name)
}

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, bucket, name string) {
	reader, obj, err := h.svc.Download(r.Context(), bucket, name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		h.logger.Error("download error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer reader.Close()

	// If image transform query params are present, process and serve transformed image.
	if hasTransformParams(r) {
		h.serveTransformed(w, r, reader, obj)
		return
	}

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

// hasTransformParams returns true if the request contains image transform query parameters.
func hasTransformParams(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("w") != "" || q.Get("h") != "" || q.Get("fmt") != "" || q.Get("q") != ""
}

// serveTransformed decodes, transforms, and serves an image with the requested parameters.
func (h *Handler) serveTransformed(w http.ResponseWriter, r *http.Request, reader io.ReadCloser, obj *Object) {
	q := r.URL.Query()

	// Verify source is a supported image format.
	srcFormat, ok := imaging.FormatFromContentType(obj.ContentType)
	if !ok {
		httputil.WriteError(w, http.StatusBadRequest, "file is not a supported image format (jpeg, png)")
		return
	}

	opts, err := parseTransformOptions(q, srcFormat)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var buf bytes.Buffer
	if err := imaging.Transform(reader, &buf, opts); err != nil {
		h.logger.Error("image transform error", "bucket", obj.Bucket, "name", obj.Name, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "image processing failed")
		return
	}

	w.Header().Set("Content-Type", opts.Format.ContentType())
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, &buf)
}

// parseTransformOptions parses image transform query parameters into imaging.Options.
func parseTransformOptions(q map[string][]string, srcFormat imaging.Format) (imaging.Options, error) {
	var opts imaging.Options

	if ws := getQuery(q, "w"); ws != "" {
		w, err := strconv.Atoi(ws)
		if err != nil || w < 0 {
			return opts, errors.New("invalid width parameter")
		}
		opts.Width = w
	}
	if hs := getQuery(q, "h"); hs != "" {
		h, err := strconv.Atoi(hs)
		if err != nil || h < 0 {
			return opts, errors.New("invalid height parameter")
		}
		opts.Height = h
	}
	if opts.Width == 0 && opts.Height == 0 {
		return opts, errors.New("w or h parameter is required for image transforms")
	}

	if fit := getQuery(q, "fit"); fit != "" {
		opts.Fit = imaging.ParseFit(fit)
	}

	if qs := getQuery(q, "q"); qs != "" {
		quality, err := strconv.Atoi(qs)
		if err != nil || quality < 1 || quality > 100 {
			return opts, errors.New("quality must be 1-100")
		}
		opts.Quality = quality
	}

	if fmts := getQuery(q, "fmt"); fmts != "" {
		f, ok := imaging.ParseFormat(fmts)
		if !ok {
			return opts, errors.New("unsupported format (use jpeg or png)")
		}
		opts.Format = f
	} else {
		opts.Format = srcFormat
	}

	return opts, nil
}

func getQuery(q map[string][]string, key string) string {
	if vals, ok := q[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	name := chi.URLParam(r, "*")

	if err := h.svc.DeleteObject(r.Context(), bucket, name); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		h.logger.Error("delete error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type signRequest struct {
	ExpiresIn int `json:"expiresIn"` // seconds, default 3600
}

type signResponse struct {
	URL string `json:"url"`
}

func (h *Handler) HandleSign(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	name := chi.URLParam(r, "name")

	var req signRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	expiry := time.Duration(req.ExpiresIn) * time.Second
	if expiry <= 0 {
		expiry = time.Hour
	}
	if expiry > 7*24*time.Hour {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "expiresIn must not exceed 604800 (7 days)",
			"https://allyourbase.io/guide/file-storage")
		return
	}

	// Verify object exists.
	if _, err := h.svc.GetObject(r.Context(), bucket, name); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		h.logger.Error("sign error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	token := h.svc.SignURL(bucket, name, expiry)
	url := "/api/storage/" + bucket + "/" + name + "?" + token
	httputil.WriteJSON(w, http.StatusOK, signResponse{URL: url})
}
