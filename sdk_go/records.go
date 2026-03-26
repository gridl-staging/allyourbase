// Package allyourbase Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/sdk_go/records.go.
package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

type ListParams struct {
	Page      int
	PerPage   int
	Sort      string
	Filter    string
	Search    string
	Fields    string
	Expand    string
	SkipTotal bool
}

type GetParams struct {
	Fields string
	Expand string
}

type RecordsClient struct {
	client *Client
}

// TODO: Document RecordsClient.List.
func (r *RecordsClient) List(ctx context.Context, collection string, params ListParams) (*ListResponse, error) {
	q := url.Values{}
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.PerPage > 0 {
		q.Set("perPage", strconv.Itoa(params.PerPage))
	}
	if params.Sort != "" {
		q.Set("sort", params.Sort)
	}
	if params.Filter != "" {
		q.Set("filter", params.Filter)
	}
	if params.Search != "" {
		q.Set("search", params.Search)
	}
	if params.Fields != "" {
		q.Set("fields", params.Fields)
	}
	if params.Expand != "" {
		q.Set("expand", params.Expand)
	}
	if params.SkipTotal {
		q.Set("skipTotal", "true")
	}
	body, err := r.client.doJSON(ctx, http.MethodGet, "/api/collections/"+collection, q, nil)
	if err != nil {
		return nil, err
	}
	var out ListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TODO: Document RecordsClient.Get.
func (r *RecordsClient) Get(ctx context.Context, collection, id string, params GetParams) (map[string]any, error) {
	q := url.Values{}
	if params.Fields != "" {
		q.Set("fields", params.Fields)
	}
	if params.Expand != "" {
		q.Set("expand", params.Expand)
	}
	body, err := r.client.doJSON(ctx, http.MethodGet, "/api/collections/"+collection+"/"+id, q, nil)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *RecordsClient) Create(ctx context.Context, collection string, data map[string]any) (map[string]any, error) {
	return r.writeRecord(ctx, http.MethodPost, "/api/collections/"+collection, data)
}

func (r *RecordsClient) Update(ctx context.Context, collection, id string, data map[string]any) (map[string]any, error) {
	return r.writeRecord(ctx, http.MethodPatch, "/api/collections/"+collection+"/"+id, data)
}

func (r *RecordsClient) Delete(ctx context.Context, collection, id string) error {
	_, err := r.client.doJSON(ctx, http.MethodDelete, "/api/collections/"+collection+"/"+id, nil, nil)
	return err
}

func (r *RecordsClient) Batch(ctx context.Context, collection string, operations []BatchOperation) ([]BatchResult, error) {
	body, err := r.client.doJSON(ctx, http.MethodPost, "/api/collections/"+collection+"/batch", nil, map[string]any{"operations": operations})
	if err != nil {
		return nil, err
	}
	var out []BatchResult
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *RecordsClient) writeRecord(ctx context.Context, method, path string, data map[string]any) (map[string]any, error) {
	body, err := r.client.doJSON(ctx, method, path, nil, data)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
