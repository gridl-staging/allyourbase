package auth

import (
	"context"
	"strings"
)

type requestMetadata struct {
	userAgent string
	ipAddress string
}

type requestMetadataCtxKey struct{}

func contextWithRequestMetadata(ctx context.Context, userAgent, ipAddress string) context.Context {
	meta := requestMetadata{
		userAgent: strings.TrimSpace(userAgent),
		ipAddress: strings.TrimSpace(ipAddress),
	}
	return context.WithValue(ctx, requestMetadataCtxKey{}, meta)
}

func requestMetadataFromContext(ctx context.Context) (userAgent, ipAddress string) {
	meta, ok := ctx.Value(requestMetadataCtxKey{}).(requestMetadata)
	if !ok {
		return "", ""
	}
	return strings.TrimSpace(meta.userAgent), strings.TrimSpace(meta.ipAddress)
}

func nullableString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
