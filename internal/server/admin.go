package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
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
// When admin.password is not set, all requests pass through.
func (s *Server) requireAdminToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.adminMu.RLock()
		aa := s.adminAuth
		s.adminMu.RUnlock()

		if aa == nil {
			next.ServeHTTP(w, r)
			return
		}

		token, ok := httputil.ExtractBearerToken(r)
		if !ok || !aa.validateToken(token) {
			httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized, "admin authentication required",
				"https://allyourbase.io/guide/admin-dashboard")
			return
		}

		next.ServeHTTP(w, r)
	})
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
	userAuth := auth.RequireAuth(authSvc)
	return func(next http.Handler) http.Handler {
		userNext := next
		if s.appRL != nil {
			userNext = s.appRL.Middleware(userNext)
		}
		userHandler := userAuth(userNext)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fast path: admin token bypasses user-auth entirely.
			if s.isAdminToken(r) {
				next.ServeHTTP(w, r)
				return
			}
			// Fall back to the standard user-auth middleware chain.
			userHandler.ServeHTTP(w, r)
		})
	}
}
