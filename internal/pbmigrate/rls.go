// Package pbmigrate Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar22_pm_03_pbmigrate_hardening/allyourbase_dev/internal/pbmigrate/rls.go.
package pbmigrate

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	authFieldRegex        = regexp.MustCompile(`@request\.auth\.(\w+)`)
	nestedAuthFieldRegex  = regexp.MustCompile(`@request\.auth\.\w+\.`)
	unsupportedTokenRegex = regexp.MustCompile(`@[A-Za-z0-9_.]+`)

	// pbArrayFilterOpRegex detects PocketBase array filter operators (?=, ?!=, ?~, ?!~)
	// which have no PostgreSQL equivalent and must not pass through as convertible SQL.
	pbArrayFilterOpRegex = regexp.MustCompile(`\?!?[=~]`)
)

// classifyRule classifies a PocketBase API rule for RLS conversion.
// nil = locked (admin-only), "" = open to all, otherwise attempts expression conversion.
// This is the single entry point for rule classification; all callers (GenerateRLSPolicies,
// countPolicies, ConvertRuleToRLS) must use this to ensure consistent diagnostic reporting.
func classifyRule(rule *string) RuleClassification {
	if rule == nil {
		return RuleClassification{Status: RuleStatusLocked, Diagnostic: "locked (admin-only)"}
	}
	if *rule == "" {
		return RuleClassification{Status: RuleStatusConvertible, PgExpr: "true", Rule: ""}
	}
	return convertRuleExpression(*rule)
}

// ConvertRuleToRLS converts a PocketBase API rule to a PostgreSQL RLS policy.
// It delegates to classifyRule for consistent classification.
func ConvertRuleToRLS(tableName string, action string, rule *string) (string, error) {
	cl := classifyRule(rule)
	switch cl.Status {
	case RuleStatusLocked:
		return "", nil
	case RuleStatusUnsupported:
		return "", fmt.Errorf("failed to convert rule: %s", cl.Diagnostic)
	default:
		return buildRLSPolicy(tableName, action, cl.PgExpr), nil
	}
}

// buildRLSPolicy generates the CREATE POLICY SQL statement
func buildRLSPolicy(tableName, action, expression string) string {
	policyName := fmt.Sprintf("%s_%s_policy", tableName, strings.ToLower(action))
	policyName = SanitizeIdentifier(policyName)
	tableName = SanitizeIdentifier(tableName)

	var cmd, clause string

	switch strings.ToUpper(action) {
	case "LIST", "VIEW":
		cmd = "SELECT"
		clause = "USING"
	case "CREATE":
		cmd = "INSERT"
		clause = "WITH CHECK"
	case "UPDATE":
		cmd = "UPDATE"
		clause = "USING"
	case "DELETE":
		cmd = "DELETE"
		clause = "USING"
	default:
		cmd = "ALL"
		clause = "USING"
	}

	return fmt.Sprintf("CREATE POLICY %s ON %s FOR %s %s (%s);",
		policyName, tableName, cmd, clause, expression)
}

// TODO: Document convertRuleExpression.
func convertRuleExpression(rule string) RuleClassification {
	// Nested traversals are ambiguous in PostgreSQL RLS and must be manually reviewed.
	if token := findFirstUnquotedRuleMatch(rule, nestedAuthFieldRegex); token != "" {
		return RuleClassification{
			Status:     RuleStatusUnsupported,
			Rule:       rule,
			Diagnostic: fmt.Sprintf("unsupported nested @request.auth field access %q", token),
		}
	}

	// PocketBase array filter operators (?=, ?!=, ?~, ?!~) have no PostgreSQL
	// equivalent. Check BEFORE the rewrite step so != → <> doesn't mask ?!=.
	if op := findFirstUnquotedRuleMatch(rule, pbArrayFilterOpRegex); op != "" {
		return RuleClassification{
			Status:     RuleStatusUnsupported,
			Rule:       rule,
			Diagnostic: fmt.Sprintf("unsupported PocketBase array operator %q; manual review required", op),
		}
	}

	// Only transform unquoted segments so string literals like
	// 'user@example.com' or '@request.auth.id' remain intact.
	converted := rewriteUnquotedRuleSegments(rule, func(segment string) string {
		segment = strings.ReplaceAll(segment, "@request.auth.id", "current_setting('app.user_id', true)")
		segment = authFieldRegex.ReplaceAllStringFunc(segment, func(match string) string {
			field := authFieldRegex.FindStringSubmatch(match)[1]
			return fmt.Sprintf("(SELECT %s FROM %s WHERE id = current_setting('app.user_id', true))",
				SanitizeIdentifier(field), SanitizeIdentifier("ayb_auth_users"))
		})
		segment = strings.ReplaceAll(segment, "&&", "AND")
		segment = strings.ReplaceAll(segment, "||", "OR")
		segment = strings.ReplaceAll(segment, "!=", "<>")
		return segment
	})

	// Any remaining @token is unsupported PocketBase-specific syntax.
	if token := findUnsupportedPocketBaseToken(converted); token != "" {
		return RuleClassification{
			Status:     RuleStatusUnsupported,
			Rule:       rule,
			Diagnostic: fmt.Sprintf("unsupported PocketBase token %q; manual review required", token),
		}
	}
	if err := validateEmbeddedSQLExpression(converted); err != nil {
		return RuleClassification{
			Status:     RuleStatusUnsupported,
			Rule:       rule,
			Diagnostic: err.Error(),
		}
	}

	return RuleClassification{
		Status: RuleStatusConvertible,
		PgExpr: converted,
		Rule:   rule,
	}
}

