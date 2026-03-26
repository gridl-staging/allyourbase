package push

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSentinelErrorsDistinct(t *testing.T) {
	testutil.True(t, !errors.Is(ErrInvalidToken, ErrUnregistered))
	testutil.True(t, !errors.Is(ErrInvalidToken, ErrProviderError))
	testutil.True(t, !errors.Is(ErrInvalidToken, ErrPayloadTooLarge))
	testutil.True(t, !errors.Is(ErrInvalidToken, ErrProviderAuth))
}

func TestLogProviderSend(t *testing.T) {
	p := NewLogProvider(slog.Default())
	res, err := p.Send(context.Background(), "token-1", &Message{
		Title: "Title",
		Body:  "Body",
	})
	testutil.NoError(t, err)
	testutil.NotNil(t, res)
	testutil.True(t, res.MessageID != "")
}

func TestCaptureProviderSendAndReset(t *testing.T) {
	p := &CaptureProvider{}

	_, err := p.Send(context.Background(), "token-1", &Message{Title: "hello", Body: "world"})
	testutil.NoError(t, err)

	_, err = p.Send(context.Background(), "token-2", &Message{Title: "title", Body: "body"})
	testutil.NoError(t, err)

	testutil.SliceLen(t, p.Calls, 2)
	testutil.Equal(t, "token-1", p.Calls[0].Token)
	testutil.Equal(t, "hello", p.Calls[0].Message.Title)
	testutil.Equal(t, "token-2", p.Calls[1].Token)

	p.Reset()
	testutil.SliceLen(t, p.Calls, 0)
}

func TestProviderInterfaceConformance(t *testing.T) {
	var _ Provider = (*LogProvider)(nil)
	var _ Provider = (*CaptureProvider)(nil)
	var _ Provider = (*FCMProvider)(nil)
	var _ Provider = (*APNSProvider)(nil)
}
