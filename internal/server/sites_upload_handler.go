// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/sites_upload_handler.go.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/sites"
	"github.com/allyourbase/ayb/internal/storage"
)

const (
	sitesStorageBucketName      = "_ayb_sites"
	deployUploadMultipartMemory = 32 << 20
	deployUploadFailedMessage   = "deploy file upload failed"
)

type deployUploadStorage interface {
	GetBucket(ctx context.Context, name string) (*storage.Bucket, error)
	CreateBucket(ctx context.Context, name string, public bool) (*storage.Bucket, error)
	Upload(ctx context.Context, bucket, name, contentType string, userID *string, r io.Reader) (*storage.Object, error)
}

type deployUploadRequest struct {
	file         multipart.File
	relativeName string
	contentType  string
}

// TODO: Document handleAdminUploadDeployFile.
func handleAdminUploadDeployFile(svc siteManager, storageSvc deployUploadStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := extractSiteID(w, r)
		if !ok {
			return
		}
		deployID, ok := extractDeployID(w, r)
		if !ok {
			return
		}

		if err := svc.EnsureDeployUploading(r.Context(), siteID, deployID); err != nil {
			writeDeployUploadStateError(w, err)
			return
		}

		upload, ok := parseDeployUploadRequest(w, r)
		if !ok {
			return
		}
		defer upload.file.Close()

		objectName, err := deployUploadObjectName(siteID, deployID, upload.relativeName)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "file path must stay within the deploy directory")
			return
		}

		if err := ensureSitesStorageBucket(r.Context(), storageSvc); err != nil {
			markDeployFailedAfterUploadError(r.Context(), svc, siteID, deployID)
			httputil.WriteError(w, http.StatusInternalServerError, "failed to upload deploy file")
			return
		}

		uploadedObject, err := storageSvc.Upload(
			r.Context(),
			sitesStorageBucketName,
			objectName,
			upload.contentType,
			nil,
			upload.file,
		)
		if err != nil {
			markDeployFailedAfterUploadError(r.Context(), svc, siteID, deployID)
			httputil.WriteError(w, http.StatusInternalServerError, "failed to upload deploy file")
			return
		}

		deploy, err := svc.RecordDeployFileUpload(r.Context(), siteID, deployID, uploadedObject.Size)
		if err != nil {
			writeDeployUploadStateError(w, err)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, deploy)
	}
}

// TODO: Document parseDeployUploadRequest.
func parseDeployUploadRequest(w http.ResponseWriter, r *http.Request) (*deployUploadRequest, bool) {
	if err := r.ParseMultipartForm(deployUploadMultipartMemory); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return nil, false
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "missing \"file\" field in multipart form")
		return nil, false
	}

	relativeName := r.FormValue("name")
	if relativeName == "" {
		relativeName = header.Filename
	}
	normalizedName, err := normalizeDeployUploadPath(relativeName)
	if err != nil {
		file.Close()
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}

	return &deployUploadRequest{
		file:         file,
		relativeName: normalizedName,
		contentType:  detectUploadContentType(normalizedName, header.Header.Get("Content-Type")),
	}, true
}

func normalizeDeployUploadPath(rawPath string) (string, error) {
	cleanCandidate := strings.TrimSpace(strings.ReplaceAll(rawPath, "\\", "/"))
	if cleanCandidate == "" {
		return "", fmt.Errorf("file name is required")
	}
	if strings.HasPrefix(cleanCandidate, "/") {
		return "", fmt.Errorf("file name must be relative")
	}

	normalizedPath := path.Clean(cleanCandidate)
	if normalizedPath == "." || normalizedPath == ".." || strings.HasPrefix(normalizedPath, "../") {
		return "", fmt.Errorf("file name must stay within the deploy directory")
	}
	return normalizedPath, nil
}

func deployUploadObjectName(siteID, deployID, relativeName string) (string, error) {
	deployPrefix := path.Join("sites", siteID, deployID)
	objectName := path.Join(deployPrefix, relativeName)
	if !strings.HasPrefix(objectName, deployPrefix+"/") {
		return "", fmt.Errorf("invalid deploy object path")
	}
	return objectName, nil
}

func detectUploadContentType(fileName, formContentType string) string {
	contentType := mime.TypeByExtension(filepath.Ext(fileName))
	if contentType != "" {
		return contentType
	}
	if formContentType != "" {
		return formContentType
	}
	return "application/octet-stream"
}

func ensureSitesStorageBucket(ctx context.Context, storageSvc deployUploadStorage) error {
	_, err := storageSvc.GetBucket(ctx, sitesStorageBucketName)
	if err == nil {
		return nil
	}
	if !errors.Is(err, storage.ErrBucketNotFound) {
		return err
	}

	_, err = storageSvc.CreateBucket(ctx, sitesStorageBucketName, false)
	if err != nil && !errors.Is(err, storage.ErrAlreadyExists) {
		return err
	}
	return nil
}

func markDeployFailedAfterUploadError(ctx context.Context, svc siteManager, siteID, deployID string) {
	_, _ = svc.FailDeploy(ctx, siteID, deployID, deployUploadFailedMessage)
}

func writeDeployUploadStateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sites.ErrDeployNotFound):
		httputil.WriteError(w, http.StatusNotFound, "deploy not found")
	case errors.Is(err, sites.ErrInvalidTransition):
		httputil.WriteError(w, http.StatusConflict, "deploy is not accepting file uploads")
	default:
		httputil.WriteError(w, http.StatusInternalServerError, "failed to process deploy upload")
	}
}
