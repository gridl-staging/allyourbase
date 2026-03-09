package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

type ctxKey struct{}

// RequireAuth returns middleware that rejects requests without a valid JWT or API key.
func RequireAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearerToken(r)
			if !ok {
				httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized,
					"missing or invalid authorization header",
					"https://allyourbase.io/guide/authentication")
				return
			}

			claims, err := validateTokenOrAPIKey(r.Context(), svc, token)
			if err != nil {
				httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized,
					"invalid or expired token",
					"https://allyourbase.io/guide/authentication")
				return
			}

			if claims.MFAPending {
				httputil.WriteError(w, http.StatusUnauthorized, "MFA verification required")
				return
			}

			ctx := context.WithValue(r.Context(), ctxKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth returns middleware that extracts JWT or API key claims if present
// but does not reject unauthenticated requests.
func OptionalAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token, ok := extractBearerToken(r); ok {
				if claims, err := validateTokenOrAPIKey(r.Context(), svc, token); err == nil && !claims.MFAPending {
					ctx := context.WithValue(r.Context(), ctxKey{}, claims)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireMFAPending returns middleware that accepts only MFA pending tokens.
// This is the inverse of RequireAuth — it requires mfa_pending: true.
func RequireMFAPending(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearerToken(r)
			if !ok {
				httputil.WriteError(w, http.StatusUnauthorized, "no MFA challenge pending")
				return
			}

			claims, err := svc.ValidateToken(token)
			if err != nil || !claims.MFAPending {
				httputil.WriteError(w, http.StatusUnauthorized, "no MFA challenge pending")
				return
			}

			ctx := context.WithValue(r.Context(), mfaPendingCtxKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type mfaPendingCtxKey struct{}

// mfaPendingClaimsFromContext retrieves MFA pending claims from the request context.
func mfaPendingClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(mfaPendingCtxKey{}).(*Claims)
	return claims
}

// ClaimsFromContext retrieves auth claims from the request context.
// Returns nil if no claims are present.
func ClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(ctxKey{}).(*Claims)
	return claims
}

// ContextWithClaims returns a new context with the given claims attached.
// This is primarily useful for testing.
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, ctxKey{}, claims)
}

// validateTokenOrAPIKey checks if the token is an OAuth access token (ayb_at_ prefix),
// an API key (ayb_ prefix), or a JWT, and validates accordingly.
// OAuth token check comes first since ayb_at_ also matches the ayb_ API key prefix.
func validateTokenOrAPIKey(ctx context.Context, svc *Service, token string) (*Claims, error) {
	if IsOAuthAccessToken(token) {
		info, err := svc.ValidateOAuthToken(ctx, token)
		if err != nil {
			return nil, err
		}
		return oauthTokenInfoToClaims(info), nil
	}
	if IsAPIKey(token) {
		return svc.ValidateAPIKey(ctx, token)
	}
	return svc.ValidateToken(token)
}

// oauthTokenInfoToClaims converts OAuth token info into Claims for downstream handlers.
func oauthTokenInfoToClaims(info *OAuthTokenInfo) *Claims {
	claims := &Claims{
		APIKeyScope:        info.Scope,
		AllowedTables:      info.AllowedTables,
		AppID:              info.AppID,
		AppRateLimitRPS:    info.AppRateLimitRPS,
		AppRateLimitWindow: info.AppRateLimitWindowSeconds,
	}
	if info.UserID != nil {
		claims.Subject = *info.UserID
	}
	return claims
}

// ErrScopeReadOnly is returned when a readonly API key attempts a write operation.
var ErrScopeReadOnly = errors.New("api key scope does not permit write operations")

// ErrScopeTableDenied is returned when an API key is not allowed to access a table.
var ErrScopeTableDenied = errors.New("api key scope does not permit access to this table")

// CheckWriteScope verifies that the current claims allow write operations.
// Returns nil for JWT tokens (no scope) and full-access API keys.
func CheckWriteScope(claims *Claims) error {
	if claims == nil {
		return nil
	}
	if !claims.IsWriteAllowed() {
		return ErrScopeReadOnly
	}
	return nil
}

// CheckTableScope verifies that the current claims allow access to the given table.
// Returns nil for JWT tokens (no scope) and API keys with no table restrictions.
func CheckTableScope(claims *Claims, table string) error {
	if claims == nil {
		return nil
	}
	if !claims.IsTableAllowed(table) {
		return ErrScopeTableDenied
	}
	return nil
}

func extractBearerToken(r *http.Request) (string, bool) {
	return httputil.ExtractBearerToken(r)
}
