package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/imaging"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// fakeBackend is a simple in-memory Backend for handler tests.
type fakeBackend struct {
	files map[string][]byte
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{files: make(map[string][]byte)}
}

func (f *fakeBackend) Put(_ context.Context, bucket, name string, r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	f.files[bucket+"/"+name] = data
	return int64(len(data)), nil
}

func (f *fakeBackend) Get(_ context.Context, bucket, name string) (io.ReadCloser, error) {
	data, ok := f.files[bucket+"/"+name]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeBackend) Delete(_ context.Context, bucket, name string) error {
	delete(f.files, bucket+"/"+name)
	return nil
}

func (f *fakeBackend) Exists(_ context.Context, bucket, name string) (bool, error) {
	_, ok := f.files[bucket+"/"+name]
	return ok, nil
}

func newTestService() *Service {
	return &Service{backend: newFakeBackend(), signKey: []byte("test-key"), logger: testutil.DiscardLogger()}
}

// testRouter creates a chi router with the handler mounted, matching the server's mount pattern.
func testRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/storage", func(r chi.Router) {
		r.Mount("/", h.Routes())
	})
	return r
}

func TestHandleUploadMissingFile(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	router := testRouter(h)

	// Empty multipart form — no "file" field.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/storage/images", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusBadRequest, rec.Code)
	var errResp map[string]any
	testutil.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	testutil.Contains(t, errResp["message"].(string), `missing "file" field`)
}

func TestHandleUploadInvalidBucket(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	router := testRouter(h)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "test.txt")
	fw.Write([]byte("data"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/storage/INVALID", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusBadRequest, rec.Code)
	testutil.Contains(t, rec.Body.String(), "invalid bucket name")
}

func TestHandleUploadReturnsInternalErrorWhenMutationUploadFails(t *testing.T) {
	t.Parallel()

	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	h.mutations.upload = func(_ context.Context, _, _, _ string, _ *string, _ io.Reader) (*Object, error) {
		return nil, errors.New("storage backend unavailable")
	}
	router := testRouter(h)
	req := newUploadRequest(t, "/api/storage/images", "cat.jpg", []byte("image-bytes"))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusInternalServerError, rec.Code)
	testutil.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var errResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	testutil.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	testutil.Equal(t, http.StatusInternalServerError, errResp.Code)
	testutil.Equal(t, "internal error", errResp.Message)
}

func TestHandleUploadReturnsInternalErrorPromptlyWhenUploadContextEnds(t *testing.T) {
	t.Parallel()

	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	uploadStarted := make(chan struct{})
	h.mutations.upload = func(ctx context.Context, _, _, _ string, _ *string, _ io.Reader) (*Object, error) {
		close(uploadStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	router := testRouter(h)
	req := newUploadRequest(t, "/api/storage/images", "cat.jpg", []byte("image-bytes"))
	requestTimeout := 40 * time.Millisecond
	reqCtx, cancel := context.WithTimeout(req.Context(), requestTimeout)
	defer cancel()
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	start := time.Now()
	router.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	select {
	case <-uploadStarted:
	default:
		t.Fatal("expected upload mutation to be invoked")
	}

	testutil.Equal(t, http.StatusInternalServerError, rec.Code)
	testutil.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	maxCompletion := requestTimeout + 120*time.Millisecond
	testutil.True(t, elapsed < maxCompletion, "upload should return shortly after request context ends")

	var errResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	testutil.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	testutil.Equal(t, http.StatusInternalServerError, errResp.Code)
	testutil.Equal(t, "internal error", errResp.Message)
}

func TestPublicObjectResponseURLUsesCDN(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "https://cdn.example.com")
	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg", nil)
	req.Host = "api.example.com"
	obj := Object{Bucket: "images", Name: "photo.jpg"}
	got := h.publicObjectResponseURL(req, obj, true)
	testutil.Equal(t, "https://cdn.example.com/api/storage/images/photo.jpg", got)
}

func TestPublicObjectResponseURLEmptyCDNUsesOrigin(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg", nil)
	req.Host = "api.example.com"
	obj := Object{Bucket: "images", Name: "photo.jpg"}
	got := h.publicObjectResponseURL(req, obj, true)
	testutil.Equal(t, "http://api.example.com/api/storage/images/photo.jpg", got)
}

func TestPublicObjectResponseURLPrivateBucketEmpty(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "https://cdn.example.com")
	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg", nil)
	req.Host = "api.example.com"
	obj := Object{Bucket: "images", Name: "photo.jpg"}
	testutil.Equal(t, "", h.publicObjectResponseURL(req, obj, false))
}

