package server

import "testing"

func TestQueryAnalyticsSortClause(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{in: "", want: "total_exec_time DESC"},
		{in: "total_time", want: "total_exec_time DESC"},
		{in: "calls", want: "calls DESC"},
		{in: "mean_time", want: "mean_exec_time DESC"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := queryAnalyticsSortClause(tc.in)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestQueryAnalyticsSortClauseRejectsInvalid(t *testing.T) {
	t.Parallel()

	if _, err := queryAnalyticsSortClause("random"); err == nil {
		t.Fatal("expected error for invalid sort")
	}
}

func TestSuggestIndexForQueryStat(t *testing.T) {
	t.Parallel()

	stat := adminQueryStat{
		Query:          "SELECT * FROM orders o JOIN users u ON o.user_id = u.id WHERE o.status = $1",
		TotalExecTime:  9000,
		Rows:           12000,
		SharedBlksHit:  100,
		SharedBlksRead: 900,
	}

	suggestions := suggestIndexForQueryStat(stat)
	if len(suggestions) == 0 {
		t.Fatal("expected at least one index suggestion")
	}
	if suggestions[0].Confidence != "high" {
		t.Fatalf("confidence got %q want high", suggestions[0].Confidence)
	}
	if suggestions[0].Statement == "" {
		t.Fatal("expected create index statement")
	}
}

func TestSuggestIndexForQueryStatNoSuggestionForFastQuery(t *testing.T) {
	t.Parallel()

	stat := adminQueryStat{
		Query:          "SELECT * FROM orders WHERE id = $1",
		TotalExecTime:  10,
		Rows:           10,
		SharedBlksHit:  100,
		SharedBlksRead: 0,
	}

	suggestions := suggestIndexForQueryStat(stat)
	if len(suggestions) != 0 {
		t.Fatalf("expected no suggestions, got %d", len(suggestions))
	}
}
