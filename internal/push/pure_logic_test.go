package push

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// ---------------------------------------------------------------------------
// isValidProvider
// ---------------------------------------------------------------------------

func TestIsValidProvider(t *testing.T) {
	t.Parallel()

	// Valid providers — the two supported push services.
	if !isValidProvider(ProviderFCM) {
		t.Error("isValidProvider(ProviderFCM) = false, want true")
	}
	if !isValidProvider(ProviderAPNS) {
		t.Error("isValidProvider(ProviderAPNS) = false, want true")
	}

	// Invalid providers — unknown names, empty, case variants.
	invalid := []string{"", "FCM", "apns ", "webpush", "sns", "APNS"}
	for _, p := range invalid {
		if isValidProvider(p) {
			t.Errorf("isValidProvider(%q) = true, want false", p)
		}
	}
}

// ---------------------------------------------------------------------------
// isValidPlatform
// ---------------------------------------------------------------------------

func TestIsValidPlatform(t *testing.T) {
	t.Parallel()

	if !isValidPlatform(PlatformAndroid) {
		t.Error("isValidPlatform(PlatformAndroid) = false, want true")
	}
	if !isValidPlatform(PlatformIOS) {
		t.Error("isValidPlatform(PlatformIOS) = false, want true")
	}

	invalid := []string{"", "Android", "iOS", "web", "windows", "IOS"}
	for _, p := range invalid {
		if isValidPlatform(p) {
			t.Errorf("isValidPlatform(%q) = true, want false", p)
		}
	}
}

// ---------------------------------------------------------------------------
// cloneStringMap
// ---------------------------------------------------------------------------

func TestCloneStringMap(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns nil", func(t *testing.T) {
		t.Parallel()
		got := cloneStringMap(nil)
		if got != nil {
			t.Fatalf("cloneStringMap(nil) = %v, want nil", got)
		}
	})

	t.Run("empty map returns nil", func(t *testing.T) {
		t.Parallel()
		// The function returns nil for len==0, not an empty map.
		got := cloneStringMap(map[string]string{})
		if got != nil {
			t.Fatalf("cloneStringMap(empty) = %v, want nil", got)
		}
	})

	t.Run("clones all entries", func(t *testing.T) {
		t.Parallel()
		orig := map[string]string{"a": "1", "b": "2"}
		cloned := cloneStringMap(orig)
		if len(cloned) != 2 || cloned["a"] != "1" || cloned["b"] != "2" {
			t.Fatalf("cloneStringMap = %v, want map[a:1 b:2]", cloned)
		}
	})

	t.Run("mutation isolation", func(t *testing.T) {
		t.Parallel()
		// Mutating the clone must not affect the original.
		orig := map[string]string{"key": "val"}
		cloned := cloneStringMap(orig)
		cloned["key"] = "changed"
		if orig["key"] != "val" {
			t.Fatal("mutating clone affected original map")
		}
	})
}

// ---------------------------------------------------------------------------
// classifyProviderError
// ---------------------------------------------------------------------------

