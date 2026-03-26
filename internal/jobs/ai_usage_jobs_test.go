package jobs

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeAIUsageAggregator struct {
	calledDay time.Time
	calls     int
	err       error
}

func (f *fakeAIUsageAggregator) AggregateDailyUsage(_ context.Context, day time.Time) (int64, error) {
	f.calls++
	f.calledDay = day
	if f.err != nil {
		return 0, f.err
	}
	return 1, nil
}

func TestAIUsageAggregationJobHandler_DefaultsToYesterdayUTC(t *testing.T) {
	svc := NewService(&Store{}, slog.New(slog.NewTextHandler(io.Discard, nil)), DefaultServiceConfig())
	agg := &fakeAIUsageAggregator{}
	RegisterAIUsageAggregationHandler(svc, agg)

	h, ok := svc.handlers[AIUsageAggregationJobType]
	testutil.True(t, ok)
	testutil.NoError(t, h(context.Background(), []byte("{}")))
	testutil.Equal(t, 1, agg.calls)
	// We assert UTC midnight normalization.
	testutil.Equal(t, 0, agg.calledDay.Hour())
	testutil.Equal(t, time.UTC.String(), agg.calledDay.Location().String())
}
