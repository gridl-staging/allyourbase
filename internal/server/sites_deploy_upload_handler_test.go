package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/sites"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestHandleAdminUploadDeployFileSuccess(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	deployID := "00000000-0000-0000-0000-000000000002"
	mgr := &fakeSiteManager{
		deploys: []sites.Deploy{{ID: deployID, SiteID: siteID, Status: sites.StatusUploading}},
	}
	st := &fakeDeployUploadStorage{getBucketErr: storage.ErrBucketNotFound}

	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/{deployId}/files", handleAdminUploadDeployFile(mgr, st))
	req := buildDeployUploadRequest(t, "/admin/sites/"+siteID+"/deploys/"+deployID+"/files", "assets/app.js", []byte("console.log('ok')"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Equal(t, 1, st.createBucketCalls)
	testutil.Equal(t, "_ayb_sites", st.uploadBucket)
	testutil.Equal(t, "sites/"+siteID+"/"+deployID+"/assets/app.js", st.uploadName)

	var deploy sites.Deploy
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&deploy))
	testutil.Equal(t, 1, deploy.FileCount)
	testutil.Equal(t, int64(len("console.log('ok')")), deploy.TotalBytes)
}

func TestHandleAdminUploadDeployFileRejectsNonUploadingDeploy(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	deployID := "00000000-0000-0000-0000-000000000002"
	mgr := &fakeSiteManager{
		deploys: []sites.Deploy{{ID: deployID, SiteID: siteID, Status: sites.StatusLive}},
	}
	st := &fakeDeployUploadStorage{}

	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/{deployId}/files", handleAdminUploadDeployFile(mgr, st))
	req := buildDeployUploadRequest(t, "/admin/sites/"+siteID+"/deploys/"+deployID+"/files", "index.html", []byte("<h1>Hello</h1>"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
	testutil.Equal(t, 0, st.uploadCalls)
}

func TestHandleAdminUploadDeployFileRejectsMismatchedDeploy(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	deployID := "00000000-0000-0000-0000-000000000002"
	mgr := &fakeSiteManager{
		deploys: []sites.Deploy{{ID: deployID, SiteID: "00000000-0000-0000-0000-000000000099", Status: sites.StatusUploading}},
	}
	st := &fakeDeployUploadStorage{}

	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/{deployId}/files", handleAdminUploadDeployFile(mgr, st))
	req := buildDeployUploadRequest(t, "/admin/sites/"+siteID+"/deploys/"+deployID+"/files", "index.html", []byte("<h1>Hello</h1>"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Equal(t, 0, st.uploadCalls)
}

func TestHandleAdminUploadDeployFileRejectsPathEscape(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	deployID := "00000000-0000-0000-0000-000000000002"
	mgr := &fakeSiteManager{
		deploys: []sites.Deploy{{ID: deployID, SiteID: siteID, Status: sites.StatusUploading}},
	}
	st := &fakeDeployUploadStorage{}

	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/{deployId}/files", handleAdminUploadDeployFile(mgr, st))
	req := buildDeployUploadRequest(t, "/admin/sites/"+siteID+"/deploys/"+deployID+"/files", "../outside.txt", []byte("secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Equal(t, 0, st.uploadCalls)
}

func TestHandleAdminUploadDeployFileRejectsPathTraversalVariants(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	deployID := "00000000-0000-0000-0000-000000000002"

	traversalNames := []string{
		"../outside.txt",
		"foo/../../etc/passwd",
		`foo\..\..\..\etc\passwd`,
		"/etc/passwd",
		"",
		".",
		"..",
	}

	for _, name := range traversalNames {
		mgr := &fakeSiteManager{
			deploys: []sites.Deploy{{ID: deployID, SiteID: siteID, Status: sites.StatusUploading}},
		}
		st := &fakeDeployUploadStorage{}

		r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/{deployId}/files", handleAdminUploadDeployFile(mgr, st))
		req := buildDeployUploadRequest(t, "/admin/sites/"+siteID+"/deploys/"+deployID+"/files", name, []byte("secret"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("name=%q: expected 400, got %d", name, w.Code)
		}
		if st.uploadCalls != 0 {
			t.Errorf("name=%q: expected 0 upload calls, got %d", name, st.uploadCalls)
		}
	}
}

func TestHandleAdminUploadDeployFileMarksDeployFailedOnStorageError(t *testing.T) {
	t.Parallel()
	siteID := "00000000-0000-0000-0000-000000000001"
	deployID := "00000000-0000-0000-0000-000000000002"
	mgr := &fakeSiteManager{
		deploys: []sites.Deploy{{ID: deployID, SiteID: siteID, Status: sites.StatusUploading}},
	}
	st := &fakeDeployUploadStorage{uploadErr: errors.New("backend timed out")}

	r := siteRouter(http.MethodPost, "/admin/sites/{siteId}/deploys/{deployId}/files", handleAdminUploadDeployFile(mgr, st))
	req := buildDeployUploadRequest(t, "/admin/sites/"+siteID+"/deploys/"+deployID+"/files", "index.html", []byte("<h1>Hello</h1>"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, false, strings.Contains(w.Body.String(), "backend timed out"))
	testutil.Equal(t, sites.StatusFailed, mgr.deploys[0].Status)
	testutil.NotNil(t, mgr.deploys[0].ErrorMessage)
}

type fakeDeployUploadStorage struct {
	getBucketErr error

	createBucketErr error
	uploadErr       error

	createBucketCalls int
	uploadCalls       int

	uploadBucket string
	uploadName   string
}

func (f *fakeDeployUploadStorage) GetBucket(_ context.Context, _ string) (*storage.Bucket, error) {
	if f.getBucketErr != nil {
		return nil, f.getBucketErr
	}
	return &storage.Bucket{Name: "_ayb_sites", Public: false}, nil
}

func (f *fakeDeployUploadStorage) CreateBucket(_ context.Context, _ string, _ bool) (*storage.Bucket, error) {
	f.createBucketCalls++
	if f.createBucketErr != nil {
		return nil, f.createBucketErr
	}
	return &storage.Bucket{Name: "_ayb_sites", Public: false}, nil
}

func (f *fakeDeployUploadStorage) Upload(_ context.Context, bucket, name, _ string, _ *string, r io.Reader) (*storage.Object, error) {
	f.uploadCalls++
	f.uploadBucket = bucket
	f.uploadName = name

	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &storage.Object{
		Bucket: bucket,
		Name:   name,
		Size:   int64(len(body)),
	}, nil
}

func buildDeployUploadRequest(t *testing.T, path, fileName string, body []byte) *http.Request {
	t.Helper()
	payload := new(bytes.Buffer)
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("name", fileName)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create multipart file field: %v", err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatalf("write multipart file body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, payload)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
