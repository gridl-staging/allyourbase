package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/sites"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeSiteRuntimeResolver struct {
	byDomain map[string]*sites.RuntimeSite
	bySlug   map[string]*sites.RuntimeSite
	err      error

	domainLookups []string
	slugLookups   []string
}

func (f *fakeSiteRuntimeResolver) ResolveRuntimeSiteByCustomDomainID(_ context.Context, domainID string) (*sites.RuntimeSite, error) {
	f.domainLookups = append(f.domainLookups, domainID)
	if f.err != nil {
		return nil, f.err
	}
	runtimeSite, ok := f.byDomain[domainID]
	if !ok {
		return nil, sites.ErrSiteNotFound
	}
	return runtimeSite, nil
}

func (f *fakeSiteRuntimeResolver) ResolveRuntimeSiteBySlug(_ context.Context, slug string) (*sites.RuntimeSite, error) {
	f.slugLookups = append(f.slugLookups, slug)
	if f.err != nil {
		return nil, f.err
	}
	runtimeSite, ok := f.bySlug[slug]
	if !ok {
		return nil, sites.ErrSiteNotFound
	}
	return runtimeSite, nil
}

type fakeSiteRuntimeStorage struct {
	objects map[string]runtimeObject
	errors  map[string]error

	downloads []string
}

type runtimeObject struct {
	body        string
	contentType string
}

func (f *fakeSiteRuntimeStorage) Download(_ context.Context, bucket, name string) (io.ReadCloser, *storage.Object, error) {
	f.downloads = append(f.downloads, bucket+"/"+name)
	if err, ok := f.errors[name]; ok {
		return nil, nil, err
	}
	obj, ok := f.objects[name]
	if !ok {
		return nil, nil, storage.ErrNotFound
	}
	return io.NopCloser(strings.NewReader(obj.body)), &storage.Object{
		Bucket:      bucket,
		Name:        name,
		ContentType: obj.contentType,
		Size:        int64(len(obj.body)),
	}, nil
}

func TestSiteRuntimeMiddleware_ServesLiveDeployForCustomDomain(t *testing.T) {
	t.Parallel()

	resolver := &fakeSiteRuntimeResolver{
		byDomain: map[string]*sites.RuntimeSite{
			"dom-1": {
				SiteID:       "site-1",
				Slug:         "alpha",
				SPAMode:      true,
				LiveDeployID: "dep-1",
			},
		},
	}
	st := &fakeSiteRuntimeStorage{objects: map[string]runtimeObject{
		"sites/site-1/dep-1/assets/app.js": {
			body:        "console.log('runtime');",
			contentType: "application/javascript",
		},
	}}

	nextCalled := false
	h := buildSiteRuntimeMiddleware(resolver, st, "localhost")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Host = "app.example.com"
	req = req.WithContext(context.WithValue(req.Context(), customDomainRouteKey{}, RouteEntry{DomainID: "dom-1", Hostname: "app.example.com", Status: StatusActive}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, false, nextCalled)
	testutil.Equal(t, "console.log('runtime');", w.Body.String())
	testutil.Equal(t, "application/javascript", w.Header().Get("Content-Type"))
	testutil.Equal(t, 1, len(resolver.domainLookups))
}

func TestSiteRuntimeMiddleware_ResolvesDerivedSlugHostAndFallsBackToIndex(t *testing.T) {
	t.Parallel()

	resolver := &fakeSiteRuntimeResolver{
		bySlug: map[string]*sites.RuntimeSite{
			"alpha": {
				SiteID:       "site-1",
				Slug:         "alpha",
				SPAMode:      true,
				LiveDeployID: "dep-1",
			},
		},
	}
	st := &fakeSiteRuntimeStorage{objects: map[string]runtimeObject{
		"sites/site-1/dep-1/index.html": {
			body:        "<html>alpha</html>",
			contentType: "text/html; charset=utf-8",
		},
	}}

	h := buildSiteRuntimeMiddleware(resolver, st, "localhost")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/missing/client/route", nil)
	req.Host = "alpha.localhost"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "<html>alpha</html>", w.Body.String())
	testutil.Equal(t, 1, len(resolver.slugLookups))
	testutil.Equal(t, "alpha", resolver.slugLookups[0])
	testutil.Equal(t, 2, len(st.downloads))
	testutil.Equal(t, "_ayb_sites/sites/site-1/dep-1/missing/client/route", st.downloads[0])
	testutil.Equal(t, "_ayb_sites/sites/site-1/dep-1/index.html", st.downloads[1])
}

