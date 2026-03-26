package storage

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

type cdnProviderStub struct {
	purgeURLsFn func(context.Context, []string) error
	purgeAllFn  func(context.Context) error
}

func (p *cdnProviderStub) Name() string {
	return "stub"
}

func (p *cdnProviderStub) PurgeURLs(ctx context.Context, publicURLs []string) error {
	if p.purgeURLsFn != nil {
		return p.purgeURLsFn(ctx, publicURLs)
	}
	return nil
}

func (p *cdnProviderStub) PurgeAll(ctx context.Context) error {
	if p.purgeAllFn != nil {
		return p.purgeAllFn(ctx)
	}
	return nil
}

func newUploadRequest(t *testing.T, path, filename string, body []byte) *http.Request {
	t.Helper()

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	filePart, err := writer.CreateFormFile("file", filename)
	testutil.NoError(t, err)
	_, err = io.Copy(filePart, bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, path, payload)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestHandleUploadOverwriteEnqueuesCDNPurgeAsync(t *testing.T) {
	t.Parallel()

	providerCalled := make(chan []string, 1)
	releaseProvider := make(chan struct{})
	provider := &cdnProviderStub{
		purgeURLsFn: func(_ context.Context, publicURLs []string) error {
			providerCalled <- publicURLs
			<-releaseProvider
			return nil
		},
	}

	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "https://cdn.example.com")
	h.SetCDNProvider(provider)
	h.mutations.upload = func(_ context.Context, bucket, name, contentType string, userID *string, r io.Reader) (*Object, error) {
		data, _ := io.ReadAll(r)
		_ = data
		return &Object{
			ID:          "obj-1",
			Bucket:      bucket,
			Name:        name,
			Size:        int64(len(data)),
			ContentType: contentType,
			CreatedAt:   time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 3, 1, 10, 1, 0, 0, time.UTC),
		}, nil
	}

	router := testRouter(h)
	req := newUploadRequest(t, "/api/storage/images", "cat.jpg", []byte("image-bytes"))
	rec := httptest.NewRecorder()

	start := time.Now()
	router.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	testutil.Equal(t, http.StatusCreated, rec.Code)
	testutil.True(t, elapsed < 100*time.Millisecond, "upload handler should not block on CDN purge")

	select {
	case urls := <-providerCalled:
		testutil.SliceLen(t, urls, 1)
		testutil.Equal(t, "https://cdn.example.com/api/storage/images/cat.jpg", urls[0])
	case <-time.After(time.Second):
		t.Fatal("expected async purge to be enqueued")
	}
	close(releaseProvider)
}

func TestHandleDeleteStillReturnsNoContentWhenPurgeFails(t *testing.T) {
	t.Parallel()

	providerCalled := make(chan []string, 1)
	provider := &cdnProviderStub{
		purgeURLsFn: func(_ context.Context, publicURLs []string) error {
			providerCalled <- publicURLs
			return context.DeadlineExceeded
		},
	}

	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "https://cdn.example.com")
	h.SetCDNProvider(provider)
	h.mutations.getObject = func(_ context.Context, bucket, name string) (*Object, error) {
		return &Object{Bucket: bucket, Name: name, Size: 5}, nil
	}
	h.mutations.deleteObject = func(_ context.Context, bucket, name string) error { return nil }

	router := testRouter(h)
	req := httptest.NewRequest(http.MethodDelete, "/api/storage/private-bucket/path/to/file.txt", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	testutil.Equal(t, http.StatusNoContent, rec.Code)

	select {
	case urls := <-providerCalled:
		testutil.SliceLen(t, urls, 1)
		testutil.Equal(t, "https://cdn.example.com/api/storage/private-bucket/path/to/file.txt", urls[0])
	case <-time.After(time.Second):
		t.Fatal("expected delete flow to enqueue async purge")
	}
}

func TestHandleResumablePatchOverwriteEnqueuesCDNPurge(t *testing.T) {
	t.Parallel()

	providerCalled := make(chan []string, 1)
	provider := &cdnProviderStub{
		purgeURLsFn: func(_ context.Context, publicURLs []string) error {
			providerCalled <- publicURLs
			return nil
		},
	}

	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "https://cdn.example.com")
	h.SetCDNProvider(provider)
	h.mutations.appendResumableUpload = func(_ context.Context, id string, offset int64, callerUserID *string, src io.Reader) (*ResumableUpload, bool, error) {
		data, _ := io.ReadAll(src)
		_ = data
		return &ResumableUpload{
			ID:           id,
			Bucket:       "images",
			Name:         "video.mp4",
			UploadedSize: 12,
			TotalSize:    12,
		}, true, nil
	}
	h.mutations.finalizeResumableUpload = func(_ context.Context, id string, callerUserID *string) (*Object, error) {
		return &Object{
			ID:        "obj-2",
			Bucket:    "images",
			Name:      "video.mp4",
			CreatedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 3, 1, 10, 2, 0, 0, time.UTC),
		}, nil
	}

	router := testRouter(h)
	req := httptest.NewRequest(http.MethodPatch, "/api/storage/upload/resumable/upload-123", bytes.NewReader([]byte("chunk-data")))
	req.Header.Set("Tus-Resumable", tusResumableVersion)
	req.Header.Set("Content-Type", tusOffsetContentType)
	req.Header.Set(tusUploadOffsetHeader, "0")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	testutil.Equal(t, http.StatusNoContent, rec.Code)

	select {
	case urls := <-providerCalled:
		testutil.SliceLen(t, urls, 1)
		testutil.Equal(t, "https://cdn.example.com/api/storage/images/video.mp4", urls[0])
	case <-time.After(time.Second):
		t.Fatal("expected resumable overwrite to enqueue async purge")
	}
}

func TestCDNPurgeUsesDetachedContextWithBoundedTimeout(t *testing.T) {
	t.Parallel()

	ctxErrCh := make(chan error, 1)
	provider := &cdnProviderStub{
		purgeURLsFn: func(ctx context.Context, _ []string) error {
			<-ctx.Done()
			ctxErrCh <- ctx.Err()
			return ctx.Err()
		},
	}

	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "https://cdn.example.com")
	h.SetCDNProvider(provider)
	h.cdnPurgeCoordinator.timeout = 25 * time.Millisecond
	h.mutations.getObject = func(_ context.Context, bucket, name string) (*Object, error) {
		return &Object{Bucket: bucket, Name: name, Size: 1}, nil
	}
	h.mutations.deleteObject = func(_ context.Context, bucket, name string) error { return nil }

	router := testRouter(h)
	req := httptest.NewRequest(http.MethodDelete, "/api/storage/images/file.txt", nil)
	requestCtx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(requestCtx)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	cancel() // simulate caller context cancellation right after response
	testutil.Equal(t, http.StatusNoContent, rec.Code)

	select {
	case err := <-ctxErrCh:
		testutil.ErrorContains(t, err, context.DeadlineExceeded.Error())
	case <-time.After(time.Second):
		t.Fatal("expected purge provider context to time out")
	}
}
