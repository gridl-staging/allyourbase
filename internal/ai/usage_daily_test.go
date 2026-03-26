package ai

import (
	"testing"
	"time"
)

func TestAggregateDailyUsageRows_MultiProviderModelDay(t *testing.T) {
	logs := []CallLog{
		{Provider: "openai", Model: "gpt-4o", InputTokens: 10, OutputTokens: 5, CostUSD: 0.01, CreatedAt: time.Date(2026, 2, 27, 1, 0, 0, 0, time.UTC)},
		{Provider: "openai", Model: "gpt-4o", InputTokens: 20, OutputTokens: 10, CostUSD: 0.02, CreatedAt: time.Date(2026, 2, 27, 2, 0, 0, 0, time.UTC)},
		{Provider: "openai", Model: "text-embedding-3-small", InputTokens: 30, OutputTokens: 0, CostUSD: 0.03, CreatedAt: time.Date(2026, 2, 27, 3, 0, 0, 0, time.UTC)},
		{Provider: "anthropic", Model: "claude-sonnet", InputTokens: 40, OutputTokens: 20, CostUSD: 0.04, CreatedAt: time.Date(2026, 2, 28, 1, 0, 0, 0, time.UTC)},
	}

	rows := AggregateDailyUsageRows(logs)
	if len(rows) != 3 {
		t.Fatalf("rows = %d; want 3", len(rows))
	}

	idx := map[string]DailyUsage{}
	for _, r := range rows {
		idx[r.Day.Format("2006-01-02")+"|"+r.Provider+"|"+r.Model] = r
	}

	r := idx["2026-02-27|openai|gpt-4o"]
	if r.Calls != 2 || r.InputTokens != 30 || r.OutputTokens != 15 || r.TotalTokens != 45 {
		t.Fatalf("unexpected openai/gpt-4o row: %+v", r)
	}

	r = idx["2026-02-27|openai|text-embedding-3-small"]
	if r.Calls != 1 || r.TotalTokens != 30 {
		t.Fatalf("unexpected embedding row: %+v", r)
	}

	r = idx["2026-02-28|anthropic|claude-sonnet"]
	if r.Calls != 1 || r.TotalTokens != 60 {
		t.Fatalf("unexpected anthropic row: %+v", r)
	}
}

func TestMergeDailyUsageRows_IdempotentRerun(t *testing.T) {
	base := []DailyUsage{{
		Day:         time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
		Provider:    "openai",
		Model:       "gpt-4o",
		Calls:       10,
		InputTokens: 100,
	}}

	merged := MergeDailyUsageRows(nil, base)
	merged = MergeDailyUsageRows(merged, base)

	if len(merged) != 1 {
		t.Fatalf("merged rows = %d; want 1", len(merged))
	}
	if merged[0].Calls != 10 || merged[0].InputTokens != 100 {
		t.Fatalf("idempotent merge failed: %+v", merged[0])
	}
}
