package edgefunc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestGojaRuntime_Fetch_SimpleGET(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"greeting":"hello from server"}`))
	}))
	defer srv.Close()

	rt := NewGojaRuntime(WithHTTPClient(srv.Client()))

	code := `
function handler(request) {
	var resp = fetch("` + srv.URL + `");
	return {
		statusCode: resp.status,
		body: resp.text(),
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), `"greeting":"hello from server"`))
}

func TestGojaRuntime_Fetch_POSTWithBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "POST", r.Method)
		testutil.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(201)
		w.Write(body)
	}))
	defer srv.Close()

	rt := NewGojaRuntime(WithHTTPClient(srv.Client()))

	code := `
function handler(request) {
	var resp = fetch("` + srv.URL + `", {
		method: "POST",
		headers: {"Content-Type": "application/json"},
		body: JSON.stringify({name: "alice"})
	});
	return {
		statusCode: resp.status,
		body: resp.text(),
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 201, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), `"name":"alice"`))
}

func TestGojaRuntime_Fetch_JSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":42,"active":true}`))
	}))
	defer srv.Close()

	rt := NewGojaRuntime(WithHTTPClient(srv.Client()))

	code := `
function handler(request) {
	var resp = fetch("` + srv.URL + `");
	var data = resp.json();
	return {
		statusCode: 200,
		body: JSON.stringify({id: data.id, active: data.active}),
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), `"id":42`))
	testutil.True(t, strings.Contains(string(resp.Body), `"active":true`))
}

func TestGojaRuntime_Fetch_ResponseHeaders(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "test-value")
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	rt := NewGojaRuntime(WithHTTPClient(srv.Client()))

	code := `
function handler(request) {
	var resp = fetch("` + srv.URL + `");
	return {
		statusCode: 200,
		body: resp.headers.get("X-Custom"),
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, "test-value", string(resp.Body))
}

func TestGojaRuntime_Fetch_RespectsContextTimeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
		w.Write([]byte("too late"))
	}))
	defer srv.Close()

	rt := NewGojaRuntime(WithHTTPClient(srv.Client()))

	code := `
function handler(request) {
	var resp = fetch("` + srv.URL + `");
	return { statusCode: 200, body: resp.text() };
}
`
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := rt.Execute(ctx, code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected error from timeout, got nil")
}

func TestGojaRuntime_Fetch_NetworkError(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `
function handler(request) {
	var resp = fetch("http://127.0.0.1:1"); // nothing listening
	return { statusCode: 200, body: "should not reach" };
}
`
	_, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected network error, got nil")
}

func TestGojaRuntime_Fetch_StatusText(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	rt := NewGojaRuntime(WithHTTPClient(srv.Client()))

	// Fetch API statusText should be just "Not Found", not "404 Not Found".
	code := `
function handler(request) {
	var resp = fetch("` + srv.URL + `");
	return {
		statusCode: 200,
		body: resp.statusText,
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, "Not Found", string(resp.Body))
}

func TestGojaRuntime_Fetch_ResponseBodySizeLimit(t *testing.T) {
	t.Parallel()
	// Server returns a response larger than maxFetchResponseBytes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		// Write 6 MB (exceeds 5 MB limit).
		chunk := make([]byte, 1024)
		for i := range chunk {
			chunk[i] = 'A'
		}
		for i := 0; i < 6*1024; i++ {
			w.Write(chunk)
		}
	}))
	defer srv.Close()

	rt := NewGojaRuntime(WithHTTPClient(srv.Client()))

	code := `
function handler(request) {
	var resp = fetch("` + srv.URL + `");
	return { statusCode: 200, body: resp.text() };
}
`
	_, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected error from oversized response body")
	testutil.True(t, strings.Contains(err.Error(), "limit"),
		"expected 'limit' in error, got: %s", err)
}

func TestGojaRuntime_Fetch_OkProperty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(404)
		}
		w.Write([]byte("body"))
	}))
	defer srv.Close()

	rt := NewGojaRuntime(WithHTTPClient(srv.Client()))

	code := `
function handler(request) {
	var r1 = fetch("` + srv.URL + `/ok");
	var r2 = fetch("` + srv.URL + `/notfound");
	return {
		statusCode: 200,
		body: JSON.stringify({ok: r1.ok, notOk: r2.ok}),
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(string(resp.Body), `"ok":true`))
	testutil.True(t, strings.Contains(string(resp.Body), `"notOk":false`))
}
