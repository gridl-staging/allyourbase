// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/server/admin_audit.go.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/google/uuid"
)

type adminAuditEntry struct {
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	UserID    *string         `json:"user_id,omitempty"`
	APIKeyID  *string         `json:"api_key_id,omitempty"`
	TableName string          `json:"table_name"`
	RecordID  json.RawMessage `json:"record_id,omitempty"`
	Operation string          `json:"operation"`
	OldValues json.RawMessage `json:"old_values,omitempty"`
	NewValues json.RawMessage `json:"new_values,omitempty"`
	IPAddress *string         `json:"ip_address,omitempty"`
}

type adminAuditListResponse struct {
	Items  []adminAuditEntry `json:"items"`
	Count  int               `json:"count"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

type adminAuditFilters struct {
	tableName  string
	userID     string
	operation  string
	limit      int
	offset     int
	fromTime   time.Time
	toTime     time.Time
	toDateOnly bool
}

const (
	defaultAdminListLimit = 100
	maxAdminListLimit     = 500
)

type adminTimeRange struct {
	fromTime   time.Time
	toTime     time.Time
	toDateOnly bool
}

// handleAdminAudit serves HTTP requests to query the audit log with optional filters for table name, user ID, operation type, and timestamp range, with pagination via limit and offset, returning results as JSON.
func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	if s.pool == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	filters, badRequestMessage := parseAdminAuditFilters(r.URL.Query())
	if badRequestMessage != "" {
		httputil.WriteError(w, http.StatusBadRequest, badRequestMessage)
		return
	}

	sql, args := buildAdminAuditQuery(filters)
	rows, err := s.pool.Query(r.Context(), sql, args...)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query audit log")
		return
	}
	defer rows.Close()

	items := make([]adminAuditEntry, 0, filters.limit)
	for rows.Next() {
		var item adminAuditEntry
		var recordIDText, oldValuesText, newValuesText *string
		if err := rows.Scan(
			&item.ID,
			&item.Timestamp,
			&item.UserID,
			&item.APIKeyID,
			&item.TableName,
			&recordIDText,
			&item.Operation,
			&oldValuesText,
			&newValuesText,
			&item.IPAddress,
		); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to decode audit log row")
			return
		}
		if recordIDText != nil {
			item.RecordID = json.RawMessage(*recordIDText)
		}
		if oldValuesText != nil {
			item.OldValues = json.RawMessage(*oldValuesText)
		}
		if newValuesText != nil {
			item.NewValues = json.RawMessage(*newValuesText)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to iterate audit log rows")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, adminAuditListResponse{
		Items:  items,
		Count:  len(items),
		Limit:  filters.limit,
		Offset: filters.offset,
	})
}

// TODO: Document parseAdminAuditFilters.
func parseAdminAuditFilters(query url.Values) (adminAuditFilters, string) {
	filters := adminAuditFilters{
		tableName: strings.TrimSpace(query.Get("table")),
		operation: strings.ToUpper(strings.TrimSpace(query.Get("operation"))),
		limit:     defaultAdminListLimit,
	}

	userID, err := normalizeAuditUserID(query.Get("user_id"))
	if err != nil {
		return filters, "invalid user_id filter; must be a UUID"
	}
	filters.userID = userID

	if filters.operation != "" {
		switch filters.operation {
		case "INSERT", "UPDATE", "DELETE":
		default:
			return filters, "invalid operation filter; must be one of: INSERT, UPDATE, DELETE"
		}
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

// TODO: Document buildAdminAuditQuery.
func buildAdminAuditQuery(filters adminAuditFilters) (string, []any) {
	sql := `
		SELECT id::text, timestamp, user_id::text, api_key_id::text, table_name,
		       record_id::text, operation, old_values::text, new_values::text, host(ip_address)
		FROM _ayb_audit_log`
	whereClauses := make([]string, 0, 6)
	args := make([]any, 0, 8)
	argPos := 1

	if filters.tableName != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("table_name = $%d", argPos))
		args = append(args, filters.tableName)
		argPos++
	}
	if filters.userID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("user_id = $%d::uuid", argPos))
		args = append(args, filters.userID)
		argPos++
	}
	if filters.operation != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("operation = $%d", argPos))
		args = append(args, filters.operation)
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
	sql += fmt.Sprintf("\nORDER BY timestamp DESC, id DESC LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, filters.limit, filters.offset)
	return sql, args
}

func parseAuditTimeFilter(raw string) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	if parsedDate, err := time.Parse("2006-01-02", raw); err == nil {
		return parsedDate.UTC(), true, nil
	}
	parsedTime, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false, err
	}
	return parsedTime.UTC(), false, nil
}

// TODO: Document parseAdminListPagination.
func parseAdminListPagination(query url.Values) (int, int, string) {
	limit := defaultAdminListLimit
	offset := 0

	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return 0, 0, "invalid limit"
		}
		if parsedLimit <= 0 {
			parsedLimit = defaultAdminListLimit
		}
		if parsedLimit > maxAdminListLimit {
			parsedLimit = maxAdminListLimit
		}
		limit = parsedLimit
	}

	if rawOffset := strings.TrimSpace(query.Get("offset")); rawOffset != "" {
		parsedOffset, err := strconv.Atoi(rawOffset)
		if err != nil {
			return 0, 0, "invalid offset"
		}
		if parsedOffset < 0 {
			parsedOffset = 0
		}
		offset = parsedOffset
	}

	return limit, offset, ""
}

// TODO: Document parseAdminTimeRange.
func parseAdminTimeRange(query url.Values) (adminTimeRange, string) {
	fromTime, _, err := parseAuditTimeFilter(query.Get("from"))
	if err != nil {
		return adminTimeRange{}, "invalid from filter; expected RFC3339 or YYYY-MM-DD"
	}

	toTime, toDateOnly, err := parseAuditTimeFilter(query.Get("to"))
	if err != nil {
		return adminTimeRange{}, "invalid to filter; expected RFC3339 or YYYY-MM-DD"
	}
	if !fromTime.IsZero() && !toTime.IsZero() && toTime.Before(fromTime) {
		return adminTimeRange{}, "to filter must be greater than or equal to from filter"
	}

	return adminTimeRange{
		fromTime:   fromTime,
		toTime:     toTime,
		toDateOnly: toDateOnly,
	}, ""
}

func normalizeAuditUserID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
