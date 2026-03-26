package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestCloudflare_PurgeURLs_SendsAuthHeaderAndPayload(t *testing.T) {
	t.Parallel()

	var gotAuth, gotMethod, gotPath string
	var gotPayload cloudflarePurgeRequest

	provider := NewCloudflareCDNProvider(CloudflareCDNOptions{
		ZoneID:   "zone-1",
		APIToken: "token",
		HTTPClient: &fakeHTTPDoer{handler: func(req *http.Request) (*http.Response, error) {
			gotAuth = req.Header.Get("Authorization")
			gotMethod = req.Method
			gotPath = req.URL.Path
			err := json.NewDecoder(req.Body).Decode(&gotPayload)
			if err != nil {
				return nil, err
			}
			return makeHTTPResponse(http.StatusOK, `{"success":true}`), nil
		}},
	})

	err := provider.PurgeURLs(context.Background(), []string{"https://api.example.com/public/a", "https://api.example.com/public/b"})
	testutil.NoError(t, err)
	testutil.Equal(t, http.MethodPost, gotMethod)
	testutil.Equal(t, "/client/v4/zones/zone-1/purge_cache", gotPath)
	testutil.Equal(t, "Bearer token", gotAuth)
	if !reflect.DeepEqual([]string{"https://api.example.com/public/a", "https://api.example.com/public/b"}, gotPayload.Files) {
		t.Fatalf("expected payload files %v, got %v", []string{"https://api.example.com/public/a", "https://api.example.com/public/b"}, gotPayload.Files)
	}
}

func TestCloudflare_PurgeURLs_BatchesAtThirtyBoundary(t *testing.T) {
	t.Parallel()

	requestCounts := make(map[int]int)
	requests := 0
	urls := make([]string, 0, 31)
	for i := 0; i < 31; i++ {
		urls = append(urls, fmt.Sprintf("https://cdn.example.com/%d", i))
	}

	provider := NewCloudflareCDNProvider(CloudflareCDNOptions{
		ZoneID:   "zone-1",
		APIToken: "token",
		HTTPClient: &fakeHTTPDoer{handler: func(req *http.Request) (*http.Response, error) {
			requests++

			var payload cloudflarePurgeRequest
			_ = json.NewDecoder(req.Body).Decode(&payload)
			requestCounts[len(payload.Files)]++
			return makeHTTPResponse(http.StatusOK, `{"success":true}`), nil
		}},
		MaxRetries: 0,
	})

	err := provider.PurgeURLs(context.Background(), urls)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, requests)
	testutil.Equal(t, 1, requestCounts[30])
	testutil.Equal(t, 1, requestCounts[1])
}

func TestCloudflare_PurgeURLsRetriesTransportAndHTTPRetryables(t *testing.T) {
	t.Parallel()

	attempts := 0
	provider := NewCloudflareCDNProvider(CloudflareCDNOptions{
		ZoneID:   "zone-1",
		APIToken: "token",
		HTTPClient: &fakeHTTPDoer{handler: func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return makeHTTPResponse(http.StatusInternalServerError, `retry`), nil
			}
			return makeHTTPResponse(http.StatusOK, `{"success":true}`), nil
		}},
		MaxRetries: 3,
	})

	err := provider.PurgeURLs(context.Background(), []string{"https://cdn.example.com/file"})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, attempts)
}

func TestCloudflare_PurgeURLs_NonRetryableHTTPError(t *testing.T) {
	t.Parallel()

	attempts := 0
	provider := NewCloudflareCDNProvider(CloudflareCDNOptions{
		ZoneID:   "zone-1",
		APIToken: "token",
		HTTPClient: &fakeHTTPDoer{handler: func(req *http.Request) (*http.Response, error) {
			attempts++
			return makeHTTPResponse(http.StatusUnauthorized, `nope`), nil
		}},
		MaxRetries: 3,
	})

	err := provider.PurgeURLs(context.Background(), []string{"https://cdn.example.com/file"})
	testutil.ErrorContains(t, err, "status=401")
	testutil.Equal(t, 1, attempts)
}

func TestCloudflare_PurgeAll_UsesPurgeEverything(t *testing.T) {
	t.Parallel()

	var payload cloudflarePurgeRequest
	provider := NewCloudflareCDNProvider(CloudflareCDNOptions{
		ZoneID:   "zone-1",
		APIToken: "token",
		HTTPClient: &fakeHTTPDoer{handler: func(req *http.Request) (*http.Response, error) {
			err := json.NewDecoder(req.Body).Decode(&payload)
			if err != nil {
				return nil, err
			}
			return makeHTTPResponse(http.StatusOK, `{"success":true}`), nil
		}},
	})

	err := provider.PurgeAll(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, true, payload.PurgeEverything)
	testutil.Equal(t, 0, len(payload.Files))
}

type fakeHTTPDoer struct {
	handler func(*http.Request) (*http.Response, error)
}

func (f *fakeHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	return f.handler(req)
}

func makeHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
