package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type tenantSearchPathConn interface {
	tenant.RequestConn
	Destroy(ctx context.Context) error
	Release()
}

type tenantConnAcquireFunc func(ctx context.Context) (tenantSearchPathConn, error)

type pgxTenantSearchPathConn struct {
	conn *pgxpool.Conn
}

func (c *pgxTenantSearchPathConn) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return c.conn.Exec(ctx, sql, arguments...)
}

func (c *pgxTenantSearchPathConn) Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error) {
	return c.conn.Query(ctx, sql, arguments...)
}

func (c *pgxTenantSearchPathConn) QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row {
	return c.conn.QueryRow(ctx, sql, arguments...)
}

func (c *pgxTenantSearchPathConn) Begin(ctx context.Context) (pgx.Tx, error) {
	return c.conn.Begin(ctx)
}

func (c *pgxTenantSearchPathConn) Release() {
	if c.conn == nil {
		return
	}
	c.conn.Release()
	c.conn = nil
}

func (c *pgxTenantSearchPathConn) Destroy(ctx context.Context) error {
	if c.conn == nil {
		return nil
	}
	conn := c.conn.Hijack()
	c.conn = nil
	return conn.Close(ctx)
}

func newTenantConnAcquire(pool *pgxpool.Pool) tenantConnAcquireFunc {
	if pool == nil {
		return nil
	}
	return func(ctx context.Context) (tenantSearchPathConn, error) {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		return &pgxTenantSearchPathConn{conn: conn}, nil
	}
}

// setTenantSearchPath is middleware that sets the PostgreSQL search_path to a tenant's schema for schema-isolated tenants, resetting it to public after the request completes.
func (s *Server) setTenantSearchPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant.TenantFromContext(r.Context())
		if tenantID == "" || s == nil || s.tenantSvc == nil || s.tenantConnAcquire == nil {
			next.ServeHTTP(w, r)
			return
		}

		tenantInfo, err := s.tenantSvc.GetTenant(r.Context(), tenantID)
		if err != nil {
			if logger := s.currentLogger(); logger != nil {
				logger.Warn("failed to load tenant for search_path isolation", "tenant_id", tenantID, "error", err)
			}
			httputil.WriteError(w, http.StatusServiceUnavailable, "tenant schema isolation unavailable")
			return
		}
		if tenantInfo == nil {
			if logger := s.currentLogger(); logger != nil {
				logger.Warn("tenant lookup returned nil during search_path isolation", "tenant_id", tenantID)
			}
			httputil.WriteError(w, http.StatusServiceUnavailable, "tenant schema isolation unavailable")
			return
		}
		if tenantInfo.IsolationMode != "schema" {
			next.ServeHTTP(w, r)
			return
		}

		conn, err := s.tenantConnAcquire(r.Context())
		if err != nil {
			if logger := s.currentLogger(); logger != nil {
				logger.Warn("failed to acquire tenant search_path connection", "tenant_id", tenantID, "error", err)
			}
			httputil.WriteError(w, http.StatusServiceUnavailable, "tenant schema isolation unavailable")
			return
		}

		schemaName := pgx.Identifier{tenantInfo.Slug}.Sanitize()
		searchPathSQL := fmt.Sprintf(`SET search_path TO %s, public`, schemaName)
		if _, err := conn.Exec(r.Context(), searchPathSQL); err != nil {
			if logger := s.currentLogger(); logger != nil {
				logger.Warn("failed to set tenant search_path", "tenant_id", tenantID, "slug", tenantInfo.Slug, "error", err)
			}
			destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer destroyCancel()
			if destroyErr := conn.Destroy(destroyCtx); destroyErr != nil {
				if logger := s.currentLogger(); logger != nil {
					logger.Warn("failed to destroy connection after search_path set failure", "tenant_id", tenantID, "error", destroyErr)
				}
			}
			httputil.WriteError(w, http.StatusServiceUnavailable, "tenant schema isolation unavailable")
			return
		}

		ctx := tenant.ContextWithRequestConn(r.Context(), conn)
		defer func() {
			resetCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := conn.Exec(resetCtx, `SET search_path TO public`); err != nil {
				if logger := s.currentLogger(); logger != nil {
					logger.Warn("failed to reset tenant search_path", "tenant_id", tenantID, "slug", tenantInfo.Slug, "error", err)
				}
				destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer destroyCancel()
				if err := conn.Destroy(destroyCtx); err != nil {
					if logger := s.currentLogger(); logger != nil {
						logger.Warn("failed to destroy tainted tenant search_path connection", "tenant_id", tenantID, "slug", tenantInfo.Slug, "error", err)
					}
				}
				return
			}
			conn.Release()
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
