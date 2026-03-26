package ai

import (
	"sort"
	"time"
)

type dailyUsageKey struct {
	day      string
	provider string
	model    string
}

// AggregateDailyUsageRows aggregates call logs into UTC day/provider/model rows.
func AggregateDailyUsageRows(logs []CallLog) []DailyUsage {
	rowsByKey := make(map[dailyUsageKey]DailyUsage)
	for _, l := range logs {
		day := l.CreatedAt.UTC().Format("2006-01-02")
		key := dailyUsageKey{day: day, provider: l.Provider, model: l.Model}
		row := rowsByKey[key]
		if row.Day.IsZero() {
			parsedDay, _ := time.Parse("2006-01-02", day)
			row.Day = parsedDay.UTC()
			row.Provider = l.Provider
			row.Model = l.Model
		}
		row.Calls++
		row.InputTokens += l.InputTokens
		row.OutputTokens += l.OutputTokens
		row.TotalTokens += l.InputTokens + l.OutputTokens
		row.TotalCostUSD += l.CostUSD
		rowsByKey[key] = row
	}

	out := make([]DailyUsage, 0, len(rowsByKey))
	for _, row := range rowsByKey {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Day.Equal(out[j].Day) {
			return out[i].Day.Before(out[j].Day)
		}
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// MergeDailyUsageRows merges incoming rows by unique day/provider/model, replacing
// existing entries for the same key. This keeps reruns idempotent.
func MergeDailyUsageRows(existing, incoming []DailyUsage) []DailyUsage {
	merged := make(map[dailyUsageKey]DailyUsage, len(existing)+len(incoming))
	for _, row := range existing {
		key := dailyUsageKey{day: row.Day.UTC().Format("2006-01-02"), provider: row.Provider, model: row.Model}
		merged[key] = row
	}
	for _, row := range incoming {
		key := dailyUsageKey{day: row.Day.UTC().Format("2006-01-02"), provider: row.Provider, model: row.Model}
		merged[key] = row
	}

	out := make([]DailyUsage, 0, len(merged))
	for _, row := range merged {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Day.Equal(out[j].Day) {
			return out[i].Day.Before(out[j].Day)
		}
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Model < out[j].Model
	})
	return out
}
