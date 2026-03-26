// Package ai Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/ai/assistant_response_parser.go.
package ai

import (
	"regexp"
	"strings"
)

var sqlBlockRegexp = regexp.MustCompile("(?is)```sql\\s*(.*?)```")
var anyCodeBlockRegexp = regexp.MustCompile("(?is)```.*?```")
var deleteStatementRegexp = regexp.MustCompile(`(?is)\bdelete\s+from\b[^;]*;?`)
var whereClauseRegexp = regexp.MustCompile(`(?is)\bwhere\b`)

// ParseAssistantResponseText extracts structured fields from provider text output.
func ParseAssistantResponseText(text string) AssistantParsedResponse {
	trimmed := strings.TrimSpace(text)
	sql := ""
	if match := sqlBlockRegexp.FindStringSubmatch(trimmed); len(match) == 2 {
		sql = strings.TrimSpace(match[1])
	}

	explanation := trimmed
	if sql != "" {
		explanation = strings.TrimSpace(sqlBlockRegexp.ReplaceAllString(trimmed, ""))
	} else {
		explanation = strings.TrimSpace(anyCodeBlockRegexp.ReplaceAllString(trimmed, ""))
	}

	warning := strings.Join(detectDestructiveWarnings(firstNonEmpty(sql, trimmed)), "; ")
	return AssistantParsedResponse{
		SQL:         sql,
		Explanation: explanation,
		Warning:     warning,
	}
}

// TODO: Document detectDestructiveWarnings.
func detectDestructiveWarnings(input string) []string {
	lower := strings.ToLower(input)
	warnings := make([]string, 0, 3)
	if strings.Contains(lower, "drop database") {
		warnings = append(warnings, "Never run DROP DATABASE in this environment.")
	}
	if strings.Contains(lower, "drop table") {
		warnings = append(warnings, "Destructive statement detected: DROP TABLE.")
	}
	if strings.Contains(lower, "truncate") {
		warnings = append(warnings, "Destructive statement detected: TRUNCATE.")
	}
	if hasDeleteWithoutWhere(lower) {
		warnings = append(warnings, "Potentially destructive statement detected: DELETE without WHERE.")
	}
	return warnings
}

func hasDeleteWithoutWhere(input string) bool {
	matches := deleteStatementRegexp.FindAllString(input, -1)
	for _, stmt := range matches {
		if !whereClauseRegexp.MatchString(stmt) {
			return true
		}
	}
	return false
}
