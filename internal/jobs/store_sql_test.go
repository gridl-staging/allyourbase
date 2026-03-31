package jobs

import (
	"os"
	"regexp"
	"testing"
)

func TestAdvanceScheduleAndEnqueueRequiresEnabledSchedule(t *testing.T) {
	paths := []string{"store.go", "store_schedule.go"}
	src := make([]byte, 0, 4096)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		src = append(src, data...)
		src = append(src, '\n')
	}

	re := regexp.MustCompile(`WHERE id = \$1\s+AND enabled = true\s+AND next_run_at <= NOW\(\)`)
	if !re.Match(src) {
		t.Fatal("AdvanceScheduleAndEnqueue must gate enqueue on enabled = true")
	}
}
