package cli

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestDeployAPIErrorMessagePrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     DeployAPIError
		wantMsg string
	}{
		{
			name:    "message set",
			err:     DeployAPIError{Provider: "fly", StatusCode: 422, Message: "app already exists", Body: "raw body"},
			wantMsg: "fly API error (422): app already exists",
		},
		{
			name:    "falls back to body",
			err:     DeployAPIError{Provider: "digitalocean", StatusCode: 500, Body: "internal failure"},
			wantMsg: "digitalocean API error (500): internal failure",
		},
		{
			name:    "falls back to unknown",
			err:     DeployAPIError{Provider: "fly", StatusCode: 503},
			wantMsg: "unknown fly api error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.err.Error()
			if got != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

func TestIsDeployStatusCodeMatches(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("wrapped: %w", &DeployAPIError{Provider: "fly", StatusCode: 409})
	if !IsDeployStatusCode(err, 409) {
		t.Error("expected IsDeployStatusCode to match 409")
	}
	if IsDeployStatusCode(err, 500) {
		t.Error("expected IsDeployStatusCode NOT to match 500")
	}
}

func TestIsDeployStatusCodeNonMatchingError(t *testing.T) {
	t.Parallel()

	if IsDeployStatusCode(errors.New("other"), 409) {
		t.Error("expected false for non-DeployAPIError")
	}
	if IsDeployStatusCode(nil, 409) {
		t.Error("expected false for nil error")
	}
}

func TestBuildDeployAPIError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		provider    string
		statusCode  int
		body        []byte
		messageKeys []string
		wantMessage string
		wantBody    string
	}{
		{
			name:        "prefers first matching key",
			provider:    "fly",
			statusCode:  http.StatusUnprocessableEntity,
			body:        []byte(`{"error":"primary","message":"secondary"}`),
			messageKeys: []string{"error", "message"},
			wantMessage: "primary",
			wantBody:    `{"error":"primary","message":"secondary"}`,
		},
		{
			name:        "falls back to later key",
			provider:    "digitalocean",
			statusCode:  http.StatusUnprocessableEntity,
			body:        []byte(`{"error":"fallback","message":"preferred"}`),
			messageKeys: []string{"message", "error"},
			wantMessage: "preferred",
			wantBody:    `{"error":"fallback","message":"preferred"}`,
		},
		{
			name:        "falls back to trimmed raw body",
			provider:    "fly",
			statusCode:  http.StatusBadRequest,
			body:        []byte(" plain body "),
			messageKeys: []string{"error"},
			wantMessage: "plain body",
			wantBody:    "plain body",
		},
		{
			name:        "falls back to status text",
			provider:    "fly",
			statusCode:  http.StatusBadGateway,
			body:        []byte(""),
			messageKeys: []string{"error"},
			wantMessage: http.StatusText(http.StatusBadGateway),
			wantBody:    "",
		},
		{
			name:        "blank preferred field falls back to status text",
			provider:    "fly",
			statusCode:  http.StatusBadGateway,
			body:        []byte(`{"error":" "}`),
			messageKeys: []string{"error", "message"},
			wantMessage: http.StatusText(http.StatusBadGateway),
			wantBody:    `{"error":" "}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			apiErr := buildDeployAPIError(tt.provider, tt.statusCode, tt.body, tt.messageKeys...)
			if apiErr.Message != tt.wantMessage {
				t.Fatalf("Message = %q, want %q", apiErr.Message, tt.wantMessage)
			}
			if apiErr.Body != tt.wantBody {
				t.Fatalf("Body = %q, want %q", apiErr.Body, tt.wantBody)
			}
		})
	}
}
