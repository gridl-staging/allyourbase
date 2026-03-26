package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrSessionNotFound  = errors.New("session not found")
	ErrSessionForbidden = errors.New("session does not belong to user")
)

// SessionInfo represents a refresh-token-backed auth session.
type SessionInfo struct {
	ID           string    `json:"id"`
	UserAgent    string    `json:"user_agent"`
	IPAddress    string    `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	Current      bool      `json:"current"`
}

type sessionIDRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// ListSessions lists active sessions for a user and marks the current session.
func (s *Service) ListSessions(ctx context.Context, userID, currentSessionID string) ([]SessionInfo, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id,
		        COALESCE(user_agent, ''),
		        COALESCE(ip_address, ''),
		        created_at,
		        COALESCE(last_active_at, created_at)
		 FROM _ayb_sessions
		 WHERE user_id = $1 AND expires_at > NOW()
		 ORDER BY COALESCE(last_active_at, created_at) DESC, created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]SessionInfo, 0)
	for rows.Next() {
		var session SessionInfo
		if err := rows.Scan(
			&session.ID,
			&session.UserAgent,
			&session.IPAddress,
			&session.CreatedAt,
			&session.LastActiveAt,
		); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		session.Current = currentSessionID != "" && session.ID == currentSessionID
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sessions: %w", err)
	}

	return sessions, nil
}

// RevokeSession revokes a specific session for a user.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
	result, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_sessions WHERE id = $1 AND user_id = $2`,
		sessionID, userID,
	)
	if err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	if result.RowsAffected() > 0 {
		s.addSessionToDenyList(sessionID)
		return nil
	}

	var exists bool
	err = s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM _ayb_sessions WHERE id = $1)`,
		sessionID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking session existence: %w", err)
	}
	if exists {
		return ErrSessionForbidden
	}
	return ErrSessionNotFound
}

// RevokeAllExceptCurrent revokes every session except the current one.
func (s *Service) RevokeAllExceptCurrent(ctx context.Context, userID, currentSessionID string) error {
	if currentSessionID == "" {
		return fmt.Errorf("%w: current session id is required", ErrValidation)
	}
	rows, err := s.pool.Query(ctx,
		`DELETE FROM _ayb_sessions WHERE user_id = $1 AND id <> $2 RETURNING id`,
		userID, currentSessionID,
	)
	if err != nil {
		return fmt.Errorf("revoking sessions: %w", err)
	}
	return s.denyListFromSessionRows(rows)
}

// RevokeAllSessions revokes all sessions for a user.
func (s *Service) RevokeAllSessions(ctx context.Context, userID string) error {
	rows, err := s.pool.Query(ctx,
		`DELETE FROM _ayb_sessions WHERE user_id = $1 RETURNING id`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("revoking all sessions: %w", err)
	}
	return s.denyListFromSessionRows(rows)
}

func (s *Service) addSessionToDenyList(sessionID string) {
	if s.denyList == nil || sessionID == "" {
		return
	}
	s.denyList.Add(sessionID, s.tokenDur)
}

func (s *Service) denyListFromSessionRows(rows sessionIDRows) error {
	defer rows.Close()

	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return fmt.Errorf("scanning revoked session id: %w", err)
		}
		s.addSessionToDenyList(sessionID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating revoked session ids: %w", err)
	}
	return nil
}
