package tenant

import (
	"bytes"
	"os"
	"testing"
)

func TestServiceGoUnder500Lines(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatalf("read service.go: %v", err)
	}
	lineCount := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lineCount++
	}
	if lineCount >= 500 {
		t.Fatalf("service.go has %d lines; want fewer than 500", lineCount)
	}
}