func TestSignedObjectPathNotRewrittenForCDN(t *testing.T) {
	t.Parallel()
	got := signedObjectPath("images", "nested/photo.jpg", "sig=abc&exp=1")
	testutil.Equal(t, "/api/storage/images/nested/photo.jpg?sig=abc&exp=1", got)
	testutil.True(t, !strings.HasPrefix(got, "https://cdn.example.com"))
}

func TestHandleSignedURLInvalid(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	router := testRouter(h)

	// Request with invalid signature — rejected before hitting DB.
	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg?sig=invalid&exp=9999999999", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusForbidden, rec.Code)

	var resp map[string]any
	testutil.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	msg, ok := resp["message"].(string)
	testutil.True(t, ok, "response should contain a 'message' string field")
	testutil.Contains(t, msg, "invalid or expired signed URL")
}

func TestHandleSignedURLExpired(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger(), 10<<20, "")
	router := testRouter(h)

	// Generate a signed URL that already expired.
	token := svc.SignURL("images", "photo.jpg", -time.Second)
	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg?"+token, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusForbidden, rec.Code)
	testutil.Contains(t, rec.Body.String(), "invalid or expired signed URL")
}

func TestHandleUploadNoContentType(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	router := testRouter(h)

	// Non-multipart request body.
	req := httptest.NewRequest(http.MethodPost, "/api/storage/images", bytes.NewReader([]byte("not multipart")))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusBadRequest, rec.Code)
	testutil.Contains(t, rec.Body.String(), "invalid multipart form")
}

// Note: Tests that exercise full upload/serve/delete/list flows (which require
// database metadata operations) belong in integration tests with a real DB.
// See storage_integration_test.go (requires TEST_DATABASE_URL).

// --- Image transform tests ---

// makeHandlerTestJPEG creates a solid-color JPEG for handler tests.
func makeHandlerTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encoding test JPEG: %v", err)
	}
	return buf.Bytes()
}

func makeHandlerTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 50, G: 100, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding test PNG: %v", err)
	}
	return buf.Bytes()
}

func TestHasTransformParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"no params", "/api/storage/img/photo.jpg", false},
		{"width only", "/api/storage/img/photo.jpg?w=200", true},
		{"height only", "/api/storage/img/photo.jpg?h=150", true},
		{"format only", "/api/storage/img/photo.jpg?fmt=png", true},
		{"format long key only", "/api/storage/img/photo.jpg?format=png", true},
		{"quality only", "/api/storage/img/photo.jpg?q=50", true},
		{"width and height", "/api/storage/img/photo.jpg?w=200&h=150", true},
		{"all params", "/api/storage/img/photo.jpg?w=200&h=150&fit=cover&q=80&fmt=jpeg", true},
		{"unrelated params", "/api/storage/img/photo.jpg?sig=abc&exp=123", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			testutil.Equal(t, tc.want, hasTransformParams(req))
		})
	}
}

func TestParseTransformOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		query     string
		srcFormat imaging.Format
		wantW     int
		wantH     int
		wantFit   imaging.Fit
		wantQ     int
		wantFmt   imaging.Format
		wantErr   string
	}{
		{
			name:      "width only",
			query:     "w=300",
			srcFormat: imaging.FormatJPEG,
			wantW:     300, wantFmt: imaging.FormatJPEG,
		},
		{
			name:      "height only",
			query:     "h=200",
			srcFormat: imaging.FormatPNG,
			wantH:     200, wantFmt: imaging.FormatPNG,
		},
		{
			name:      "all params",
			query:     "w=400&h=300&fit=cover&q=90&fmt=png",
			srcFormat: imaging.FormatJPEG,
			wantW:     400, wantH: 300, wantFit: imaging.FitCover, wantQ: 90, wantFmt: imaging.FormatPNG,
		},
		{
			name:      "no dimensions",
			query:     "fmt=png",
			srcFormat: imaging.FormatJPEG,
			wantErr:   "w or h parameter is required",
		},
		{
			name:      "invalid width",
			query:     "w=abc",
			srcFormat: imaging.FormatJPEG,
			wantErr:   "invalid width",
		},
		{
			name:      "negative width",
			query:     "w=-5",
			srcFormat: imaging.FormatJPEG,
			wantErr:   "invalid width",
		},
		{
			name:      "invalid height",
			query:     "h=xyz",
			srcFormat: imaging.FormatJPEG,
			wantErr:   "invalid height",
		},
		{
			name:      "quality too low",
			query:     "w=100&q=0",
			srcFormat: imaging.FormatJPEG,
			wantErr:   "quality must be 1-100",
		},
		{
			name:      "quality too high",
			query:     "w=100&q=101",
			srcFormat: imaging.FormatJPEG,
			wantErr:   "quality must be 1-100",
		},
		{
			name:      "webp format",
			query:     "w=100&fmt=webp",
			srcFormat: imaging.FormatJPEG,
			wantW:     100, wantFmt: imaging.FormatWebP,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vals, _ := url.ParseQuery(tc.query)
			opts, err := parseTransformOptions(vals, tc.srcFormat)
			if tc.wantErr != "" {
				testutil.ErrorContains(t, err, tc.wantErr)
				return
			}
			testutil.NoError(t, err)
			if tc.wantW != 0 {
				testutil.Equal(t, tc.wantW, opts.Width)
			}
			if tc.wantH != 0 {
				testutil.Equal(t, tc.wantH, opts.Height)
			}
			if tc.wantFit != "" {
				testutil.Equal(t, tc.wantFit, opts.Fit)
			}
			if tc.wantQ != 0 {
				testutil.Equal(t, tc.wantQ, opts.Quality)
			}
			testutil.Equal(t, tc.wantFmt, opts.Format)
		})
	}
}

func TestServeTransformedJPEG(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 800, 600)
	obj := &Object{Bucket: "img", Name: "photo.jpg", Size: int64(len(imgData)), ContentType: "image/jpeg"}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200&h=150", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
	testutil.True(t, rec.Body.Len() > 0, "body should not be empty")

	// Verify output dimensions.
	result, _, err := image.Decode(bytes.NewReader(rec.Body.Bytes()))
	testutil.NoError(t, err)
	testutil.Equal(t, 200, result.Bounds().Dx())
	testutil.Equal(t, 150, result.Bounds().Dy())
}

func TestServeTransformedFormatConversion(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{Bucket: "img", Name: "photo.jpg", Size: int64(len(imgData)), ContentType: "image/jpeg"}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=100&fmt=png", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	// Verify PNG header.
	body := rec.Body.Bytes()
	testutil.True(t, len(body) > 4, "body should not be empty")
	testutil.Equal(t, byte(0x89), body[0])
	testutil.Equal(t, byte('P'), body[1])
}

func TestServeTransformedPNG(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestPNG(t, 600, 400)
	obj := &Object{Bucket: "img", Name: "icon.png", Size: int64(len(imgData)), ContentType: "image/png"}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/icon.png?w=150&h=100", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "image/png", rec.Header().Get("Content-Type"))
}

func TestServeTransformedCoverMode(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 800, 600)
	obj := &Object{Bucket: "img", Name: "photo.jpg", Size: int64(len(imgData)), ContentType: "image/jpeg"}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200&h=200&fit=cover", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	testutil.Equal(t, http.StatusOK, rec.Code)
	result, _, err := image.Decode(bytes.NewReader(rec.Body.Bytes()))
	testutil.NoError(t, err)
	testutil.Equal(t, 200, result.Bounds().Dx())
	testutil.Equal(t, 200, result.Bounds().Dy())
}

func TestServeTransformedNonImage(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	obj := &Object{Bucket: "docs", Name: "readme.txt", Size: 100, ContentType: "text/plain"}
	reader := io.NopCloser(bytes.NewReader([]byte("hello world")))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/docs/readme.txt?w=200", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
	testutil.Contains(t, rec.Body.String(), "not a supported image format")
}

