// Package allyourbase Provides the core HTTP client for authenticated API requests to the AllYourBase service.
package allyourbase

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithAPIKey(apiKey string) Option {
	return func(c *Client) {
		c.token = apiKey
		c.refreshToken = ""
	}
}

func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		c.userAgent = userAgent
	}
}

type Client struct {
	baseURL      string
	httpClient   *http.Client
	token        string
	refreshToken string
	userAgent    string

	Auth    *AuthClient
	Records *RecordsClient
	Storage *StorageClient
	Edge    *EdgeClient
}

func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.Auth = &AuthClient{client: c}
	c.Records = &RecordsClient{client: c}
	c.Storage = &StorageClient{client: c}
	c.Edge = &EdgeClient{client: c}
	return c
}

func (c *Client) SetTokens(token, refreshToken string) {
	c.token = token
	c.refreshToken = refreshToken
}

func (c *Client) ClearTokens() {
	c.token = ""
	c.refreshToken = ""
}

func (c *Client) Token() string {
	return c.token
}

func (c *Client) RefreshToken() string {
	return c.refreshToken
}

// doJSON sends an HTTP request with a JSON-encoded body and returns the raw response body, automatically applying authentication.
func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	var rdr io.Reader
	headers := map[string]string{}
	if body != nil {
		buf := bytes.NewBuffer(nil)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return nil, err
		}
		rdr = buf
		headers["Content-Type"] = "application/json"
	}
	_, respBody, _, err := c.do(ctx, method, path, query, rdr, headers, true)
	if err != nil {
		return nil, err
	}
	return respBody, nil
}

// do sends an HTTP request and returns the status code, response body, response headers, and any error, automatically applying authentication and normalizing HTTP errors.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body io.Reader, headers map[string]string, useAuth bool) (int, []byte, http.Header, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if useAuth && c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, nil, nil, normalizeError(resp.StatusCode, resp.Status, respBody)
	}
	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil, resp.Header.Clone(), nil
	}
	return resp.StatusCode, respBody, resp.Header.Clone(), nil
}

// normalizeError constructs an Error from an HTTP response status and body, extracting error details from JSON-encoded error information in the response if available.
func normalizeError(status int, statusText string, body []byte) error {
	apiErr := &Error{Status: status, Message: statusText}
	if len(body) == 0 {
		return apiErr
	}
	var payload struct {
		Message string         `json:"message"`
		Code    any            `json:"code"`
		Data    map[string]any `json:"data"`
		DocURL  string         `json:"doc_url"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return apiErr
	}
	if payload.Message != "" {
		apiErr.Message = payload.Message
	}
	apiErr.Code = normalizeErrorCode(payload.Code)
	apiErr.Data = payload.Data
	apiErr.DocURL = payload.DocURL
	return apiErr
}

// normalizeErrorCode converts any numeric or string type to its string representation, handling floats, integers, and JSON numbers, returning empty string for NaN or infinite values.
func normalizeErrorCode(code any) string {
	switch v := code.(type) {
	case string:
		return v
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return ""
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return ""
		}
		return strconv.FormatFloat(f, 'f', -1, 32)
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}
