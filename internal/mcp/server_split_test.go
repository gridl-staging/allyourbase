package mcp

import (
	"bytes"
	"os"
	"testing"
)

func TestServerGoUnder500Lines(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	lineCount := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lineCount++
	}
	if lineCount >= 500 {
		t.Fatalf("server.go has %d lines; want fewer than 500", lineCount)
	}
}
