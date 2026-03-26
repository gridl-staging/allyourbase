// Package vault Vault encrypts and decrypts secrets using AES-256-GCM with per-secret HKDF-derived keys. Master keys are resolved from environment variables, configuration, or persisted in ~/.ayb/vault-key.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	masterKeyEnvName         = "AYB_VAULT_MASTER_KEY"
	keyFileRelativePath      = ".ayb/vault-key"
	derivedKeySize           = 32
	hkdfSaltSize             = 32
	hkdfInfo                 = "ayb-vault-aes256-gcm"
	minDecodedMasterKeyBytes = 16
)

// Vault encrypts and decrypts secrets using AES-256-GCM. A unique key is
// derived per secret via HKDF-SHA256 and a per-secret random salt.
type Vault struct {
	masterKey []byte
}

// New creates a vault from the provided master key material.
func New(masterKey []byte) (*Vault, error) {
	if len(masterKey) == 0 {
		return nil, errors.New("master key cannot be empty")
	}
	keyCopy := make([]byte, len(masterKey))
	copy(keyCopy, masterKey)
	return &Vault{masterKey: keyCopy}, nil
}

// Encrypt seals plaintext with AES-256-GCM. The returned nonce contains the
// HKDF salt prefix and the GCM nonce suffix.
func (v *Vault) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	salt := make([]byte, hkdfSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, fmt.Errorf("generating salt: %w", err)
	}

	key, err := deriveKey(v.masterKey, salt)
	if err != nil {
		return nil, nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("initializing cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("initializing gcm: %w", err)
	}

	gcmNonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(gcmNonce); err != nil {
		return nil, nil, fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext = gcm.Seal(nil, gcmNonce, plaintext, nil)
	nonce = make([]byte, len(salt)+len(gcmNonce))
	copy(nonce, salt)
	copy(nonce[len(salt):], gcmNonce)
	return ciphertext, nonce, nil
}

// Decrypt opens ciphertext produced by Encrypt.
func (v *Vault) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	if len(nonce) < hkdfSaltSize {
		return nil, fmt.Errorf("invalid nonce length: got %d, need at least %d", len(nonce), hkdfSaltSize)
	}

	salt := nonce[:hkdfSaltSize]
	gcmNonce := nonce[hkdfSaltSize:]

	key, err := deriveKey(v.masterKey, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initializing cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initializing gcm: %w", err)
	}
	if len(gcmNonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid gcm nonce length: got %d, want %d", len(gcmNonce), gcm.NonceSize())
	}

	plaintext, err := gcm.Open(nil, gcmNonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting secret: %w", err)
	}
	return plaintext, nil
}

func deriveKey(masterKey, salt []byte) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, masterKey, salt, hkdfInfo, derivedKeySize)
	if err != nil {
		return nil, fmt.Errorf("deriving key: %w", err)
	}
	return key, nil
}

// ResolveMasterKey returns the key material with precedence:
// AYB_VAULT_MASTER_KEY env var -> config master key -> ~/.ayb/vault-key.
// If all are unset, it generates and persists a new random 32-byte key.
func ResolveMasterKey(configMasterKey string) ([]byte, error) {
	if raw := strings.TrimSpace(os.Getenv(masterKeyEnvName)); raw != "" {
		return decodeMasterKey(raw)
	}
	if raw := strings.TrimSpace(configMasterKey); raw != "" {
		return decodeMasterKey(raw)
	}

	keyPath, err := defaultKeyPath()
	if err != nil {
		return nil, fmt.Errorf("resolving key path: %w", err)
	}

	if data, err := os.ReadFile(keyPath); err == nil {
		key, err := decodeMasterKey(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, fmt.Errorf("decoding persisted master key %s: %w", keyPath, err)
		}
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading persisted master key %s: %w", keyPath, err)
	}

	key := make([]byte, derivedKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating master key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, fmt.Errorf("creating key directory: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		return nil, fmt.Errorf("persisting generated master key: %w", err)
	}
	return key, nil
}

func defaultKeyPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, keyFileRelativePath), nil
}

// parses a master key string in hex, base64 (standard, raw standard, URL-safe, or raw URL-safe), or raw byte form. It attempts hex decoding first, then tries each base64 variant in sequence, and finally falls back to the raw string as bytes if at least 16 bytes long. Returns an error if the input is empty or cannot produce at least 16 bytes.
func decodeMasterKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("master key is empty")
	}

	decodedHex, err := hex.DecodeString(raw)
	if err == nil && len(decodedHex) >= minDecodedMasterKeyBytes {
		return decodedHex, nil
	}

	base64Decoders := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	}
	for _, decode := range base64Decoders {
		decoded, err := decode(raw)
		if err != nil {
			continue
		}
		if len(decoded) >= minDecodedMasterKeyBytes {
			return decoded, nil
		}
	}
	if len(raw) < minDecodedMasterKeyBytes {
		return nil, fmt.Errorf("master key too short: got %d bytes, need at least %d", len(raw), minDecodedMasterKeyBytes)
	}
	return []byte(raw), nil
}
