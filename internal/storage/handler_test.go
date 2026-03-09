package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
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
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
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

func TestHandleSignedURLInvalid(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
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
	h := NewHandler(svc, testutil.DiscardLogger(), 10<<20)
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
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
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
			name:      "unsupported format",
			query:     "w=100&fmt=webp",
			srcFormat: imaging.FormatJPEG,
			wantErr:   "unsupported format",
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
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
	imgData := makeHandlerTestJPEG(t, 800, 600)
	obj := &Object{Bucket: "img", Name: "photo.jpg", Size: int64(len(imgData)), ContentType: "image/jpeg"}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200&h=150", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj)

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
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{Bucket: "img", Name: "photo.jpg", Size: int64(len(imgData)), ContentType: "image/jpeg"}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=100&fmt=png", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj)

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
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
	imgData := makeHandlerTestPNG(t, 600, 400)
	obj := &Object{Bucket: "img", Name: "icon.png", Size: int64(len(imgData)), ContentType: "image/png"}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/icon.png?w=150&h=100", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "image/png", rec.Header().Get("Content-Type"))
}

func TestServeTransformedCoverMode(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
	imgData := makeHandlerTestJPEG(t, 800, 600)
	obj := &Object{Bucket: "img", Name: "photo.jpg", Size: int64(len(imgData)), ContentType: "image/jpeg"}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=200&h=200&fit=cover", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj)

	testutil.Equal(t, http.StatusOK, rec.Code)
	result, _, err := image.Decode(bytes.NewReader(rec.Body.Bytes()))
	testutil.NoError(t, err)
	testutil.Equal(t, 200, result.Bounds().Dx())
	testutil.Equal(t, 200, result.Bounds().Dy())
}

func TestServeTransformedNonImage(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
	obj := &Object{Bucket: "docs", Name: "readme.txt", Size: 100, ContentType: "text/plain"}
	reader := io.NopCloser(bytes.NewReader([]byte("hello world")))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/docs/readme.txt?w=200", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj)

	testutil.Equal(t, http.StatusBadRequest, rec.Code)
	testutil.Contains(t, rec.Body.String(), "not a supported image format")
}

func TestServeTransformedInvalidParams(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
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
			h.serveTransformed(rec, req, reader, obj)
			testutil.Equal(t, http.StatusBadRequest, rec.Code)
			testutil.Contains(t, rec.Body.String(), tc.wantMsg)
		})
	}
}

func TestServeTransformedCacheHeader(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20)
	imgData := makeHandlerTestJPEG(t, 400, 300)
	obj := &Object{Bucket: "img", Name: "photo.jpg", Size: int64(len(imgData)), ContentType: "image/jpeg"}
	reader := io.NopCloser(bytes.NewReader(imgData))

	req := httptest.NewRequest(http.MethodGet, "/api/storage/img/photo.jpg?w=100", nil)
	rec := httptest.NewRecorder()
	h.serveTransformed(rec, req, reader, obj)

	testutil.Equal(t, http.StatusOK, rec.Code)
	testutil.Equal(t, "public, max-age=86400", rec.Header().Get("Cache-Control"))
	testutil.Equal(t, strconv.Itoa(rec.Body.Len()), rec.Header().Get("Content-Length"))
}
