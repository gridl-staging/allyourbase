// Package server HTTP handlers and utility functions for analyzing query performance and suggesting indexes based on pg_stat_statements data.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/jackc/pgx/v5"
)

type adminQueryIndexSuggestion struct {
	Statement  string `json:"statement"`
	Confidence string `json:"confidence"`
}

type adminQueryStat struct {
	QueryID          string                      `json:"queryid"`
	Query            string                      `json:"query"`
	Calls            int64                       `json:"calls"`
	TotalExecTime    float64                     `json:"total_exec_time"`
	MeanExecTime     float64                     `json:"mean_exec_time"`
	Rows             int64                       `json:"rows"`
	SharedBlksHit    int64                       `json:"shared_blks_hit"`
	SharedBlksRead   int64                       `json:"shared_blks_read"`
	IndexSuggestions []adminQueryIndexSuggestion `json:"index_suggestions,omitempty"`
}

type adminQueryAnalyticsResponse struct {
	Items []adminQueryStat `json:"items"`
	Count int              `json:"count"`
	Limit int              `json:"limit"`
	Sort  string           `json:"sort"`
}

// handleAdminQueryAnalytics handles HTTP requests to the admin query analytics endpoint, returning query statistics from pg_stat_statements with index suggestions. It validates sort and limit parameters, checks that pg_stat_statements is installed, queries the extension, and generates index candidates for each query.
func (s *Server) handleAdminQueryAnalytics(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	sortBy := strings.TrimSpace(query.Get("sort"))
	sortClause, err := queryAnalyticsSortClause(sortBy)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid sort; must be one of: total_time, calls, mean_time")
		return
	}

	limit := 50
	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if parsedLimit <= 0 {
			parsedLimit = 50
		}
		if parsedLimit > 500 {
			parsedLimit = 500
		}
		limit = parsedLimit
	}

	if s.pool == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	enabled, err := pgStatStatementsEnabled(r.Context(), s)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to check pg_stat_statements extension")
		return
	}
	if !enabled {
		httputil.WriteError(w, http.StatusServiceUnavailable, "pg_stat_statements extension not enabled - see docs")
		return
	}

	sql := fmt.Sprintf(`SELECT queryid::text, query, calls, total_exec_time, mean_exec_time, rows, shared_blks_hit, shared_blks_read
		FROM pg_stat_statements
		ORDER BY %s
		LIMIT $1`, sortClause)

	rows, err := s.pool.Query(r.Context(), sql, limit)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "pg_stat_statements") {
			httputil.WriteError(w, http.StatusServiceUnavailable, "pg_stat_statements extension not enabled - see docs")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query pg_stat_statements")
		return
	}
	defer rows.Close()

	items := make([]adminQueryStat, 0, limit)
	for rows.Next() {
		var item adminQueryStat
		if err := rows.Scan(
			&item.QueryID,
			&item.Query,
			&item.Calls,
			&item.TotalExecTime,
			&item.MeanExecTime,
			&item.Rows,
			&item.SharedBlksHit,
			&item.SharedBlksRead,
		); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to decode pg_stat_statements row")
			return
		}
		item.IndexSuggestions = suggestIndexForQueryStat(item)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to iterate pg_stat_statements rows")
		return
	}

	if sortBy == "" {
		sortBy = "total_time"
	}

	httputil.WriteJSON(w, http.StatusOK, adminQueryAnalyticsResponse{
		Items: items,
		Count: len(items),
		Limit: limit,
		Sort:  sortBy,
	})
}

func pgStatStatementsEnabled(ctx context.Context, s *Server) (bool, error) {
	var installedVersion *string
	err := s.pool.QueryRow(
		ctx,
		`SELECT installed_version FROM pg_available_extensions WHERE name = 'pg_stat_statements'`,
	).Scan(&installedVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return installedVersion != nil && strings.TrimSpace(*installedVersion) != "", nil
}

func queryAnalyticsSortClause(sortBy string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "", "total_time":
		return "total_exec_time DESC", nil
	case "calls":
		return "calls DESC", nil
	case "mean_time":
		return "mean_exec_time DESC", nil
	default:
		return "", fmt.Errorf("unsupported sort value %q", sortBy)
	}
}

