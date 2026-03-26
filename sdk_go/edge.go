package allyourbase

import (
	"bytes"
	"context"
	"net/http"
)

type EdgeInvokeRequest struct {
	Method  string
	Body    []byte
	Headers map[string]string
}

type EdgeInvokeResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

type EdgeClient struct {
	client *Client
}

func (e *EdgeClient) Invoke(ctx context.Context, name string, req EdgeInvokeRequest) (*EdgeInvokeResponse, error) {
	method := req.Method
	if method == "" {
		method = http.MethodPost
	}
	status, body, headers, err := e.client.do(ctx, method, "/functions/v1/"+name, nil, bytes.NewReader(req.Body), req.Headers, true)
	if err != nil {
		return nil, err
	}
	return &EdgeInvokeResponse{StatusCode: status, Headers: headers, Body: body}, nil
}
