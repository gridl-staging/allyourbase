// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/server/admin_request_logs_handler.go.
package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
)

type adminRequestLogEntry struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	StatusCode   int       `json:"status_code"`
	DurationMS   int64     `json:"duration_ms"`
	UserID       *string   `json:"user_id,omitempty"`
	APIKeyID     *string   `json:"api_key_id,omitempty"`
	RequestSize  int64     `json:"request_size"`
	ResponseSize int64     `json:"response_size"`
	IPAddress    *string   `json:"ip_address,omitempty"`
	RequestID    *string   `json:"request_id,omitempty"`
}

type adminRequestLogListResponse struct {
	Items  []adminRequestLogEntry `json:"items"`
	Count  int                    `json:"count"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
}

type adminRequestLogFilters struct {
	method     string
	path       string
	statusCode int
	limit      int
	offset     int
	fromTime   time.Time
	toTime     time.Time
	toDateOnly bool
}

// handleAdminRequestLogs returns filtered and paginated request logs with support for filtering by HTTP method, path pattern (with * wildcards), status code (100-599), and timestamp range (RFC3339 or YYYY-MM-DD format). Query parameters are validated; invalid inputs return HTTP 400 errors.
func (s *Server) handleAdminRequestLogs(w http.ResponseWriter, r *http.Request) {
	filters, badRequestMessage := parseAdminRequestLogFilters(r.URL.Query())
	if badRequestMessage != "" {
		httputil.WriteError(w, http.StatusBadRequest, badRequestMessage)
		return
	}

	if s.pool == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	sql, args := buildAdminRequestLogsQuery(filters)
	rows, err := s.pool.Query(r.Context(), sql, args...)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query request logs")
		return
	}
	defer rows.Close()

	items := make([]adminRequestLogEntry, 0, filters.limit)
	for rows.Next() {
		var item adminRequestLogEntry
		if err := rows.Scan(
			&item.ID, &item.Timestamp, &item.Method, &item.Path,
			&item.StatusCode, &item.DurationMS,
			&item.UserID, &item.APIKeyID,
			&item.RequestSize, &item.ResponseSize,
			&item.IPAddress, &item.RequestID,
		); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to decode request log row")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to iterate request log rows")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, adminRequestLogListResponse{
		Items:  items,
		Count:  len(items),
		Limit:  filters.limit,
		Offset: filters.offset,
	})
}

// TODO: Document parseAdminRequestLogFilters.
func parseAdminRequestLogFilters(query url.Values) (adminRequestLogFilters, string) {
	filters := adminRequestLogFilters{
		method: strings.ToUpper(strings.TrimSpace(query.Get("method"))),
		path:   strings.TrimSpace(query.Get("path")),
		limit:  defaultAdminListLimit,
	}

	if rawStatus := strings.TrimSpace(query.Get("status")); rawStatus != "" {
		parsed, err := strconv.Atoi(rawStatus)
		if err != nil || parsed < 100 || parsed > 599 {
			return filters, "invalid status; must be an integer 100–599"
		}
		filters.statusCode = parsed
	}

	limit, offset, errMsg := parseAdminListPagination(query)
	if errMsg != "" {
		return filters, errMsg
	}
	filters.limit = limit
	filters.offset = offset

	timeRange, errMsg := parseAdminTimeRange(query)
	if errMsg != "" {
		return filters, errMsg
	}
	filters.fromTime = timeRange.fromTime
	filters.toTime = timeRange.toTime
	filters.toDateOnly = timeRange.toDateOnly

	return filters, ""
}

// TODO: Document buildAdminRequestLogsQuery.
func buildAdminRequestLogsQuery(filters adminRequestLogFilters) (string, []any) {
	sql := `SELECT id, timestamp, method, path, status_code, duration_ms,
				user_id, api_key_id, request_size, response_size, host(ip_address), request_id
			FROM _ayb_request_logs`

	var whereClauses []string
	var args []any
	argPos := 1

	if filters.method != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("method = $%d", argPos))
		args = append(args, filters.method)
		argPos++
	}
	if filters.path != "" {
		clause, likePath, nextArgPos := buildPathLikeClause(filters.path, argPos)
		whereClauses = append(whereClauses, clause)
		args = append(args, likePath)
		argPos = nextArgPos
	}
	if filters.statusCode != 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("status_code = $%d", argPos))
		args = append(args, filters.statusCode)
		argPos++
	}
	if !filters.fromTime.IsZero() {
		whereClauses = append(whereClauses, fmt.Sprintf("timestamp >= $%d", argPos))
		args = append(args, filters.fromTime)
		argPos++
	}
	if !filters.toTime.IsZero() {
		if filters.toDateOnly {
			whereClauses = append(whereClauses, fmt.Sprintf("timestamp < $%d", argPos))
			args = append(args, filters.toTime.Add(24*time.Hour))
		} else {
			whereClauses = append(whereClauses, fmt.Sprintf("timestamp <= $%d", argPos))
			args = append(args, filters.toTime)
		}
		argPos++
	}

	if len(whereClauses) > 0 {
		sql += "\nWHERE " + strings.Join(whereClauses, " AND ")
	}
	sql += fmt.Sprintf("\nORDER BY timestamp DESC LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, filters.limit, filters.offset)
	return sql, args
}

func buildPathLikeClause(path string, argPos int) (string, string, int) {
	escaped := strings.NewReplacer(
		"\\", "\\\\",
		"%", "\\%",
		"_", "\\_",
	).Replace(path)
	// Users provide * wildcards in query params; convert to SQL LIKE.
	escaped = strings.ReplaceAll(escaped, "*", "%")
	return fmt.Sprintf("path LIKE $%d ESCAPE '\\'", argPos), escaped, argPos + 1
}
