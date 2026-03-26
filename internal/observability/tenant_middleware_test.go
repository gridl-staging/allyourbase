package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/tenant"
)

func TestTenantMiddleware_MetricsWithTenantContext(t *testing.T) {
	registry, err := NewHTTPMetrics()
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := TenantContextMiddleware(registry)(handler)

	tenantID := "00000000-0000-0000-0000-000000000001"
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := tenant.ContextWithTenantID(context.Background(), tenantID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestTenantMiddleware_MetricsWithoutTenantContext(t *testing.T) {
	registry, err := NewHTTPMetrics()
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := TenantContextMiddleware(registry)(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestTenantContextMiddleware_NilHTTPMetrics(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := TenantContextMiddleware(nil)(handler)

	tenantID := "00000000-0000-0000-0000-000000000001"
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := tenant.ContextWithTenantID(context.Background(), tenantID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestTenantMiddleware_ExtractsTenantIDFromContext(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		expectTenantID bool
	}{
		{"with tenant id", "00000000-0000-0000-0000-000000000001", true},
		{"empty tenant id", "", false},
		{"no tenant context", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.expectTenantID && tt.tenantID != "" {
				ctx = tenant.ContextWithTenantID(ctx, tt.tenantID)
			}

			extractedID := tenant.TenantFromContext(ctx)
			if tt.expectTenantID {
				if extractedID != tt.tenantID {
					t.Errorf("tenantID = %q, want %q", extractedID, tt.tenantID)
				}
			} else {
				if extractedID != "" {
					t.Errorf("tenantID = %q, want empty", extractedID)
				}
			}
		})
	}
}

func TestTenantMiddleware_WithBaseHTTPMetrics_DoesNotDuplicateRequests(t *testing.T) {
	registry, err := NewHTTPMetrics()
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	chain := registry.Middleware(TenantContextMiddleware(registry)(handler))

	tenantID := "00000000-0000-0000-0000-000000000001"
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(tenant.ContextWithTenantID(context.Background(), tenantID))

	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	metricsResp := httptest.NewRecorder()
	registry.Handler().ServeHTTP(metricsResp, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	metricsBody := metricsResp.Body.String()

	var matching []string
	for _, line := range strings.Split(metricsBody, "\n") {
		if !strings.HasPrefix(line, "ayb_http_requests_total{") {
			continue
		}
		if strings.Contains(line, `method="GET"`) &&
			strings.Contains(line, `route="unknown"`) &&
			strings.Contains(line, `status="200"`) {
			matching = append(matching, line)
		}
	}
	if len(matching) != 1 {
		t.Fatalf("expected exactly 1 matching request metric line, got %d:\n%s", len(matching), strings.Join(matching, "\n"))
	}
	if !strings.Contains(matching[0], `tenant_id="`+tenantID+`"`) {
		t.Fatalf("expected tenant-labeled metric line, got: %s", matching[0])
	}
}
