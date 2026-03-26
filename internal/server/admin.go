// Package server admin.go provides stateless HMAC-based authentication for the admin dashboard and middleware for protecting API endpoints with user authentication combined with rate limiting.
package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/google/uuid"
)

// adminAuth handles simple password-based admin dashboard authentication.
// Stateless: tokens are HMAC-derived from a per-boot secret, so no storage needed.
type adminAuth struct {
	password string
	secret   []byte // random 32 bytes, regenerated each server start
}

func newAdminAuth(password string) *adminAuth {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return &adminAuth{password: password, secret: secret}
}

func (a *adminAuth) token() string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte("ayb-admin"))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *adminAuth) validatePassword(password string) bool {
	return subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) == 1
}

func (a *adminAuth) validateToken(token string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.token())) == 1
}

// handleAdminStatus returns whether admin authentication is required.
func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	s.adminMu.RLock()
	configured := s.adminAuth != nil
	s.adminMu.RUnlock()
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{
		"auth": configured,
	})
}

// handleAdminLogin validates the admin password and returns a token.
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	s.adminMu.RLock()
	aa := s.adminAuth
	s.adminMu.RUnlock()

	if aa == nil {
		httputil.WriteError(w, http.StatusNotFound, "admin auth not configured")
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if !httputil.DecodeJSON(w, r, &body) {
		return
	}

	if !aa.validatePassword(body.Password) {
		httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized, "invalid password",
			"https://allyourbase.io/guide/admin-dashboard")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"token": aa.token(),
	})
}

// requireAdminToken returns middleware that requires a valid admin token.
// Fails closed when admin auth is not configured.
func (s *Server) requireAdminToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.adminMu.RLock()
		aa := s.adminAuth
		s.adminMu.RUnlock()

		if aa == nil {
			httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized, "admin authentication required",
				"https://allyourbase.io/guide/admin-dashboard")
			return
		}

		token, ok := httputil.ExtractBearerToken(r)
		if !ok || !aa.validateToken(token) {
			httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized, "admin authentication required",
				"https://allyourbase.io/guide/admin-dashboard")
			return
		}

		next.ServeHTTP(w, withAdminAuditContext(r, token))
	})
}

func withAuditIPContext(r *http.Request) *http.Request {
	ctx := audit.ContextWithIP(r.Context(), httputil.ClientIP(r))
	return r.WithContext(ctx)
}

func withAdminAuditContext(r *http.Request, token string) *http.Request {
	r = withAuditIPContext(r)
	if principal := adminAuditPrincipal(token); principal != "" {
		ctx := audit.ContextWithPrincipal(r.Context(), principal)
		r = r.WithContext(ctx)
	}
	return r
}

func adminAuditPrincipal(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("ayb-admin:"+token)).String()
}

// ResetAdminPassword generates a new random admin password and returns it.
// The new password takes effect immediately for subsequent login attempts.
// Existing admin tokens are invalidated (new HMAC secret is generated).
func (s *Server) ResetAdminPassword() (string, error) {
	s.adminMu.RLock()
	configured := s.adminAuth != nil
	s.adminMu.RUnlock()

	if !configured {
		return "", fmt.Errorf("admin auth not configured")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating password: %w", err)
	}
	pw := hex.EncodeToString(b)

	s.adminMu.Lock()
	s.adminAuth = newAdminAuth(pw)
	s.adminMu.Unlock()

	return pw, nil
}

// isAdminToken checks whether the bearer token in the request is a valid admin token.
func (s *Server) isAdminToken(r *http.Request) bool {
	s.adminMu.RLock()
	aa := s.adminAuth
	s.adminMu.RUnlock()

	if aa == nil {
		return false
	}
	token, ok := httputil.ExtractBearerToken(r)
	return ok && aa.validateToken(token)
}

// requireAdminOrUserAuth returns middleware that accepts either a valid admin
// HMAC token or a valid user JWT / API key.  This is used on the auto-generated
// CRUD API so that the admin dashboard (which holds an admin token) can read and
// write collection data when user-auth is enabled.
func (s *Server) requireAdminOrUserAuth(authSvc *auth.Service) func(http.Handler) http.Handler {
	userProtected := s.requireUserAuthWithRateLimit(authSvc)

	return func(next http.Handler) http.Handler {
		// Prepare the standard user-auth middleware chain once.
		userNext := userProtected(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = withAuditIPContext(r)
			// Fast path: admin token bypasses user-auth entirely.
			if s.isAdminToken(r) {
				token, _ := httputil.ExtractBearerToken(r)
				next.ServeHTTP(w, withAdminAuditContext(r, token))
				return
			}
			// Fall back to the standard user-auth middleware chain.
			userNext.ServeHTTP(w, r)
		})
	}
}

// requireUserAuthWithRateLimit returns middleware that enforces user authentication while applying rate limiting to both authenticated and anonymous requests. The middleware extracts user claims for valid tokens, applies app and API-route rate limiters, and then enforces strict authentication, enabling per-user rate limit buckets.
func (s *Server) requireUserAuthWithRateLimit(authSvc *auth.Service) func(http.Handler) http.Handler {
	userAuth := auth.RequireAuth(authSvc)
	optionalAuth := auth.OptionalAuth(authSvc)
	return func(next http.Handler) http.Handler {
		// Run anonymous/app rate limiters before strict auth so unauthenticated
		// requests are still rate-limited. OptionalAuth populates claims for
		// valid tokens, enabling per-user buckets before RequireAuth enforces auth.
		userNext := userAuth(next)
		if s.appRL != nil {
			userNext = s.appRL.Middleware(userNext)
		}
		if s.apiRL != nil || s.apiAnonRL != nil {
			userNext = APIRouteRateLimitMiddleware(s.apiRL, s.apiAnonRL, s.apiRateLimit, s.apiAnonRateLimit)(userNext)
		}
		userNext = optionalAuth(userNext)
		return userNext
	}
}
