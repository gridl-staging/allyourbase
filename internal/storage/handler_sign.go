package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

func getQuery(q map[string][]string, key string) string {
	if vals, ok := q[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func getFormatQuery(q map[string][]string) string {
	if v := getQuery(q, "fmt"); v != "" {
		return v
	}
	return getQuery(q, "format")
}

func (h *Handler) publicObjectURL(r *http.Request, bucket, name string) string {
	u := url.URL{
		Scheme: requestScheme(r),
		Host:   r.Host,
		Path:   "/api/storage/" + url.PathEscape(bucket) + "/" + escapeObjectPath(name),
	}
	return u.String()
}

func requestScheme(r *http.Request) string {
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func escapeObjectPath(name string) string {
	parts := strings.Split(name, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func (h *Handler) publicObjectResponseURL(r *http.Request, obj Object, isPublic bool) string {
	if !isPublic {
		return ""
	}
	return RewritePublicURL(h.publicObjectURL(r, obj.Bucket, obj.Name), h.cdnURL)
}

type signRequest struct {
	ExpiresIn int `json:"expiresIn"` // seconds, default 3600
}

type signResponse struct {
	URL string `json:"url"`
}

// HandleSign generates a signed URL for a file with a time-limited expiry. The expiry duration defaults to one hour and must not exceed seven days.
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
	url := signedObjectPath(bucket, name, token)
	httputil.WriteJSON(w, http.StatusOK, signResponse{URL: url})
}

func signedObjectPath(bucket, name, token string) string {
	return "/api/storage/" + url.PathEscape(bucket) + "/" + escapeObjectPath(name) + "?" + token
}

func hashObjectVersion(h interface{ Write([]byte) (int, error) }, obj *Object) {
	_, _ = h.Write([]byte(obj.ID))
	_, _ = h.Write([]byte(":"))
	_, _ = h.Write([]byte(strconv.FormatInt(obj.UpdatedAt.UTC().UnixNano(), 10)))
}

func computeObjectETag(obj *Object) string {
	h := sha256.New()
	hashObjectVersion(h, obj)
	_, _ = h.Write([]byte(":"))
	_, _ = h.Write([]byte(strconv.FormatInt(obj.Size, 10)))
	return `"` + hex.EncodeToString(h.Sum(nil))[:16] + `"`
}

// ifNoneMatchMatches returns true if any ETag token in an If-None-Match header matches the provided ETag, supporting both strong and weak ETag formats. Returns false if either the header or ETag is empty, or if no tokens match.
func ifNoneMatchMatches(headerValue, etag string) bool {
	if headerValue == "" || etag == "" {
		return false
	}
	for _, token := range strings.Split(headerValue, ",") {
		candidate := strings.TrimSpace(token)
		if candidate == "" {
			continue
		}
		if candidate == "*" || candidate == etag {
			return true
		}
		if strings.HasPrefix(candidate, "W/") && strings.TrimSpace(strings.TrimPrefix(candidate, "W/")) == etag {
			return true
		}
	}
	return false
}

func applyConditionalRawETag(w http.ResponseWriter, r *http.Request, obj *Object) bool {
	etag := computeObjectETag(obj)
	w.Header().Set("ETag", etag)
	if ifNoneMatchMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}
