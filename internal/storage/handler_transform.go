package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/imaging"
)

func hasTransformParams(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("w") != "" || q.Get("h") != "" || getFormatQuery(q) != "" || q.Get("q") != "" || q.Get("crop") != ""
}

// serveTransformed decodes, transforms, and serves an image with the requested parameters.
func (h *Handler) serveTransformed(w http.ResponseWriter, r *http.Request, reader io.ReadCloser, obj *Object, isPublic bool) {
	q := r.URL.Query()

	// Animated GIF passthrough: if source is GIF and animated, serve original unchanged.
	if strings.HasPrefix(obj.ContentType, "image/gif") {
		data, err := io.ReadAll(reader)
		if err != nil {
			h.logger.Error("reading GIF data", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		animated, err := imaging.IsAnimatedGIF(bytes.NewReader(data))
		if err == nil && animated {
			w.Header().Set("Content-Type", "image/gif")
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Header().Set("Cache-Control", cacheControlTransformed(isPublic))
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
		// Not animated (or decode error) — fall through to transform.
		// GIF is not a supported transform source format, so FormatFromContentType will reject it.
	}

	// Verify source is a supported image format.
	srcFormat, ok := imaging.FormatFromContentType(obj.ContentType)
	if !ok {
		httputil.WriteError(w, http.StatusBadRequest, "file is not a supported image format (jpeg, png, webp)")
		return
	}

	opts, err := parseTransformOptions(q, srcFormat)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Compute deterministic ETag before transform (based on object identity + transform params).
	etag := computeTransformETag(obj, r.URL.RawQuery)
	if ifNoneMatchMatches(r.Header.Get("If-None-Match"), etag) {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
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
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", cacheControlTransformed(isPublic))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, &buf)
}

// computeTransformETag generates a deterministic ETag from the object identity and
// sorted transform-relevant query parameters. Changes when the source file changes
// or the transform parameters change. Non-transform params (sig, exp) are excluded
// so signed URL regeneration doesn't invalidate the ETag.
func computeTransformETag(obj *Object, rawQuery string) string {
	h := sha256.New()
	hashObjectVersion(h, obj)
	_, _ = h.Write([]byte(":"))
	_, _ = h.Write([]byte(transformQueryString(rawQuery)))
	return `"` + hex.EncodeToString(h.Sum(nil))[:16] + `"`
}

// transformQueryString extracts only transform-relevant query params, sorts them,
// and returns a canonical representation for ETag hashing.
func transformQueryString(raw string) string {
	q, _ := url.ParseQuery(raw)

	canonical := make([]string, 0, 6)
	if v := getQuery(q, "w"); v != "" {
		canonical = append(canonical, "w="+url.QueryEscape(v))
	}
	if v := getQuery(q, "h"); v != "" {
		canonical = append(canonical, "h="+url.QueryEscape(v))
	}
	if v := getFormatQuery(q); v != "" {
		canonical = append(canonical, "fmt="+url.QueryEscape(v))
	}
	if v := getQuery(q, "q"); v != "" {
		canonical = append(canonical, "q="+url.QueryEscape(v))
	}
	if v := getQuery(q, "fit"); v != "" {
		canonical = append(canonical, "fit="+url.QueryEscape(v))
	}
	if v := getQuery(q, "crop"); v != "" {
		canonical = append(canonical, "crop="+url.QueryEscape(v))
	}
	sort.Strings(canonical)
	return strings.Join(canonical, "&")
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

	if format := getFormatQuery(q); format != "" {
		f, ok := imaging.ParseFormat(format)
		if !ok {
			return opts, errors.New("unsupported format (use jpeg, png, webp, or avif)")
		}
		opts.Format = f
	} else {
		opts.Format = srcFormat
	}

	if cropStr := getQuery(q, "crop"); cropStr != "" {
		crop, ok := imaging.ParseCropMode(cropStr)
		if !ok {
			return opts, errors.New("unsupported crop mode (use center or smart)")
		}
		opts.Crop = crop
	}

	return opts, nil
}

func cacheControlRaw(isPublic bool) string {
	if isPublic {
		return "public, max-age=31536000, immutable"
	}
	return "private, no-cache"
}

func cacheControlTransformed(isPublic bool) string {
	if isPublic {
		return "public, max-age=86400"
	}
	return "private, no-cache"
}
