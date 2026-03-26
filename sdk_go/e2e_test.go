package allyourbase

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestE2EContract(t *testing.T) {
	baseURL := os.Getenv("AYB_TEST_URL")
	if baseURL == "" {
		t.Skip("AYB_TEST_URL not set")
	}

	c := NewClient(baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = c.Auth.Login(ctx, "does-not-matter@example.com", "bad")
}
