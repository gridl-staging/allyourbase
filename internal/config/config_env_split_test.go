package config

import (
	"bytes"
	"os"
	"testing"
)

func TestConfigEnvGoUnder500Lines(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("config_env.go")
	if err != nil {
		t.Fatalf("read config_env.go: %v", err)
	}
	lineCount := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lineCount++
	}
	if lineCount >= 500 {
		t.Fatalf("config_env.go has %d lines; want fewer than 500", lineCount)
	}
}
