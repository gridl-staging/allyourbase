package ai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AssistantHistoryStore persists assistant query history entries.
type AssistantHistoryStore interface {
	Insert(ctx context.Context, entry AssistantHistoryEntry) (AssistantHistoryEntry, error)
	List(ctx context.Context, filter AssistantHistoryFilter) ([]AssistantHistoryEntry, int, error)
}

// PgAssistantHistoryStore is the PostgreSQL-backed assistant history store.
type PgAssistantHistoryStore struct {
	pool *pgxpool.Pool
}

// NewPgAssistantHistoryStore creates a PostgreSQL-backed assistant history store.
func NewPgAssistantHistoryStore(pool *pgxpool.Pool) *PgAssistantHistoryStore {
	return &PgAssistantHistoryStore{pool: pool}
}

// Insert persists a single assistant history entry and returns the stored row.
func (s *PgAssistantHistoryStore) Insert(ctx context.Context, entry AssistantHistoryEntry) (AssistantHistoryEntry, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_ai_assistant_history (
			mode, query_text, response_text, sql_text, explanation, warning,
			provider, model, status, duration_ms, input_tokens, output_tokens
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at`,
		entry.Mode, entry.QueryText, entry.ResponseText, entry.SQL, entry.Explanation, entry.Warning,
		entry.Provider, entry.Model, entry.Status, entry.DurationMs, entry.InputTokens, entry.OutputTokens,
	).Scan(&entry.ID, &entry.CreatedAt)
	if err != nil {
		return AssistantHistoryEntry{}, fmt.Errorf("inserting assistant history: %w", err)
	}
	return entry, nil
}

// List returns newest-first assistant history entries with optional mode filtering.
func (s *PgAssistantHistoryStore) List(ctx context.Context, filter AssistantHistoryFilter) ([]AssistantHistoryEntry, int, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	perPage := filter.PerPage
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 200 {
		perPage = 200
	}
	offset := (page - 1) * perPage

	args := make([]any, 0, 3)
	whereClause := ""
	if filter.Mode != "" {
		whereClause = "WHERE mode = $1"
		args = append(args, filter.Mode)
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM _ayb_ai_assistant_history " + whereClause
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting assistant history: %w", err)
	}

	listQuery := "SELECT id, mode, query_text, response_text, sql_text, explanation, warning, provider, model, status, duration_ms, input_tokens, output_tokens, created_at FROM _ayb_ai_assistant_history " + whereClause + " ORDER BY created_at DESC"
	if filter.Mode != "" {
		listQuery += " LIMIT $2 OFFSET $3"
		args = append(args, perPage, offset)
	} else {
		listQuery += " LIMIT $1 OFFSET $2"
		args = append(args, perPage, offset)
	}

	rows, err := s.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing assistant history: %w", err)
	}
	defer rows.Close()

	entries := make([]AssistantHistoryEntry, 0, perPage)
	for rows.Next() {
		var entry AssistantHistoryEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.Mode,
			&entry.QueryText,
			&entry.ResponseText,
			&entry.SQL,
			&entry.Explanation,
			&entry.Warning,
			&entry.Provider,
			&entry.Model,
			&entry.Status,
			&entry.DurationMs,
			&entry.InputTokens,
			&entry.OutputTokens,
			&entry.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning assistant history row: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating assistant history rows: %w", err)
	}
	if entries == nil {
		entries = []AssistantHistoryEntry{}
	}
	return entries, total, nil
}
