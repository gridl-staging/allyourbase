// Package allyourbase Provides storage operations for uploading, downloading, and listing bucket objects.
package allyourbase

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
)

type StorageListParams struct {
	Prefix string
	Limit  int
	Offset int
}

type StorageClient struct {
	client *Client
}

// Upload uploads file content to the specified bucket with the given name and content type, returning the created storage object or an error.
func (s *StorageClient) Upload(ctx context.Context, bucket, name string, content []byte, contentType string) (*StorageObject, error) {
	buf := bytes.NewBuffer(nil)
	mw := multipart.NewWriter(buf)
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(content); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	_, body, _, err := s.client.do(ctx, http.MethodPost, "/api/storage/"+bucket, nil, buf, map[string]string{"Content-Type": mw.FormDataContentType()}, true)
	if err != nil {
		return nil, err
	}
	var out StorageObject
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if contentType != "" {
		out.ContentType = contentType
	}
	return &out, nil
}

func (s *StorageClient) Download(ctx context.Context, bucket, name string) ([]byte, error) {
	_, body, _, err := s.client.do(ctx, http.MethodGet, "/api/storage/"+bucket+"/"+name, nil, nil, nil, true)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// List returns objects stored in the specified bucket filtered by the provided parameters, or an error.
func (s *StorageClient) List(ctx context.Context, bucket string, params StorageListParams) (*StorageListResponse, error) {
	q := url.Values{}
	if params.Prefix != "" {
		q.Set("prefix", params.Prefix)
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Offset > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
	}
	body, err := s.client.doJSON(ctx, http.MethodGet, "/api/storage/"+bucket, q, nil)
	if err != nil {
		return nil, err
	}
	var out StorageListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
