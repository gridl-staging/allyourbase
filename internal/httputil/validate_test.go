package httputil

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestValidateEmail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		email   string
		wantErr string
	}{
		{name: "empty", email: "", wantErr: "email is required"},
		{name: "missing at", email: "userexample.com", wantErr: "invalid email format"},
		{name: "dotless domain", email: "user@example", wantErr: "invalid email format"},
		{name: "display name form", email: "Name <a@b.c>", wantErr: "invalid email format"},
		{name: "leading dot domain", email: "user@.example.com", wantErr: "invalid email format"},
		{name: "trailing dot domain", email: "user@example.com.", wantErr: "invalid email format"},
		{name: "newline injection", email: "user@example.com\nBcc:evil@example.com", wantErr: "invalid email format"},
		{name: "valid", email: "user@example.com", wantErr: ""},
		{name: "valid subdomain", email: "user@mail.example.com", wantErr: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateEmail(tt.email)
			if tt.wantErr == "" {
				testutil.NoError(t, err)
				return
			}
			testutil.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateEmailLoose(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		email   string
		wantErr string
	}{
		{name: "empty", email: "", wantErr: "email is required"},
		{name: "missing at", email: "userexample.com", wantErr: "invalid email format"},
		{name: "dotless domain", email: "user@example", wantErr: "invalid email format"},
		{name: "display name accepted", email: "Name <a@b.c>", wantErr: ""},
		{name: "leading dot domain accepted", email: "user@.example.com", wantErr: ""},
		{name: "trailing dot domain accepted", email: "user@example.com.", wantErr: ""},
		{name: "newline injection", email: "user@example.com\nBcc:evil@example.com", wantErr: "invalid email format"},
		{name: "carriage return injection", email: "user@example.com\rBcc:evil@example.com", wantErr: "invalid email format"},
		{name: "valid", email: "user@example.com", wantErr: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateEmailLoose(tt.email)
			if tt.wantErr == "" {
				testutil.NoError(t, err)
				return
			}
			testutil.ErrorContains(t, err, tt.wantErr)
		})
	}
}
