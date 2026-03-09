package server

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/webhooks"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeWebhookDispatcher struct {
	startPrunerCalls      int
	startPrunerInterval   time.Duration
	startPrunerRetention  time.Duration
	setDeliveryStoreCalls int
}

func (f *fakeWebhookDispatcher) Enqueue(_ *realtime.Event) {}

func (f *fakeWebhookDispatcher) SetDeliveryStore(_ webhooks.DeliveryStore) {
	f.setDeliveryStoreCalls++
}

func (f *fakeWebhookDispatcher) StartPruner(interval, retention time.Duration) {
	f.startPrunerCalls++
	f.startPrunerInterval = interval
	f.startPrunerRetention = retention
}

func (f *fakeWebhookDispatcher) Close() {}

func TestNewStartsLegacyWebhookPrunerWhenJobsDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Jobs.Enabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	schemaCache := schema.NewCacheHolder(nil, logger)
	fake := &fakeWebhookDispatcher{}

	origFactory := newWebhookDispatcher
	newWebhookDispatcher = func(_ webhooks.WebhookLister, _ *slog.Logger) webhookDispatcher {
		return fake
	}
	t.Cleanup(func() {
		newWebhookDispatcher = origFactory
	})

	_ = New(cfg, logger, schemaCache, &pgxpool.Pool{}, nil, nil)

	testutil.Equal(t, 1, fake.setDeliveryStoreCalls)
	testutil.Equal(t, 1, fake.startPrunerCalls)
	testutil.Equal(t, time.Hour, fake.startPrunerInterval)
	testutil.Equal(t, 7*24*time.Hour, fake.startPrunerRetention)
}

func TestNewSkipsLegacyWebhookPrunerWhenJobsEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.Jobs.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	schemaCache := schema.NewCacheHolder(nil, logger)
	fake := &fakeWebhookDispatcher{}

	origFactory := newWebhookDispatcher
	newWebhookDispatcher = func(_ webhooks.WebhookLister, _ *slog.Logger) webhookDispatcher {
		return fake
	}
	t.Cleanup(func() {
		newWebhookDispatcher = origFactory
	})

	_ = New(cfg, logger, schemaCache, &pgxpool.Pool{}, nil, nil)

	testutil.Equal(t, 1, fake.setDeliveryStoreCalls)
	testutil.Equal(t, 0, fake.startPrunerCalls)
}
