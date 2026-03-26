package migrate

import "testing"

func TestStatementPlanAdd(t *testing.T) {
	var p StatementPlan
	p.Add("table", "CREATE TABLE t1 (id int)")
	p.Add("policy", "CREATE POLICY p1 ON t1")

	if len(p.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(p.Statements))
	}
	if p.Statements[0].Kind != "table" || p.Statements[0].SQL != "CREATE TABLE t1 (id int)" {
		t.Errorf("statement 0 mismatch: %+v", p.Statements[0])
	}
	if p.Statements[1].Kind != "policy" || p.Statements[1].SQL != "CREATE POLICY p1 ON t1" {
		t.Errorf("statement 1 mismatch: %+v", p.Statements[1])
	}
}

func TestStatementPlanJoinedSQL(t *testing.T) {
	var p StatementPlan
	p.Add("table", "CREATE TABLE t1 (id int)")
	p.Add("index", "CREATE INDEX idx ON t1 (id)")

	got := p.JoinedSQL()
	want := "CREATE TABLE t1 (id int)\nCREATE INDEX idx ON t1 (id)"
	if got != want {
		t.Errorf("JoinedSQL() = %q, want %q", got, want)
	}
}

func TestStatementPlanEmptyJoinedSQL(t *testing.T) {
	var p StatementPlan
	if got := p.JoinedSQL(); got != "" {
		t.Errorf("empty plan JoinedSQL() = %q, want %q", got, "")
	}
}
