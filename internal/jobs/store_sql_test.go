package jobs

import (
	"os"
	"regexp"
	"testing"
)

func TestAdvanceScheduleAndEnqueueRequiresEnabledSchedule(t *testing.T) {
	src, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("read store.go: %v", err)
	}

	re := regexp.MustCompile(`WHERE id = \$1\s+AND enabled = true\s+AND next_run_at <= NOW\(\)`)
	if !re.Match(src) {
		t.Fatal("AdvanceScheduleAndEnqueue must gate enqueue on enabled = true")
	}
}
