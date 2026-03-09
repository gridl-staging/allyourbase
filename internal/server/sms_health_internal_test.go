package server

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestConversionRate_ZeroSent(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, 0.0, conversionRate(0, 0))
	testutil.Equal(t, 0.0, conversionRate(0, 5))
}

func TestConversionRate_FullConversion(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, 100.0, conversionRate(10, 10))
}

func TestConversionRate_PartialConversion(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, 50.0, conversionRate(100, 50))
	testutil.Equal(t, 25.0, conversionRate(4, 1))
}

func TestDeliveryStatusRank_Ordering(t *testing.T) {
	t.Parallel()
	// Each step in the lifecycle must have a higher or equal rank than the previous.
	lifecycle := []string{"pending", "accepted", "queued", "sending", "sent", "delivered"}
	for i := 1; i < len(lifecycle); i++ {
		prev := deliveryStatusRank(lifecycle[i-1])
		curr := deliveryStatusRank(lifecycle[i])
		testutil.True(t, curr >= prev,
			"expected rank(%s)=%d >= rank(%s)=%d", lifecycle[i], curr, lifecycle[i-1], prev)
	}
}

func TestDeliveryStatusRank_TerminalStatuses(t *testing.T) {
	t.Parallel()
	// All terminal statuses must share the highest rank so they can overwrite each other.
	terminals := []string{"delivered", "undelivered", "failed", "read", "canceled"}
	rank := deliveryStatusRank(terminals[0])
	for _, s := range terminals[1:] {
		testutil.Equal(t, rank, deliveryStatusRank(s))
	}
}

func TestDeliveryStatusRank_UnknownStatusIsTerminal(t *testing.T) {
	t.Parallel()
	// Unknown statuses must rank as terminal (5) so they are never silently ignored.
	testutil.Equal(t, 5, deliveryStatusRank("some-future-twilio-status"))
	testutil.Equal(t, 5, deliveryStatusRank(""))
}
