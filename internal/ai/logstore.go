// Package ai manages persistence and analytics for AI provider calls, tracking individual call logs and computing usage statistics.
package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CallLog records a single AI provider call.
type CallLog struct {
	ID             uuid.UUID  `json:"id"`
	Provider       string     `json:"provider"`
	Model          string     `json:"model"`
	InputTokens    int        `json:"input_tokens"`
	OutputTokens   int        `json:"output_tokens"`
	CostUSD        float64    `json:"cost_usd"`
	DurationMs     int        `json:"duration_ms"`
	Status         string     `json:"status"` // "success" or "error"
	ErrorMessage   string     `json:"error_message,omitempty"`
	EdgeFunctionID *uuid.UUID `json:"edge_function_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ListFilter controls pagination and filtering of call logs.
type ListFilter struct {
	Page     int
	PerPage  int
	Provider string
	Status   string
	From     time.Time
	To       time.Time
}

// UsageSummary aggregates AI usage over a time range.
type UsageSummary struct {
	TotalCalls        int                      `json:"total_calls"`
	TotalInputTokens  int                      `json:"total_input_tokens"`
	TotalOutputTokens int                      `json:"total_output_tokens"`
	TotalTokens       int                      `json:"total_tokens"`
	TotalCostUSD      float64                  `json:"total_cost_usd"`
	ErrorCount        int                      `json:"error_count"`
	ByProvider        map[string]ProviderUsage `json:"by_provider"`
}

// ProviderUsage holds per-provider aggregated usage.
type ProviderUsage struct {
	Calls        int     `json:"calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	ErrorCount   int     `json:"error_count"`
}

// DailyUsage stores day/provider/model rollups.
type DailyUsage struct {
	Day          time.Time `json:"day"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Calls        int       `json:"calls"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	TotalCostUSD float64   `json:"total_cost_usd"`
}

// LogStore persists and queries AI call logs.
type LogStore interface {
	Insert(ctx context.Context, log CallLog) error
	List(ctx context.Context, filter ListFilter) ([]CallLog, int, error)
	UsageSummary(ctx context.Context, from, to time.Time) (UsageSummary, error)
}

// PgLogStore implements LogStore backed by a pgx pool.
type PgLogStore struct {
	pool *pgxpool.Pool
}

// NewPgLogStore creates a PostgreSQL-backed log store.
func NewPgLogStore(pool *pgxpool.Pool) *PgLogStore {
	return &PgLogStore{pool: pool}
}

func (s *PgLogStore) Insert(ctx context.Context, log CallLog) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_ai_call_log (provider, model, input_tokens, output_tokens, cost_usd, duration_ms, status, error_message, edge_function_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		log.Provider, log.Model, log.InputTokens, log.OutputTokens,
		log.CostUSD, log.DurationMs, log.Status, log.ErrorMessage, log.EdgeFunctionID,
	)
	return err
}

