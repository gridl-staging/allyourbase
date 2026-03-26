package migrate

import "strings"

// PlannedStatement represents a single SQL statement with a classification kind.
type PlannedStatement struct {
	Kind string
	SQL  string
}

// StatementPlan accumulates ordered SQL statements for a migration plan.
// Embed this in package-specific plan structs to avoid duplicating Add/JoinedSQL.
type StatementPlan struct {
	Statements []PlannedStatement
}

// Add appends a classified SQL statement to the plan.
func (p *StatementPlan) Add(kind, sql string) {
	p.Statements = append(p.Statements, PlannedStatement{Kind: kind, SQL: sql})
}

// JoinedSQL concatenates all statement SQL texts with newline separators.
func (p *StatementPlan) JoinedSQL() string {
	parts := make([]string, 0, len(p.Statements))
	for _, st := range p.Statements {
		parts = append(parts, st.SQL)
	}
	return strings.Join(parts, "\n")
}