func TestSiteRuntimeMiddleware_Returns404WhenSPADisabledAndFileMissing(t *testing.T) {
	t.Parallel()

	resolver := &fakeSiteRuntimeResolver{
		bySlug: map[string]*sites.RuntimeSite{
			"alpha": {
				SiteID:       "site-1",
				Slug:         "alpha",
				SPAMode:      false,
				LiveDeployID: "dep-1",
			},
		},
	}
	st := &fakeSiteRuntimeStorage{objects: map[string]runtimeObject{}}

	h := buildSiteRuntimeMiddleware(resolver, st, "localhost")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/missing-client-route", nil)
	req.Host = "alpha.localhost"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestSiteRuntimeMiddleware_BypassesAPIAdminAndHealthPaths(t *testing.T) {
	t.Parallel()

	resolver := &fakeSiteRuntimeResolver{}
	st := &fakeSiteRuntimeStorage{}

	paths := []string{"/api/openapi.json", "/admin", "/health"}
	for _, path := range paths {
		nextCalled := false
		h := buildSiteRuntimeMiddleware(resolver, st, "localhost")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "alpha.localhost"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		testutil.Equal(t, true, nextCalled)
		testutil.Equal(t, http.StatusNoContent, w.Code)
	}
}

func TestSiteRuntimeMiddleware_UnknownHostFallsThrough(t *testing.T) {
	t.Parallel()

	resolver := &fakeSiteRuntimeResolver{}
	st := &fakeSiteRuntimeStorage{}

	nextCalled := false
	h := buildSiteRuntimeMiddleware(resolver, st, "localhost")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "unknown.example.com"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, true, nextCalled)
	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, 0, len(st.downloads))
}

func TestSiteRuntimeMiddleware_DoesNotLeakStorageObjectNameOnDownloadError(t *testing.T) {
	t.Parallel()

	resolver := &fakeSiteRuntimeResolver{
		bySlug: map[string]*sites.RuntimeSite{
			"alpha": {
				SiteID:       "site-1",
				Slug:         "alpha",
				SPAMode:      true,
				LiveDeployID: "dep-1",
			},
		},
	}
	st := &fakeSiteRuntimeStorage{
		objects: map[string]runtimeObject{},
		errors: map[string]error{
			"sites/site-1/dep-1/index.html": errors.New("backend failure for sites/site-1/dep-1/index.html"),
		},
	}

	h := buildSiteRuntimeMiddleware(resolver, st, "localhost")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "alpha.localhost"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, false, strings.Contains(w.Body.String(), "sites/site-1/dep-1/index.html"))
}

func TestSiteRuntimeMiddleware_RejectsPathTraversalVariants(t *testing.T) {
	t.Parallel()

	resolver := &fakeSiteRuntimeResolver{
		bySlug: map[string]*sites.RuntimeSite{
			"alpha": {
				SiteID:       "site-1",
				Slug:         "alpha",
				SPAMode:      false,
				LiveDeployID: "dep-1",
			},
		},
	}

	traversalPaths := []string{
		"/../../../etc/passwd",
		"/..%2f..%2fetc/passwd",
		`/foo\..\..\secret`,
	}

	for _, tp := range traversalPaths {
		st := &fakeSiteRuntimeStorage{objects: map[string]runtimeObject{}}
		h := buildSiteRuntimeMiddleware(resolver, st, "localhost")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, tp, nil)
		req.Host = "alpha.localhost"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("path=%q: expected 404, got %d", tp, w.Code)
		}
		for _, dl := range st.downloads {
			if strings.Contains(dl, "..") {
				t.Errorf("path=%q: download contained traversal: %s", tp, dl)
			}
		}
	}
}

func TestSiteRuntimeMiddleware_TombstonedDomainDoesNotExposeInternalState(t *testing.T) {
	t.Parallel()

	resolver := &fakeSiteRuntimeResolver{}
	st := &fakeSiteRuntimeStorage{}

	nextCalled := false
	h := buildSiteRuntimeMiddleware(resolver, st, "localhost")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	req.Host = "tombstoned.example.com"
	tombstonedEntry := RouteEntry{
		DomainID: "dom-secret-123",
		Hostname: "tombstoned.example.com",
		Status:   StatusTombstoned,
	}
	req = req.WithContext(context.WithValue(req.Context(), customDomainRouteKey{}, tombstonedEntry))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// The runtime middleware should not try to resolve a tombstoned domain; it falls through.
	// The host route middleware upstream already returns 421 for tombstoned domains, but
	// if context somehow still carries the tombstoned entry, runtime should treat DomainID
	// as valid and attempt resolution — which will fail with ErrSiteNotFound and fall through.
	testutil.Equal(t, true, nextCalled)
	body := w.Body.String()
	testutil.Equal(t, false, strings.Contains(body, "dom-secret-123"))
	testutil.Equal(t, 0, len(st.downloads))
}

func TestSiteRuntimeMiddleware_NonGetMethodBypasses(t *testing.T) {
	t.Parallel()

	resolver := &fakeSiteRuntimeResolver{
		bySlug: map[string]*sites.RuntimeSite{
			"alpha": {SiteID: "site-1", Slug: "alpha", SPAMode: true, LiveDeployID: "dep-1"},
		},
	}
	st := &fakeSiteRuntimeStorage{}

	nextCalled := false
	h := buildSiteRuntimeMiddleware(resolver, st, "localhost")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "alpha.localhost"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	testutil.Equal(t, true, nextCalled)
	testutil.Equal(t, 0, len(st.downloads))
}

func TestConfiguredSiteRuntimeHostBaseMapsLoopbackToLocalhost(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.Host = "127.0.0.1"

	testutil.Equal(t, "localhost", configuredSiteRuntimeHostBase(cfg))
}
