package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// HasConsent checks if a user has previously consented to the given client request.
// The stored consent scope must cover the requested scope, and stored allowed_tables
// must cover requested allowed_tables when table restrictions are present.
func (s *Service) HasConsent(ctx context.Context, userID, clientID, scope string, allowedTables []string) (bool, error) {
	var consentScope string
	var consentAllowedTables []string
	err := s.pool.QueryRow(ctx,
		`SELECT scope, allowed_tables
		 FROM _ayb_oauth_consents
		 WHERE user_id = $1 AND client_id = $2`,
		userID, clientID,
	).Scan(&consentScope, &consentAllowedTables)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("checking consent: %w", err)
	}

	if !consentScopeCoversRequest(consentScope, scope) {
		return false, nil
	}
	if !consentAllowedTablesCoverRequest(consentAllowedTables, allowedTables) {
		return false, nil
	}
	return true, nil
}

// SaveConsent records that a user has consented to a client for a given scope.
// Uses INSERT ... ON CONFLICT to upsert.
func (s *Service) SaveConsent(ctx context.Context, userID, clientID, scope string, allowedTables []string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_consents (user_id, client_id, scope, allowed_tables)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, client_id) DO UPDATE
		 SET scope = $3, allowed_tables = $4, granted_at = NOW()`,
		userID, clientID, scope, allowedTables,
	)
	if err != nil {
		return fmt.Errorf("saving consent: %w", err)
	}
	return nil
}

func consentScopeCoversRequest(consentedScope, requestedScope string) bool {
	switch consentedScope {
	case ScopeFullAccess:
		return true
	case ScopeReadWrite:
		return requestedScope == ScopeReadWrite || requestedScope == ScopeReadOnly
	case ScopeReadOnly:
		return requestedScope == ScopeReadOnly
	default:
		return consentedScope == requestedScope
	}
}

// consentAllowedTablesCoverRequest reports whether the consented tables cover all requested tables. An empty consented set indicates unrestricted access was granted.
func consentAllowedTablesCoverRequest(consentedTables, requestedTables []string) bool {
	if len(consentedTables) == 0 {
		return true
	}
	if len(requestedTables) == 0 {
		return false
	}

	allowed := make(map[string]struct{}, len(consentedTables))
	for _, tableName := range consentedTables {
		allowed[tableName] = struct{}{}
	}
	for _, requestedTable := range requestedTables {
		if _, ok := allowed[requestedTable]; !ok {
			return false
		}
	}
	return true
}