func TestClassifyProviderError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		// Direct sentinel matches.
		{"unregistered", ErrUnregistered, "unregistered"},
		{"invalid_token", ErrInvalidToken, "invalid_token"},
		{"provider_auth", ErrProviderAuth, "provider_auth"},
		{"payload_too_large", ErrPayloadTooLarge, "payload_too_large"},
		// Wrapped sentinel — errors.Is sees through fmt.Errorf wrapping.
		{"wrapped unregistered", fmt.Errorf("details: %w", ErrUnregistered), "unregistered"},
		{"wrapped invalid_token", fmt.Errorf("details: %w", ErrInvalidToken), "invalid_token"},
		// Generic provider error and unknown errors.
		{"provider_error sentinel", ErrProviderError, "provider_error"},
		{"arbitrary error", errors.New("something random"), "provider_error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyProviderError(tc.err)
			if got != tc.want {
				t.Errorf("classifyProviderError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// isPayloadTooLarge
// ---------------------------------------------------------------------------

func TestIsPayloadTooLarge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		message string
		want    bool
	}{
		// Positive matches — case insensitive substring checks.
		{"Message too big for FCM", true},
		{"Payload too large", true},
		{"invalid payload size exceeded", true},
		{"MESSAGE TOO BIG", true},   // uppercase
		{"Payload Too Large", true}, // mixed case

		// Negative matches — partial/unrelated strings.
		{"", false},
		{"invalid argument", false},
		{"generic error", false},
		{"too many requests", false},
	}
	for _, tc := range tests {
		t.Run(tc.message, func(t *testing.T) {
			t.Parallel()
			got := isPayloadTooLarge(tc.message)
			if got != tc.want {
				t.Errorf("isPayloadTooLarge(%q) = %v, want %v", tc.message, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// mapFCMError
// ---------------------------------------------------------------------------

func TestMapFCMError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		code       string
		message    string
		statusCode int
		wantErr    error
	}{
		// FCM-specific error codes.
		{"UNREGISTERED", "UNREGISTERED", "", 200, ErrUnregistered},
		{"SENDER_ID_MISMATCH", "SENDER_ID_MISMATCH", "", 200, ErrInvalidToken},
		{"INVALID_ARGUMENT generic", "INVALID_ARGUMENT", "bad field", 200, ErrInvalidToken},
		{"INVALID_ARGUMENT payload too big", "INVALID_ARGUMENT", "message too big", 200, ErrPayloadTooLarge},
		{"THIRD_PARTY_AUTH_ERROR", "THIRD_PARTY_AUTH_ERROR", "", 200, ErrProviderAuth},
		{"UNAUTHENTICATED", "UNAUTHENTICATED", "", 200, ErrProviderAuth},
		{"QUOTA_EXCEEDED", "QUOTA_EXCEEDED", "", 200, ErrProviderError},
		{"UNAVAILABLE", "UNAVAILABLE", "", 200, ErrProviderError},
		{"INTERNAL", "INTERNAL", "", 200, ErrProviderError},

		// HTTP status code fallbacks (when code doesn't match known FCM codes).
		{"HTTP 401", "HTTP_401", "", http.StatusUnauthorized, ErrProviderAuth},
		{"HTTP 403", "HTTP_403", "", http.StatusForbidden, ErrProviderAuth},
		{"HTTP 413", "HTTP_413", "", http.StatusRequestEntityTooLarge, ErrPayloadTooLarge},
		{"HTTP 500 default", "HTTP_500", "", http.StatusInternalServerError, ErrProviderError},
		{"unknown code", "UNKNOWN_CODE", "", 200, ErrProviderError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mapFCMError(tc.code, tc.message, tc.statusCode)
			if !errors.Is(got, tc.wantErr) {
				t.Errorf("mapFCMError(%q, %q, %d) = %v, want %v",
					tc.code, tc.message, tc.statusCode, got, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseFCMError
// ---------------------------------------------------------------------------

func TestParseFCMError(t *testing.T) {
	t.Parallel()

	t.Run("well-formed FCM error response", func(t *testing.T) {
		t.Parallel()
		// Standard FCM v1 error JSON with details array.
		body := `{
			"error": {
				"code": 400,
				"message": "Invalid registration",
				"status": "INVALID_ARGUMENT",
				"details": [{"errorCode": "UNREGISTERED"}]
			}
		}`
		code, message, sentinel := parseFCMError(400, []byte(body))
		// errorCode from details takes precedence over status.
		if code != "UNREGISTERED" {
			t.Errorf("code = %q, want UNREGISTERED", code)
		}
		if message != "Invalid registration" {
			t.Errorf("message = %q, want 'Invalid registration'", message)
		}
		if !errors.Is(sentinel, ErrUnregistered) {
			t.Errorf("sentinel = %v, want ErrUnregistered", sentinel)
		}
	})

	t.Run("FCM error without details uses status field", func(t *testing.T) {
		t.Parallel()
		body := `{
			"error": {
				"code": 401,
				"message": "auth failed",
				"status": "UNAUTHENTICATED"
			}
		}`
		code, _, sentinel := parseFCMError(401, []byte(body))
		// No details → falls back to status field.
		if code != "UNAUTHENTICATED" {
			t.Errorf("code = %q, want UNAUTHENTICATED", code)
		}
		if !errors.Is(sentinel, ErrProviderAuth) {
			t.Errorf("sentinel = %v, want ErrProviderAuth", sentinel)
		}
	})

	t.Run("malformed JSON uses HTTP status fallback", func(t *testing.T) {
		t.Parallel()
		code, message, sentinel := parseFCMError(500, []byte("not json"))
		// Fallback: code = "HTTP_500", message = raw body.
		if code != "HTTP_500" {
			t.Errorf("code = %q, want HTTP_500", code)
		}
		if message != "not json" {
			t.Errorf("message = %q, want 'not json'", message)
		}
		if !errors.Is(sentinel, ErrProviderError) {
			t.Errorf("sentinel = %v, want ErrProviderError", sentinel)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		t.Parallel()
		code, message, _ := parseFCMError(403, []byte(""))
		if code != "HTTP_403" {
			t.Errorf("code = %q, want HTTP_403", code)
		}
		if message != "" {
			t.Errorf("message = %q, want empty", message)
		}
	})
}

// ---------------------------------------------------------------------------
// Provider and platform constants — guard against accidental renames
// ---------------------------------------------------------------------------

func TestProviderConstants(t *testing.T) {
	t.Parallel()

	// These values are stored in DB — renaming breaks existing data.
	if ProviderFCM != "fcm" {
		t.Errorf("ProviderFCM = %q, want fcm", ProviderFCM)
	}
	if ProviderAPNS != "apns" {
		t.Errorf("ProviderAPNS = %q, want apns", ProviderAPNS)
	}
}

func TestPlatformConstants(t *testing.T) {
	t.Parallel()

	if PlatformAndroid != "android" {
		t.Errorf("PlatformAndroid = %q, want android", PlatformAndroid)
	}
	if PlatformIOS != "ios" {
		t.Errorf("PlatformIOS = %q, want ios", PlatformIOS)
	}
}

// ---------------------------------------------------------------------------
// Sentinel error distinctness — prevent copy-paste mix-ups
// ---------------------------------------------------------------------------

func TestSentinelErrorDistinctness(t *testing.T) {
	t.Parallel()

	sentinels := []error{
		ErrInvalidToken,
		ErrUnregistered,
		ErrProviderError,
		ErrPayloadTooLarge,
		ErrProviderAuth,
		ErrNotFound,
		ErrInvalidProvider,
		ErrInvalidPlatform,
		ErrInvalidPayload,
	}
	seen := make(map[string]bool, len(sentinels))
	for _, err := range sentinels {
		msg := err.Error()
		if seen[msg] {
			t.Errorf("duplicate sentinel error message: %q", msg)
		}
		seen[msg] = true
	}
}

// suppress unused import warning — json is used in test assertions.
var _ = json.Unmarshal
