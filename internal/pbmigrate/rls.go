package pbmigrate

import (
	"fmt"
	"regexp"
	"strings"
)

// ConvertRuleToRLS converts a PocketBase API rule to a PostgreSQL RLS policy
func ConvertRuleToRLS(tableName string, action string, rule *string) (string, error) {
	if rule == nil {
		// null = locked (admin-only), no policy needed
		return "", nil
	}

	ruleExpr := *rule

	if ruleExpr == "" {
		// empty string = open to all
		return buildRLSPolicy(tableName, action, "true"), nil
	}

	// Convert PocketBase rule syntax to PostgreSQL RLS
	pgExpr, err := convertRuleExpression(ruleExpr)
	if err != nil {
		return "", fmt.Errorf("failed to convert rule: %w", err)
	}

	return buildRLSPolicy(tableName, action, pgExpr), nil
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

// convertRuleExpression translates PocketBase rule syntax to PostgreSQL
func convertRuleExpression(rule string) (string, error) {
	// Replace @request.auth.id with current_setting('app.user_id', true)
	rule = strings.ReplaceAll(rule, "@request.auth.id", "current_setting('app.user_id', true)")

	// Replace @request.auth.{field} with subquery
	authFieldRegex := regexp.MustCompile(`@request\.auth\.(\w+)`)
	rule = authFieldRegex.ReplaceAllStringFunc(rule, func(match string) string {
		field := authFieldRegex.FindStringSubmatch(match)[1]
		// For now, simple replacement - in production would need more sophisticated handling
		return fmt.Sprintf("(SELECT %s FROM ayb_auth_users WHERE id = current_setting('app.user_id', true))", field)
	})

	// Replace @collection.{name}.{field} with subquery
	collectionRegex := regexp.MustCompile(`@collection\.(\w+)\.(\w+)`)
	rule = collectionRegex.ReplaceAllStringFunc(rule, func(match string) string {
		parts := collectionRegex.FindStringSubmatch(match)
		collName := parts[1]
		field := parts[2]
		return fmt.Sprintf("(SELECT %s FROM %s WHERE id = current_setting('app.user_id', true))", field, collName)
	})

	// Replace && with AND
	rule = strings.ReplaceAll(rule, "&&", "AND")

	// Replace || with OR
	rule = strings.ReplaceAll(rule, "||", "OR")

	// Replace != with <>
	rule = strings.ReplaceAll(rule, "!=", "<>")

	return rule, nil
}

// GenerateRLSPolicies creates all RLS policies for a collection
func GenerateRLSPolicies(coll PBCollection) ([]string, error) {
	var policies []string

	tableName := coll.Name

	// SELECT policy - use ListRule (ViewRule is typically the same or more permissive)
	// In PocketBase, ListRule controls listing and ViewRule controls individual record access
	// In PostgreSQL, both map to SELECT, so we use the more restrictive ListRule
	if policy, err := ConvertRuleToRLS(tableName, "list", coll.ListRule); err != nil {
		return nil, err
	} else if policy != "" {
		policies = append(policies, policy)
	}

	// Create rule
	if policy, err := ConvertRuleToRLS(tableName, "create", coll.CreateRule); err != nil {
		return nil, err
	} else if policy != "" {
		policies = append(policies, policy)
	}

	// Update rule
	if policy, err := ConvertRuleToRLS(tableName, "update", coll.UpdateRule); err != nil {
		return nil, err
	} else if policy != "" {
		policies = append(policies, policy)
	}

	// Delete rule
	if policy, err := ConvertRuleToRLS(tableName, "delete", coll.DeleteRule); err != nil {
		return nil, err
	} else if policy != "" {
		policies = append(policies, policy)
	}

	return policies, nil
}

// EnableRLS generates ALTER TABLE statement to enable RLS
func EnableRLS(tableName string) string {
	return fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY;", SanitizeIdentifier(tableName))
}