func TestServeTransformedInvalidParams(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{Bucket: "img", Name: "photo.jpg", Size: int64(len(imgData)), ContentType: "image/jpeg"}

	tests := []struct {
		name    string
		query   string
		wantMsg string
	}{
		{"no dimensions", "?fmt=png", "w or h parameter is required"},
		{"invalid width", "?w=abc", "invalid width"},
		{"invalid quality", "?w=100&q=0", "quality must be 1-100"},
		{"bad format", "?w=100&fmt=bmp", "unsupported format"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := io.NopCloser(bytes.NewReader(imgData))
			req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg"+tc.query, nil)
			rec := httptest.NewRecorder()
			h.serveTransformed(rec, req, reader, obj, true)
			testutil.Equal(t, http.StatusBadRequest, rec.Code)
			testutil.Contains(t, rec.Body.String(), tc.wantMsg)
		})
	}
}

func TestServeTransformedCacheHeader(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{
		ID: "obj-1", Bucket: "img", Name: "photo.jpg",
		Size: int64(len(imgData)), ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=100", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "public, max-age=86400", rec.Header().Get("Cache-Control"))
	testutil.Equal(t, strconv.Itoa(rec.Body.Len()), rec.Header().Get("Content-Length"))
}

func TestServeTransformedCacheHeaderPrivate(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{
		ID: "obj-2", Bucket: "img", Name: "photo.jpg",
		Size: int64(len(imgData)), ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=100", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, false)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
}

func TestRawCacheControlPublic(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "public, max-age=31536000, immutable", cacheControlRaw(true))
}

func TestRawCacheControlPrivate(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "private, no-cache", cacheControlRaw(false))
}

func TestTransformCacheControlPublic(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "public, max-age=86400", cacheControlTransformed(true))
}

func TestTransformCacheControlPrivate(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "private, no-cache", cacheControlTransformed(false))
}

// --- 4D: WebP via handler ---

