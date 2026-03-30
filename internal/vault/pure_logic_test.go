package vault

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// deriveKey
// ---------------------------------------------------------------------------

func TestDeriveKey(t *testing.T) {
	t.Parallel()

	masterKey := []byte("0123456789abcdef0123456789abcdef") // 32 bytes

	t.Run("produces 32-byte key", func(t *testing.T) {
		t.Parallel()
		salt := []byte("unique-salt-value-here-32-bytes!")
		key, err := deriveKey(masterKey, salt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(key) != derivedKeySize {
			t.Fatalf("key length = %d, want %d", len(key), derivedKeySize)
		}
	})

	t.Run("deterministic — same inputs produce same output", func(t *testing.T) {
		t.Parallel()
		salt := []byte("deterministic-test-salt-32bytes!")
		key1, err := deriveKey(masterKey, salt)
		if err != nil {
			t.Fatalf("first call: %v", err)
		}
		key2, err := deriveKey(masterKey, salt)
		if err != nil {
			t.Fatalf("second call: %v", err)
		}
		if string(key1) != string(key2) {
			t.Fatal("same inputs produced different keys — derivation is not deterministic")
		}
	})

	t.Run("different salts produce different keys", func(t *testing.T) {
		t.Parallel()
		salt1 := []byte("salt-one-padding-to-fill-length!")
		salt2 := []byte("salt-two-padding-to-fill-length!")
		key1, err := deriveKey(masterKey, salt1)
		if err != nil {
			t.Fatalf("salt1: %v", err)
		}
		key2, err := deriveKey(masterKey, salt2)
		if err != nil {
			t.Fatalf("salt2: %v", err)
		}
		if string(key1) == string(key2) {
			t.Fatal("different salts produced identical keys")
		}
	})

	t.Run("different master keys produce different keys", func(t *testing.T) {
		t.Parallel()
		salt := []byte("shared-salt-for-master-key-test!")
		mk1 := []byte("master-key-one-padding-32-bytes!")
		mk2 := []byte("master-key-two-padding-32-bytes!")
		key1, err := deriveKey(mk1, salt)
		if err != nil {
			t.Fatalf("mk1: %v", err)
		}
		key2, err := deriveKey(mk2, salt)
		if err != nil {
			t.Fatalf("mk2: %v", err)
		}
		if string(key1) == string(key2) {
			t.Fatal("different master keys produced identical derived keys")
		}
	})
}

// ---------------------------------------------------------------------------
// decodeMasterKey
// ---------------------------------------------------------------------------

func TestDecodeMasterKey(t *testing.T) {
	t.Parallel()

	// A 32-byte key for encoding tests.
	rawKey := []byte("0123456789abcdef0123456789abcdef")

	t.Run("hex-encoded key", func(t *testing.T) {
		t.Parallel()
		encoded := hex.EncodeToString(rawKey)
		got, err := decodeMasterKey(encoded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != string(rawKey) {
			t.Fatalf("decoded key differs from original")
		}
	})

	t.Run("base64 standard encoding", func(t *testing.T) {
		t.Parallel()
		encoded := base64.StdEncoding.EncodeToString(rawKey)
		got, err := decodeMasterKey(encoded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != string(rawKey) {
			t.Fatalf("decoded key differs from original")
		}
	})

	t.Run("base64 raw standard encoding (no padding)", func(t *testing.T) {
		t.Parallel()
		encoded := base64.RawStdEncoding.EncodeToString(rawKey)
		got, err := decodeMasterKey(encoded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != string(rawKey) {
			t.Fatalf("decoded key differs from original")
		}
	})

	t.Run("base64 URL-safe encoding", func(t *testing.T) {
		t.Parallel()
		encoded := base64.URLEncoding.EncodeToString(rawKey)
		got, err := decodeMasterKey(encoded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != string(rawKey) {
			t.Fatalf("decoded key differs from original")
		}
	})

	t.Run("raw string fallback for long-enough inputs", func(t *testing.T) {
		t.Parallel()
		// A string that is NOT valid hex or base64, but >= 16 bytes.
		raw := "this-is-not-hex-or-base64!!"
		got, err := decodeMasterKey(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != raw {
			t.Fatalf("expected raw bytes fallback, got %q", got)
		}
	})

	t.Run("empty string rejected", func(t *testing.T) {
		t.Parallel()
		_, err := decodeMasterKey("")
		if err == nil {
			t.Fatal("expected error for empty key")
		}
	})

	t.Run("whitespace-only rejected", func(t *testing.T) {
		t.Parallel()
		_, err := decodeMasterKey("   \t\n  ")
		if err == nil {
			t.Fatal("expected error for whitespace-only key")
		}
	})

	t.Run("too-short raw string rejected", func(t *testing.T) {
		t.Parallel()
		// 15 bytes — under the 16-byte minimum.
		_, err := decodeMasterKey("short-key-15chr")
		if err == nil {
			t.Fatal("expected error for short key")
		}
		if !strings.Contains(err.Error(), "too short") {
			t.Errorf("error should mention 'too short', got: %v", err)
		}
	})

	t.Run("leading/trailing whitespace trimmed", func(t *testing.T) {
		t.Parallel()
		encoded := "  " + hex.EncodeToString(rawKey) + "  "
		got, err := decodeMasterKey(encoded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != string(rawKey) {
			t.Fatalf("whitespace not trimmed before decoding")
		}
	})
}