var tableAliasPattern = regexp.MustCompile(`(?i)\b(?:from|join)\s+([a-zA-Z_][a-zA-Z0-9_\.\"]*)(?:\s+(?:as\s+)?([a-zA-Z_][a-zA-Z0-9_]*))?`)
var qualifiedColumnPattern = regexp.MustCompile(`(?i)\b([a-zA-Z_][a-zA-Z0-9_]*)\.([a-zA-Z_][a-zA-Z0-9_]*)\s*(>=|<=|=|>|<|like\b|in\b)`) // nolint:lll

// suggestIndexForQueryStat analyzes a query statistic and returns suggested CREATE INDEX statements for columns appearing in WHERE or JOIN conditions. It extracts table aliases and qualified column references from the query, deduplicates candidates by table and column, and assigns a confidence level based on execution metrics.
func suggestIndexForQueryStat(stat adminQueryStat) []adminQueryIndexSuggestion {
	if !isLikelyIndexBound(stat) {
		return nil
	}

	aliases := parseTableAliases(stat.Query)
	candidateMap := map[string]struct{}{}
	candidates := make([][2]string, 0, 4)
	for _, match := range qualifiedColumnPattern.FindAllStringSubmatch(stat.Query, -1) {
		if len(match) < 3 {
			continue
		}
		alias := strings.ToLower(match[1])
		column := normalizeIdentifier(match[2])
		table := aliases[alias]
		if table == "" || column == "" {
			continue
		}
		key := table + "." + column
		if _, exists := candidateMap[key]; exists {
			continue
		}
		candidateMap[key] = struct{}{}
		candidates = append(candidates, [2]string{table, column})
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i][0] == candidates[j][0] {
			return candidates[i][1] < candidates[j][1]
		}
		return candidates[i][0] < candidates[j][0]
	})

	confidence := suggestionConfidence(stat)
	suggestions := make([]adminQueryIndexSuggestion, 0, len(candidates))
	for _, c := range candidates {
		suggestions = append(suggestions, adminQueryIndexSuggestion{
			Statement:  fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_%s_%s ON %s (%s);", c[0], c[1], c[0], c[1]),
			Confidence: confidence,
		})
	}
	return suggestions
}

// parseTableAliases extracts table names and their aliases from the FROM and JOIN clauses of a SQL query, returning a map that includes both alias->table and table->table entries. It normalizes identifiers to lowercase and handles schema-qualified table names.
func parseTableAliases(query string) map[string]string {
	out := make(map[string]string)
	for _, match := range tableAliasPattern.FindAllStringSubmatch(query, -1) {
		if len(match) < 2 {
			continue
		}
		table := normalizeTableName(match[1])
		if table == "" {
			continue
		}
		alias := table
		if len(match) > 2 && strings.TrimSpace(match[2]) != "" {
			alias = normalizeIdentifier(match[2])
		}
		out[alias] = table
		out[table] = table
	}
	return out
}

func normalizeTableName(raw string) string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return ""
	}
	parts := strings.Split(clean, ".")
	last := parts[len(parts)-1]
	return normalizeIdentifier(last)
}

func normalizeIdentifier(raw string) string {
	clean := strings.Trim(strings.TrimSpace(raw), `"`)
	return strings.ToLower(clean)
}

func isLikelyIndexBound(stat adminQueryStat) bool {
	if stat.TotalExecTime < 1000 || stat.Rows < 1000 {
		return false
	}
	totalBlocks := stat.SharedBlksHit + stat.SharedBlksRead
	if totalBlocks <= 0 {
		return true
	}
	cacheMissRatio := float64(stat.SharedBlksRead) / float64(totalBlocks)
	return cacheMissRatio >= 0.15
}

// suggestionConfidence returns a confidence level string (high, medium, or low) for an index suggestion based on the query's total execution time, row count, and cache miss ratio. Queries with higher execution time, more rows, and higher cache miss ratios receive higher confidence levels.
func suggestionConfidence(stat adminQueryStat) string {
	totalBlocks := stat.SharedBlksHit + stat.SharedBlksRead
	cacheMissRatio := 1.0
	if totalBlocks > 0 {
		cacheMissRatio = float64(stat.SharedBlksRead) / float64(totalBlocks)
	}

	switch {
	case stat.TotalExecTime >= 5000 && stat.Rows >= 5000 && cacheMissRatio >= 0.5:
		return "high"
	case stat.TotalExecTime >= 2000 && stat.Rows >= 2000 && cacheMissRatio >= 0.3:
		return "medium"
	default:
		return "low"
	}
}