func TestServeTransformedWebP(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{
		ID: "obj-3", Bucket: "img", Name: "photo.jpg",
		Size: int64(len(imgData)), ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200&fmt=webp", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "image/webp", rec.Header().Get("Content-Type"))
	body := rec.Body.Bytes()
	testutil.True(t, len(body) > 4, "body should not be empty")
	// WebP RIFF header.
	testutil.Equal(t, byte('R'), body[0])
	testutil.Equal(t, byte('I'), body[1])
	testutil.Equal(t, byte('F'), body[2])
	testutil.Equal(t, byte('F'), body[3])
}

func TestParseTransformOptionsWebP(t *testing.T) {
	t.Parallel()
	vals, _ := url.ParseQuery("w=100&fmt=webp")
	opts, err := parseTransformOptions(vals, imaging.FormatJPEG)
	testutil.NoError(t, err)
	testutil.Equal(t, imaging.FormatWebP, opts.Format)
}

func TestParseTransformOptionsWebPLongFormatKey(t *testing.T) {
	t.Parallel()
	vals, _ := url.ParseQuery("w=100&format=webp")
	opts, err := parseTransformOptions(vals, imaging.FormatJPEG)
	testutil.NoError(t, err)
	testutil.Equal(t, imaging.FormatWebP, opts.Format)
}

func TestParseTransformOptionsAVIF(t *testing.T) {
	t.Parallel()
	vals, _ := url.ParseQuery("w=100&fmt=avif")
	opts, err := parseTransformOptions(vals, imaging.FormatJPEG)
	testutil.NoError(t, err)
	testutil.Equal(t, imaging.FormatAVIF, opts.Format)
}

func TestParseTransformOptionsUnsupportedFormatMessageIncludesAVIF(t *testing.T) {
	t.Parallel()
	vals, _ := url.ParseQuery("w=100&fmt=bmp")
	_, err := parseTransformOptions(vals, imaging.FormatJPEG)
	testutil.ErrorContains(t, err, "unsupported format (use jpeg, png, webp, or avif)")
}

// --- 4D: Crop params via handler ---

func TestHasTransformParamsCropOnly(t *testing.T) {
	t.Parallel()
	// crop param alone (without w/h) must still trigger transform path.
	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?crop=center", nil)
	testutil.True(t, hasTransformParams(req), "crop param alone should trigger transforms")
}

func TestParseTransformOptionsCropCenter(t *testing.T) {
	t.Parallel()
	vals, _ := url.ParseQuery("w=200&h=200&crop=center")
	opts, err := parseTransformOptions(vals, imaging.FormatJPEG)
	testutil.NoError(t, err)
	testutil.Equal(t, imaging.CropCenter, opts.Crop)
}

func TestParseTransformOptionsCropSmart(t *testing.T) {
	t.Parallel()
	vals, _ := url.ParseQuery("w=200&h=200&crop=smart")
	opts, err := parseTransformOptions(vals, imaging.FormatJPEG)
	testutil.NoError(t, err)
	testutil.Equal(t, imaging.CropSmart, opts.Crop)
}

func TestParseTransformOptionsCropInvalid(t *testing.T) {
	t.Parallel()
	vals, _ := url.ParseQuery("w=200&h=200&crop=foobar")
	_, err := parseTransformOptions(vals, imaging.FormatJPEG)
	testutil.ErrorContains(t, err, "unsupported crop mode")
}

func TestServeTransformedCropCenter(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 800, 600)
	obj := &Object{
		ID: "obj-4", Bucket: "img", Name: "photo.jpg",
		Size: int64(len(imgData)), ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200&h=200&crop=center", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	testutil.Equal(t, http.StatusOK, rec.Code)
	result, _, err := image.Decode(bytes.NewReader(rec.Body.Bytes()))
	testutil.NoError(t, err)
	testutil.Equal(t, 200, result.Bounds().Dx())
	testutil.Equal(t, 200, result.Bounds().Dy())
}

// --- 4D: Animated GIF passthrough ---

func makeHandlerTestGIF(t *testing.T, w, h, frames int) []byte {
	t.Helper()
	g := &gif.GIF{}
	for i := range frames {
		pal := color.Palette{color.Black, color.RGBA{R: uint8(i * 50), G: 100, B: 200, A: 255}}
		img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
		for y := range h {
			for x := range w {
				img.SetColorIndex(x, y, 1)
			}
		}
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("encoding test GIF: %v", err)
	}
	return buf.Bytes()
}

func TestServeTransformedAnimatedGIFPassthrough(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	gifData := makeHandlerTestGIF(t, 200, 150, 3) // 3 frames = animated
	obj := &Object{
		ID: "obj-5", Bucket: "img", Name: "anim.gif",
		Size: int64(len(gifData)), ContentType: "image/gif",
		UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	reader := io.NopCloser(bytes.NewReader(gifData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/anim.gif?w=100", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	// Animated GIF should be served as-is (passthrough), not transformed.
	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "image/gif", rec.Header().Get("Content-Type"))
	testutil.Equal(t, "public, max-age=86400", rec.Header().Get("Cache-Control"))
	testutil.True(t, bytes.Equal(gifData, rec.Body.Bytes()), "animated GIF should be served unchanged")
}

func TestServeTransformedAnimatedGIFPassthroughPrivateCache(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	gifData := makeHandlerTestGIF(t, 200, 150, 3)
	obj := &Object{
		ID: "obj-5b", Bucket: "img", Name: "anim.gif",
		Size: int64(len(gifData)), ContentType: "image/gif",
		UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	reader := io.NopCloser(bytes.NewReader(gifData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/anim.gif?w=100", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, false)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
}

func TestServeTransformedAnimatedGIFPassthroughWithAVIFRequest(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	gifData := makeHandlerTestGIF(t, 200, 150, 3)
	obj := &Object{
		ID: "obj-5c", Bucket: "img", Name: "anim.gif",
		Size: int64(len(gifData)), ContentType: "image/gif",
		UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	reader := io.NopCloser(bytes.NewReader(gifData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/anim.gif?w=100&fmt=avif", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "image/gif", rec.Header().Get("Content-Type"))
	testutil.Equal(t, "public, max-age=86400", rec.Header().Get("Cache-Control"))
	testutil.True(t, bytes.Equal(gifData, rec.Body.Bytes()), "animated GIF should be served unchanged")
}

func TestServeTransformedStaticGIFTransformed(t *testing.T) {
	// Single-frame GIFs CAN'T be transformed because GIF is not a supported
	// source format for imaging.Transform (we only support JPEG/PNG/WebP decode).
	// This test verifies we return an appropriate error for static GIF with transforms.
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	gifData := makeHandlerTestGIF(t, 200, 150, 1) // 1 frame = static
	obj := &Object{
		ID: "obj-6", Bucket: "img", Name: "static.gif",
		Size: int64(len(gifData)), ContentType: "image/gif",
		UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	reader := io.NopCloser(bytes.NewReader(gifData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/static.gif?w=100", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj, true)

	// GIF is not a supported transform source — should get an error.
	testutil.Equal(t, http.StatusBadRequest, rec.Code)
	testutil.Contains(t, rec.Body.String(), "not a supported image format")
}

// --- 4D: ETag and 304 Not Modified ---

func TestServeTransformedETagDeterministic(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{
		ID: "obj-8", Bucket: "img", Name: "photo.jpg",
		Size: int64(len(imgData)), ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	// Two identical requests should produce the same ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200&q=80", nil)
	rec1 := httptest.NewRecorder()
	h.serveTransformed(rec1, req1, io.NopCloser(bytes.NewReader(imgData)), obj, true)

	req2 := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200&q=80", nil)
	rec2 := httptest.NewRecorder()
	h.serveTransformed(rec2, req2, io.NopCloser(bytes.NewReader(imgData)), obj, true)

	testutil.Equal(t, rec1.Header().Get("ETag"), rec2.Header().Get("ETag"))
}

func TestServeTransformedETagDiffersPerTransform(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{
		ID: "obj-9", Bucket: "img", Name: "photo.jpg",
		Size: int64(len(imgData)), ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	req1 := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200", nil)
	rec1 := httptest.NewRecorder()
	h.serveTransformed(rec1, req1, io.NopCloser(bytes.NewReader(imgData)), obj, true)

	req2 := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=300", nil)
	rec2 := httptest.NewRecorder()
	h.serveTransformed(rec2, req2, io.NopCloser(bytes.NewReader(imgData)), obj, true)

	testutil.True(t, rec1.Header().Get("ETag") != rec2.Header().Get("ETag"),
		"different transforms should produce different ETags")
}

func TestServeTransformed304NotModified(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{
		ID: "obj-10", Bucket: "img", Name: "photo.jpg",
		Size: int64(len(imgData)), ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	// First request: get the ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200", nil)
	rec1 := httptest.NewRecorder()
	h.serveTransformed(rec1, req1, io.NopCloser(bytes.NewReader(imgData)), obj, true)
	etag := rec1.Header().Get("ETag")

	// Second request: send If-None-Match with the ETag.
	req2 := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.serveTransformed(rec2, req2, io.NopCloser(bytes.NewReader(imgData)), obj, true)

	testutil.Equal(t, http.StatusNotModified, rec2.Code)
	testutil.Equal(t, 0, rec2.Body.Len())
}

func TestServeTransformed304NotModifiedWithTokenList(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{
		ID: "obj-10b", Bucket: "img", Name: "photo.jpg",
		Size: int64(len(imgData)), ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}
	query := "w=200"
	etag := computeTransformETag(obj, query)

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?"+query, nil)
	req.Header.Set("If-None-Match", `"other", `+etag+`, "another"`)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, io.NopCloser(bytes.NewReader(imgData)), obj, true)

	testutil.Equal(t, http.StatusNotModified, rec.Code)
	testutil.Equal(t, 0, rec.Body.Len())
}

func TestServeTransformed304WrongETag(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "")
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{
		ID: "obj-11", Bucket: "img", Name: "photo.jpg",
		Size: int64(len(imgData)), ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200", nil)
	req.Header.Set("If-None-Match", `"wrong-etag"`)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, io.NopCloser(bytes.NewReader(imgData)), obj, true)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.True(t, rec.Body.Len() > 0, "should return full response for non-matching ETag")
}

func TestApplyConditionalRawETagSetsETag(t *testing.T) {
	t.Parallel()
	obj := &Object{
		ID:        "obj-raw-1",
		UpdatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg", nil)
	rec := httptest.NewRecorder()

	notModified := applyConditionalRawETag(rec, req, obj)
	testutil.False(t, notModified)
	testutil.Equal(t, computeObjectETag(obj), rec.Header().Get("ETag"))
}

func TestApplyConditionalRawETagReturns304OnMatch(t *testing.T) {
	t.Parallel()
	obj := &Object{
		ID:        "obj-raw-2",
		UpdatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	}
	etag := computeObjectETag(obj)

	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()

	notModified := applyConditionalRawETag(rec, req, obj)
	testutil.True(t, notModified)
	testutil.Equal(t, http.StatusNotModified, rec.Code)
	testutil.Equal(t, etag, rec.Header().Get("ETag"))
}

func TestApplyConditionalRawETagReturns304OnTokenListMatch(t *testing.T) {
	t.Parallel()
	obj := &Object{
		ID:        "obj-raw-2b",
		UpdatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	}
	etag := computeObjectETag(obj)

	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg", nil)
	req.Header.Set("If-None-Match", `"different", `+etag+`, W/"weak-etag"`)
	rec := httptest.NewRecorder()

	notModified := applyConditionalRawETag(rec, req, obj)
	testutil.True(t, notModified)
	testutil.Equal(t, http.StatusNotModified, rec.Code)
	testutil.Equal(t, etag, rec.Header().Get("ETag"))
}

func TestApplyConditionalRawETagNonMatchReturns200Path(t *testing.T) {
	t.Parallel()
	obj := &Object{
		ID:        "obj-raw-3",
		UpdatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/storage/images/photo.jpg", nil)
	req.Header.Set("If-None-Match", `"different"`)
	rec := httptest.NewRecorder()

	notModified := applyConditionalRawETag(rec, req, obj)
	testutil.False(t, notModified)
	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, computeObjectETag(obj), rec.Header().Get("ETag"))
}

func TestComputeObjectETagDiffersWithinSameSecond(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 10, 0, 0, 123, time.UTC)
	obj1 := &Object{
		ID:        "obj-raw-subsec",
		Size:      2048,
		UpdatedAt: base,
	}
	obj2 := &Object{
		ID:        "obj-raw-subsec",
		Size:      2048,
		UpdatedAt: base.Add(900 * time.Millisecond),
	}

	etag1 := computeObjectETag(obj1)
	etag2 := computeObjectETag(obj2)
	testutil.True(t, etag1 != etag2, "ETag must change for sub-second object updates")
}

func TestComputeTransformETagDiffersWithinSameSecond(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 10, 0, 0, 123, time.UTC)
	obj1 := &Object{
		ID:        "obj-transform-subsec",
		UpdatedAt: base,
	}
	obj2 := &Object{
		ID:        "obj-transform-subsec",
		UpdatedAt: base.Add(900 * time.Millisecond),
	}

	etag1 := computeTransformETag(obj1, "w=200&q=80")
	etag2 := computeTransformETag(obj2, "w=200&q=80")
	testutil.True(t, etag1 != etag2, "transform ETag must change for sub-second object updates")
}

// --- TUS metadata parsing ---

func TestParseTusMetadata(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"single pair", "name dGVzdC50eHQ=", map[string]string{"name": "test.txt"}},
		{"two pairs", "name dGVzdC50eHQ=,contentType YXBwbGljYXRpb24vb2N0ZXQtc3RyZWFt", map[string]string{
			"name":        "test.txt",
			"contentType": "application/octet-stream",
		}},
		{"key only no value", "is_confidential", map[string]string{"is_confidential": ""}},
		{"mixed key-only and kv", "is_confidential,name dGVzdC50eHQ=", map[string]string{
			"is_confidential": "",
			"name":            "test.txt",
		}},
		{"spaces around comma", "name dGVzdC50eHQ= , contentType YXBwbGljYXRpb24vb2N0ZXQtc3RyZWFt", map[string]string{
			"name":        "test.txt",
			"contentType": "application/octet-stream",
		}},
		{"invalid base64 skipped", "name !!!invalid!!!", map[string]string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseTusMetadata(tc.raw)
			testutil.Equal(t, len(tc.want), len(got))
			for k, v := range tc.want {
				testutil.Equal(t, v, got[k])
			}
		})
	}
}

// --- ETag excludes non-transform params ---

// --- TUS ownership enforcement ---

func TestEnforceUploadOwnership(t *testing.T) {
	t.Parallel()
	ownerID := "user-123"
	upload := &ResumableUpload{UserID: &ownerID}

	// Owner can access their own upload.
	testutil.NoError(t, enforceUploadOwnership(upload, &ownerID))

	// Different user cannot access.
	otherID := "user-456"
	err := enforceUploadOwnership(upload, &otherID)
	testutil.ErrorContains(t, err, "forbidden")

	// Nil caller (admin bypass) can access.
	testutil.NoError(t, enforceUploadOwnership(upload, nil))

	// Upload with no owner is reserved for admin-only access.
	noOwnerUpload := &ResumableUpload{}
	testutil.ErrorContains(t, enforceUploadOwnership(noOwnerUpload, &otherID), "forbidden")
	testutil.NoError(t, enforceUploadOwnership(noOwnerUpload, nil))
}

func TestServeResumableErrorForbidden(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	serveResumableError(rec, ErrResumableUploadForbidden)
	testutil.Equal(t, http.StatusForbidden, rec.Code)
}

// --- LIKE prefix escaping ---

func TestEscapeLikePrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"normal", "normal"},
		{"hello%world", `hello\%world`},
		{"hello_world", `hello\_world`},
		{`back\slash`, `back\\slash`},
		{"a%b_c", `a\%b\_c`},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tc.want, escapeLikePrefix(tc.input))
		})
	}
}

// --- ETag excludes non-transform params ---

func TestTransformETagExcludesSignatureParams(t *testing.T) {
	t.Parallel()
	obj := &Object{
		ID: "obj-etag-sig", Bucket: "img", Name: "photo.jpg",
		Size: 1000, ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	// Same transform params, different sig/exp — ETags should be identical.
	etag1 := computeTransformETag(obj, "w=200&sig=abc123&exp=99999")
	etag2 := computeTransformETag(obj, "w=200&sig=def456&exp=11111")
	testutil.Equal(t, etag1, etag2)

	// Different transform params — ETags should differ.
	etag3 := computeTransformETag(obj, "w=300&sig=abc123&exp=99999")
	testutil.True(t, etag1 != etag3, "different transform params should produce different ETags")
}

func TestTransformETagTreatsFormatAliasEqually(t *testing.T) {
	t.Parallel()
	obj := &Object{
		ID: "obj-etag-format", Bucket: "img", Name: "photo.jpg",
		Size: 1000, ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	etagFmt := computeTransformETag(obj, "w=200&fmt=webp")
	etagFormat := computeTransformETag(obj, "w=200&format=webp")
	testutil.Equal(t, etagFmt, etagFormat)
}

func TestTransformETagPrefersFmtOverFormatAlias(t *testing.T) {
	t.Parallel()
	obj := &Object{
		ID: "obj-etag-format-precedence", Bucket: "img", Name: "photo.jpg",
		Size: 1000, ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	// parseTransformOptions uses fmt precedence when both are present.
	// ETag canonicalization must match this behavior to avoid cache fragmentation.
	etagFmtOnly := computeTransformETag(obj, "w=200&fmt=webp")
	etagFmtAndDifferentAlias := computeTransformETag(obj, "w=200&fmt=webp&format=png")
	testutil.Equal(t, etagFmtOnly, etagFmtAndDifferentAlias)
}

func TestTransformETagKeepsValidTransformsOnMalformedUnrelatedParam(t *testing.T) {
	t.Parallel()
	obj := &Object{
		ID: "obj-etag-malformed-unrelated", Bucket: "img", Name: "photo.jpg",
		Size: 1000, ContentType: "image/jpeg",
		UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	etagClean := computeTransformETag(obj, "w=200")
	etagWithMalformedUnrelated := computeTransformETag(obj, "w=200&bad=%zz")
	testutil.Equal(t, etagClean, etagWithMalformedUnrelated)
}
