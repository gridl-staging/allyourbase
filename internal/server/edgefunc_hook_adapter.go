package server

import (
	"context"

	"github.com/allyourbase/ayb/internal/edgefunc"
)

type edgeFuncHookAdapter struct {
	svc edgeFuncAdmin
}

func (a *edgeFuncHookAdapter) InvokeHook(ctx context.Context, name string, payload []byte) ([]byte, error) {
	resp, err := a.svc.Invoke(ctx, name, edgefunc.Request{
		Method: "POST",
		Path:   "/",
		Body:   payload,
	})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
