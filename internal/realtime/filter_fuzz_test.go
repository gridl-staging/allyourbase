package realtime

import (
	"strings"
	"testing"
)

func FuzzParseFilters(f *testing.F) {
	seeds := []string{
		"",
		"status=eq.pending",
		"status=in.pending|active|review",
		"deleted_at=eq.null",
		"active=eq.true",
		"price=lt.50.5",
		"statuseq.pending",
		"status=eqpending",
		"=eq.pending",
		"status=eq.",
		"status=like.pending",
		"status=eq.pending,priority=gt.5",
		"status=in.pending|active,priority=gte.5",
		"status=eq.pending,",
		",status=eq.pending",
		"status=eq.pending,,priority=gt.5",
		"status=in.(pending|active),priority=eq.high",
		"status=in.(pending|active",
		"status=in.pending|active),priority=eq.high",
		"name=eq.名字",
		"name=eq.\x00",
		"\x00=\x01.\x02",
		"status=eq.pen,ding",
		"status=in.a|b|c|",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			t.Skip()
		}

		_, _ = ParseFilters(input)
		for _, part := range splitFilters(input) {
			_, _ = parseFilter(strings.TrimSpace(part))
		}
	})
}