// List returns a page of call logs matching the filter criteria along with the total count of matching logs. Filtering supports provider, status, and date range constraints; results are ordered by creation time in descending order.
func (s *PgLogStore) List(ctx context.Context, f ListFilter) ([]CallLog, int, error) {
	if f.PerPage <= 0 {
		f.PerPage = 50
	}
	if f.Page < 1 {
		f.Page = 1
	}
	offset := (f.Page - 1) * f.PerPage

	// Build WHERE clauses.
	where := "WHERE 1=1"
	args := []any{}
	argN := 0

	if f.Provider != "" {
		argN++
		where += fmt.Sprintf(" AND provider = $%d", argN)
		args = append(args, f.Provider)
	}
	if f.Status != "" {
		argN++
		where += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, f.Status)
	}
	if !f.From.IsZero() {
		argN++
		where += fmt.Sprintf(" AND created_at >= $%d", argN)
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		argN++
		where += fmt.Sprintf(" AND created_at <= $%d", argN)
		args = append(args, f.To)
	}

	// Count.
	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM _ayb_ai_call_log "+where, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting ai logs: %w", err)
	}

	// Fetch page.
	argN++
	limitArg := argN
	argN++
	offsetArg := argN
	query := fmt.Sprintf(
		"SELECT id, provider, model, input_tokens, output_tokens, cost_usd, duration_ms, status, error_message, edge_function_id, created_at FROM _ayb_ai_call_log %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, limitArg, offsetArg,
	)
	args = append(args, f.PerPage, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying ai logs: %w", err)
	}
	defer rows.Close()

	var logs []CallLog
	for rows.Next() {
		var l CallLog
		if err := rows.Scan(&l.ID, &l.Provider, &l.Model, &l.InputTokens, &l.OutputTokens, &l.CostUSD, &l.DurationMs, &l.Status, &l.ErrorMessage, &l.EdgeFunctionID, &l.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning ai log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// UsageSummary returns aggregated usage statistics for the time period, including call counts, token usage, costs, and error counts grouped by provider.
func (s *PgLogStore) UsageSummary(ctx context.Context, from, to time.Time) (UsageSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT provider,
		        COUNT(*) AS calls,
		        COALESCE(SUM(input_tokens), 0) AS input_tokens,
		        COALESCE(SUM(output_tokens), 0) AS output_tokens,
		        COALESCE(SUM(cost_usd), 0) AS total_cost_usd,
		        COUNT(*) FILTER (WHERE status = 'error') AS error_count
		 FROM _ayb_ai_call_log
		 WHERE created_at >= $1 AND created_at <= $2
		 GROUP BY provider`,
		from, to,
	)
	if err != nil {
		return UsageSummary{}, fmt.Errorf("querying usage summary: %w", err)
	}
	defer rows.Close()

	summary := UsageSummary{ByProvider: make(map[string]ProviderUsage)}
	for rows.Next() {
		var provider string
		var pu ProviderUsage
		if err := rows.Scan(&provider, &pu.Calls, &pu.InputTokens, &pu.OutputTokens, &pu.TotalCostUSD, &pu.ErrorCount); err != nil {
			return UsageSummary{}, fmt.Errorf("scanning usage row: %w", err)
		}
		summary.TotalCalls += pu.Calls
		summary.TotalInputTokens += pu.InputTokens
		summary.TotalOutputTokens += pu.OutputTokens
		pu.TotalTokens = pu.InputTokens + pu.OutputTokens
		summary.TotalTokens += pu.TotalTokens
		summary.TotalCostUSD += pu.TotalCostUSD
		summary.ErrorCount += pu.ErrorCount
		summary.ByProvider[provider] = pu
	}
	return summary, rows.Err()
}

// AggregateDailyUsage upserts daily provider/model usage totals for the given UTC day.
func (s *PgLogStore) AggregateDailyUsage(ctx context.Context, day time.Time) (int64, error) {
	dayUTC := day.UTC()
	start := time.Date(dayUTC.Year(), dayUTC.Month(), dayUTC.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	tag, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_ai_usage_daily (
			day, provider, model, calls, input_tokens, output_tokens, total_tokens, total_cost_usd, updated_at
		)
		SELECT
			$1::date AS day,
			provider,
			model,
			COUNT(*)::bigint AS calls,
			COALESCE(SUM(input_tokens), 0)::bigint AS input_tokens,
			COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
			(COALESCE(SUM(input_tokens), 0) + COALESCE(SUM(output_tokens), 0))::bigint AS total_tokens,
			COALESCE(SUM(cost_usd), 0)::numeric(20,6) AS total_cost_usd,
			NOW()
		FROM _ayb_ai_call_log
		WHERE created_at >= $2 AND created_at < $3
		GROUP BY provider, model
		ON CONFLICT (day, provider, model) DO UPDATE
		SET calls = EXCLUDED.calls,
		    input_tokens = EXCLUDED.input_tokens,
		    output_tokens = EXCLUDED.output_tokens,
		    total_tokens = EXCLUDED.total_tokens,
		    total_cost_usd = EXCLUDED.total_cost_usd,
		    updated_at = NOW()`,
		start, start, end,
	)
	if err != nil {
		return 0, fmt.Errorf("aggregating ai daily usage: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DailyUsage returns aggregated usage rows from _ayb_ai_usage_daily.
func (s *PgLogStore) DailyUsage(ctx context.Context, from, to time.Time) ([]DailyUsage, error) {
	if from.IsZero() {
		from = time.Now().UTC().AddDate(0, 0, -30)
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	rows, err := s.pool.Query(ctx,
		`SELECT day, provider, model, calls, input_tokens, output_tokens, total_tokens, total_cost_usd
		 FROM _ayb_ai_usage_daily
		 WHERE day >= $1::date AND day <= $2::date
		 ORDER BY day DESC, provider, model`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("querying daily usage: %w", err)
	}
	defer rows.Close()

	var out []DailyUsage
	for rows.Next() {
		var row DailyUsage
		if err := rows.Scan(&row.Day, &row.Provider, &row.Model, &row.Calls, &row.InputTokens, &row.OutputTokens, &row.TotalTokens, &row.TotalCostUSD); err != nil {
			return nil, fmt.Errorf("scanning daily usage row: %w", err)
		}
		out = append(out, row)
	}
	if out == nil {
		out = []DailyUsage{}
	}
	return out, rows.Err()
}
