package vault

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	_, err := rand.Read(b)
	testutil.NoError(t, err)
	return b
}

func TestVaultEncryptDecryptRoundTripRandomData(t *testing.T) {
	masterKey := randomBytes(t, 32)
	plaintext := randomBytes(t, 4096)

	v, err := New(masterKey)
	testutil.NoError(t, err)

	ciphertext, nonce, err := v.Encrypt(plaintext)
	testutil.NoError(t, err)
	testutil.True(t, len(ciphertext) > 0, "ciphertext should not be empty")
	testutil.True(t, len(nonce) > 0, "nonce should not be empty")

	decrypted, err := v.Decrypt(ciphertext, nonce)
	testutil.NoError(t, err)
	testutil.True(t, bytes.Equal(plaintext, decrypted), "decrypted plaintext should match original")
}

func TestVaultDecryptRejectsWrongKey(t *testing.T) {
	v1, err := New(randomBytes(t, 32))
	testutil.NoError(t, err)
	v2, err := New(randomBytes(t, 32))
	testutil.NoError(t, err)

	ciphertext, nonce, err := v1.Encrypt([]byte("top-secret"))
	testutil.NoError(t, err)

	_, err = v2.Decrypt(ciphertext, nonce)
	testutil.NotNil(t, err)
}

func TestVaultEncryptNonceUniqueness(t *testing.T) {
	v, err := New(randomBytes(t, 32))
	testutil.NoError(t, err)

	seen := make(map[string]bool)
	for range 64 {
		_, nonce, err := v.Encrypt([]byte("same payload"))
		testutil.NoError(t, err)
		key := base64.StdEncoding.EncodeToString(nonce)
		testutil.False(t, seen[key], "duplicate nonce generated")
		seen[key] = true
	}
}

func TestVaultEncryptDecryptEmptyPlaintext(t *testing.T) {
	v, err := New(randomBytes(t, 32))
	testutil.NoError(t, err)

	ciphertext, nonce, err := v.Encrypt([]byte{})
	testutil.NoError(t, err)
	decrypted, err := v.Decrypt(ciphertext, nonce)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(decrypted))
}

func TestVaultEncryptDecryptLargeValue(t *testing.T) {
	v, err := New(randomBytes(t, 32))
	testutil.NoError(t, err)
	plaintext := randomBytes(t, 128*1024)

	ciphertext, nonce, err := v.Encrypt(plaintext)
	testutil.NoError(t, err)
	decrypted, err := v.Decrypt(ciphertext, nonce)
	testutil.NoError(t, err)
	testutil.True(t, bytes.Equal(plaintext, decrypted), "decrypted large plaintext should match original")
}

func TestResolveMasterKey_EnvTakesPrecedenceOverConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	envKey := randomBytes(t, 32)
	cfgKey := randomBytes(t, 32)
	t.Setenv("AYB_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(envKey))

	resolved, err := ResolveMasterKey(base64.StdEncoding.EncodeToString(cfgKey))
	testutil.NoError(t, err)
	testutil.True(t, bytes.Equal(envKey, resolved), "env key should take precedence over config")

	_, err = os.Stat(filepath.Join(homeDir, ".ayb", "vault-key"))
	testutil.True(t, os.IsNotExist(err), "key file should not be created when env key is provided")
}

func TestResolveMasterKey_UsesConfigWhenEnvMissing(t *testing.T) {
	t.Setenv("AYB_VAULT_MASTER_KEY", "")
	t.Setenv("HOME", t.TempDir())

	cfgKey := randomBytes(t, 32)
	resolved, err := ResolveMasterKey(base64.StdEncoding.EncodeToString(cfgKey))
	testutil.NoError(t, err)
	testutil.True(t, bytes.Equal(cfgKey, resolved), "config key should be used when env is empty")
}

func TestResolveMasterKey_UsesPersistedFileWhenEnvAndConfigMissing(t *testing.T) {
	t.Setenv("AYB_VAULT_MASTER_KEY", "")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	wantKey := randomBytes(t, 32)
	keyPath := filepath.Join(homeDir, ".ayb", "vault-key")
	testutil.NoError(t, os.MkdirAll(filepath.Dir(keyPath), 0o700))
	testutil.NoError(t, os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(wantKey)), 0o600))

	resolved, err := ResolveMasterKey("")
	testutil.NoError(t, err)
	testutil.True(t, bytes.Equal(wantKey, resolved), "persisted key should be used")
}

func TestDecodeMasterKeyRejectsTooShort(t *testing.T) {
	// Raw string shorter than 16 bytes that doesn't decode as hex or base64
	// must be rejected to prevent trivially breakable encryption.
	_, err := New([]byte("ab"))
	if err != nil {
		t.Fatalf("New should accept short key material (HKDF derives): %v", err)
	}

	// But ResolveMasterKey must reject a short config/env value:
	t.Setenv("AYB_VAULT_MASTER_KEY", "")
	t.Setenv("HOME", t.TempDir())
	_, err = ResolveMasterKey("short")
	if err == nil {
		t.Fatal("expected error for short master key, got nil")
	}
	if !strings.Contains(err.Error(), "too short") {
		t.Fatalf("expected 'too short' error, got: %v", err)
	}
}

func TestResolveMasterKey_GeneratesAndPersistsWhenUnset(t *testing.T) {
	t.Setenv("AYB_VAULT_MASTER_KEY", "")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	resolved1, err := ResolveMasterKey("")
	testutil.NoError(t, err)
	testutil.Equal(t, 32, len(resolved1))

	keyPath := filepath.Join(homeDir, ".ayb", "vault-key")
	info, err := os.Stat(keyPath)
	testutil.NoError(t, err)
	testutil.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	resolved2, err := ResolveMasterKey("")
	testutil.NoError(t, err)
	testutil.True(t, bytes.Equal(resolved1, resolved2), "generated key should be stable across loads")
}
