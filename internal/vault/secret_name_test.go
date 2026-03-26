package vault

import (
	"errors"
	"testing"
)

func TestNormalizeSecretNameValid(t *testing.T) {
	t.Parallel()

	name, err := NormalizeSecretName(" API_KEY-1.v2 ")
	if err != nil {
		t.Fatalf("NormalizeSecretName returned error: %v", err)
	}
	if name != "API_KEY-1.v2" {
		t.Fatalf("got %q, want %q", name, "API_KEY-1.v2")
	}
}

func TestNormalizeSecretNameInvalid(t *testing.T) {
	t.Parallel()

	invalidNames := []string{"", "a/b", "..", "bad name", "line\nbreak"}
	for _, input := range invalidNames {
		t.Run(input, func(t *testing.T) {
			_, err := NormalizeSecretName(input)
			if !errors.Is(err, ErrInvalidSecretName) {
				t.Fatalf("expected ErrInvalidSecretName for %q, got %v", input, err)
			}
		})
	}
}
