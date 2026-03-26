package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgconn"
)

type auditCaptureExecer struct {
	args []any
}

func (a *auditCaptureExecer) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	a.args = args
	return pgconn.CommandTag{}, nil
}

func assertAuditIPArg(t *testing.T, value any, want string) {
	t.Helper()

	switch ip := value.(type) {
	case net.IP:
		testutil.Equal(t, want, ip.String())
	case string:
		testutil.Equal(t, want, ip)
	default:
		t.Fatalf("expected ip argument to be net.IP or string, got %T", value)
	}
}

func TestRequireAdminTokenMiddleware(t *testing.T) {
	t.Parallel()
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	t.Run("fails closed when admin auth not configured", func(t *testing.T) {
		t.Parallel()
		s := &Server{} // adminAuth is nil
		handler := s.requireAdminToken(okHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusUnauthorized, w.Code)
		testutil.Contains(t, w.Body.String(), "admin authentication required")
	})

	t.Run("valid token passes", func(t *testing.T) {
		t.Parallel()
		s := &Server{adminAuth: newAdminAuth("secret")}
		handler := s.requireAdminToken(okHandler)
		token := s.adminAuth.token()

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("valid token carries request ip into audit context", func(t *testing.T) {
		t.Parallel()
		auditExec := &auditCaptureExecer{}
		auditLogger := audit.NewAuditLogger(config.AuditConfig{Enabled: true, AllTables: true}, auditExec)
		s := &Server{adminAuth: newAdminAuth("secret")}
		handler := s.requireAdminToken(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := auditLogger.LogMutation(r.Context(), audit.AuditEntry{
				TableName: "_ayb_restore_jobs",
				Operation: "INSERT",
				NewValues: map[string]string{"result": "success"},
			}); err != nil {
				t.Fatalf("LogMutation: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		}))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "198.51.100.44:1234"
		req.Header.Set("Authorization", "Bearer "+s.adminAuth.token())
		handler.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.True(t, auditExec.args[1] != nil, "expected principal-backed api_key_id argument")
		assertAuditIPArg(t, auditExec.args[7], "198.51.100.44")
	})

	t.Run("invalid token rejected", func(t *testing.T) {
		t.Parallel()
		s := &Server{adminAuth: newAdminAuth("secret")}
		handler := s.requireAdminToken(okHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		handler.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusUnauthorized, w.Code)
		testutil.Contains(t, w.Body.String(), "admin authentication required")
	})

	t.Run("missing token rejected", func(t *testing.T) {
		t.Parallel()
		s := &Server{adminAuth: newAdminAuth("secret")}
		handler := s.requireAdminToken(okHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
