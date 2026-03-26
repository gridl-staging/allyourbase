package storage

import (
	"context"
	"errors"
	"net"
	"net/url"
	"reflect"
	"testing"

	sharedbackoff "github.com/allyourbase/ayb/internal/backoff"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSanitizePublicURLs(t *testing.T) {
	t.Parallel()

	got := sanitizePublicURLs([]string{"  https://cdn.example.com/a ", "", "https://cdn.example.com/a", "https://cdn.example.com/b", "\t", "https://cdn.example.com/b"})
	if !reflect.DeepEqual([]string{"https://cdn.example.com/a", "https://cdn.example.com/b"}, got) {
		t.Fatalf("expected %v, got %v", []string{"https://cdn.example.com/a", "https://cdn.example.com/b"}, got)
	}
}

func TestChunkStrings(t *testing.T) {
	t.Parallel()

	if !reflect.DeepEqual([][]string{{"1", "2", "3"}, {"4", "5", "6"}, {"7"}}, chunkStrings([]string{"1", "2", "3", "4", "5", "6", "7"}, 3)) {
		t.Fatalf("unexpected chunking output: %v", chunkStrings([]string{"1", "2", "3", "4", "5", "6", "7"}, 3))
	}
	if !reflect.DeepEqual([][]string{}, chunkStrings([]string{"1", "2"}, 0)) {
		t.Fatalf("expected no chunks, got: %v", chunkStrings([]string{"1", "2"}, 0))
	}
}

func TestResolveCDNRetrySettingsDefaultsZeroValues(t *testing.T) {
	t.Parallel()

	maxRetries, backoffConfig := resolveCDNRetrySettings(0, sharedbackoff.Config{})
	testutil.Equal(t, cdnDefaultMaxRetries, maxRetries)
	testutil.Equal(t, cdnDefaultBackoffConfig.Base, backoffConfig.Base)
	testutil.Equal(t, cdnDefaultBackoffConfig.Cap, backoffConfig.Cap)
}

func TestDoWithRetryRetriesOnlyWhenRetryable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	attempts := 0

	err := doWithRetry(ctx, 3, cdnDefaultBackoffConfig, func(err error) bool {
		return err == errTempRetry
	}, func(context.Context) error {
		attempts++
		if attempts == 1 {
			return errTempRetry
		}
		return nil
	})

	testutil.NoError(t, err)
	testutil.Equal(t, 2, attempts)
}

func TestDoWithRetryStopsOnNonRetryableError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	attempts := 0

	err := doWithRetry(ctx, 3, cdnDefaultBackoffConfig, func(err error) bool {
		return false
	}, func(context.Context) error {
		attempts++
		return errors.New("boom")
	})

	testutil.ErrorContains(t, err, "boom")
	testutil.Equal(t, 1, attempts)
}

func TestDoWithRetryStopsBeforeFirstAttemptWhenContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	attempts := 0
	err := doWithRetry(ctx, 3, cdnDefaultBackoffConfig, func(error) bool {
		return true
	}, func(context.Context) error {
		attempts++
		return errTempRetry
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	testutil.Equal(t, 0, attempts)
}

func TestIsRetryableTransportError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "context canceled", err: context.Canceled, want: false},
		{name: "context deadline exceeded", err: context.DeadlineExceeded, want: false},
		{name: "url wrapped context canceled", err: &url.Error{Err: context.Canceled}, want: false},
		{name: "url wrapped context deadline exceeded", err: &url.Error{Err: context.DeadlineExceeded}, want: true},
		{name: "timeout net error", err: timeoutNetError{}, want: true},
		{name: "dial op error", err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}, want: true},
		{name: "url wrapped dial op error", err: &url.Error{Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}, want: true},
		{name: "plain error", err: errors.New("boom"), want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, isRetryableTransportError(tt.err))
		})
	}
}

func TestNopCDNProviderIsNoop(t *testing.T) {
	t.Parallel()

	var p NopCDNProvider
	testutil.Equal(t, "nop", p.Name())
	testutil.NoError(t, p.PurgeURLs(context.Background(), []string{"https://cdn.example.com/file.jpg", ""}))
	testutil.NoError(t, p.PurgeAll(context.Background()))
}

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return false }

var errTempRetry = errors.New("temp retry")