// rewriteUnquotedRuleSegments applies a transform only to segments outside SQL
// single-quoted string literals and double-quoted identifiers.
func rewriteUnquotedRuleSegments(rule string, transform func(string) string) string {
	var builder strings.Builder
	builder.Grow(len(rule))

	start := 0
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(rule); i++ {
		switch rule[i] {
		case '\'':
			if inDoubleQuote {
				continue
			}
			if inSingleQuote && i+1 < len(rule) && rule[i+1] == '\'' {
				i++
				continue
			}
			if !inSingleQuote {
				builder.WriteString(transform(rule[start:i]))
				start = i
			}
			inSingleQuote = !inSingleQuote
			if !inSingleQuote {
				builder.WriteString(rule[start : i+1])
				start = i + 1
			}
		case '"':
			if inSingleQuote {
				continue
			}
			if inDoubleQuote && i+1 < len(rule) && rule[i+1] == '"' {
				i++
				continue
			}
			if !inDoubleQuote {
				builder.WriteString(transform(rule[start:i]))
				start = i
			}
			inDoubleQuote = !inDoubleQuote
			if !inDoubleQuote {
				builder.WriteString(rule[start : i+1])
				start = i + 1
			}
		}
	}

	if start < len(rule) {
		if inSingleQuote || inDoubleQuote {
			builder.WriteString(rule[start:])
		} else {
			builder.WriteString(transform(rule[start:]))
		}
	}

	return builder.String()
}

func findFirstUnquotedRuleMatch(rule string, re *regexp.Regexp) string {
	var match string
	rewriteUnquotedRuleSegments(rule, func(segment string) string {
		if match == "" {
			match = re.FindString(segment)
		}
		return segment
	})
	return match
}

// findUnsupportedPocketBaseToken returns the first PocketBase-style token that
// remains outside quoted SQL literals after known conversions have run.
func findUnsupportedPocketBaseToken(rule string) string {
	return findFirstUnquotedRuleMatch(rule, unsupportedTokenRegex)
}

// TODO: Document GenerateRLSPolicies.
func GenerateRLSPolicies(coll PBCollection) ([]string, []RLSDiagnostic, error) {
	var policies []string
	var diags []RLSDiagnostic
	var firstErr error

	tableName := coll.Name

	for _, a := range ruleActions(coll) {
		cl := classifyRule(a.rule)
		switch cl.Status {
		case RuleStatusConvertible:
			policies = append(policies, buildRLSPolicy(tableName, a.name, cl.PgExpr))
		case RuleStatusUnsupported:
			diags = append(diags, RLSDiagnostic{
				Collection: tableName,
				Action:     a.name,
				Rule:       cl.Rule,
				Message:    cl.Diagnostic,
			})
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to convert rule: %s", cl.Diagnostic)
			}
		}
		// RuleStatusLocked: no policy, no diagnostic
	}

	if firstErr != nil {
		return nil, diags, firstErr
	}
	return policies, nil, nil
}

// EnableRLS generates ALTER TABLE statement to enable RLS
func EnableRLS(tableName string) string {
	return fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY;", SanitizeIdentifier(tableName))
}
