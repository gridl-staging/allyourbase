package fbmigrate

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// VerifyFirebaseScrypt verifies a password against Firebase's modified scrypt hash.
//
// Firebase uses a modified scrypt algorithm:
//  1. derivedKey = scrypt(password, salt + saltSeparator, N=2^memCost, r=rounds, p=1, keyLen=32)
//  2. AES-CTR encrypt the signerKey using derivedKey as the AES key
//  3. Compare the result with the stored passwordHash
//
// Firebase's default hash config uses rounds=8 (scrypt r parameter) and memCost=14 (N=16384).
func VerifyFirebaseScrypt(password string, salt, passwordHash, signerKey, saltSeparator []byte, rounds, memCost int) (bool, error) {
	// Compute N = 2^memCost (Firebase uses memCost as the log2 of the scrypt N parameter).
	n := 1 << memCost

	// Concatenate salt + saltSeparator.
	fullSalt := make([]byte, len(salt)+len(saltSeparator))
	copy(fullSalt, salt)
	copy(fullSalt[len(salt):], saltSeparator)

	// Derive key using scrypt. rounds is the scrypt r (block size) parameter.
	derivedKey, err := scrypt.Key([]byte(password), fullSalt, n, rounds, 1, 32)
	if err != nil {
		return false, fmt.Errorf("scrypt key derivation: %w", err)
	}

	// AES-CTR encrypt the signer key with the derived key.
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return false, fmt.Errorf("creating AES cipher: %w", err)
	}

	// Firebase uses a zero IV for AES-CTR.
	iv := make([]byte, aes.BlockSize)
	stream := cipher.NewCTR(block, iv)

	encryptedSignerKey := make([]byte, len(signerKey))
	stream.XORKeyStream(encryptedSignerKey, signerKey)

	// Compare with stored hash using constant-time comparison.
	return subtle.ConstantTimeCompare(encryptedSignerKey, passwordHash) == 1, nil
}

// ParseFirebaseScryptHash parses the AYB-stored firebase-scrypt format.
// Format: $firebase-scrypt$<b64-signerKey>$<b64-saltSeparator>$<b64-salt>$<rounds>$<memCost>$<b64-passwordHash>
//
// Returns the decoded components needed for verification.
func ParseFirebaseScryptHash(encoded string) (signerKey, saltSeparator, salt, passwordHash []byte, rounds, memCost int, err error) {
	if !strings.HasPrefix(encoded, "$firebase-scrypt$") {
		return nil, nil, nil, nil, 0, 0, fmt.Errorf("not a firebase-scrypt hash")
	}

	// Remove prefix and split.
	rest := strings.TrimPrefix(encoded, "$firebase-scrypt$")
	parts := strings.Split(rest, "$")
	if len(parts) != 6 {
		return nil, nil, nil, nil, 0, 0, fmt.Errorf("invalid firebase-scrypt hash: expected 6 parts, got %d", len(parts))
	}

	signerKey, err = base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, nil, nil, 0, 0, fmt.Errorf("decoding signer key: %w", err)
	}

	saltSeparator, err = base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, nil, nil, 0, 0, fmt.Errorf("decoding salt separator: %w", err)
	}

	salt, err = base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, nil, nil, 0, 0, fmt.Errorf("decoding salt: %w", err)
	}

	rounds, err = strconv.Atoi(parts[3])
	if err != nil {
		return nil, nil, nil, nil, 0, 0, fmt.Errorf("parsing rounds: %w", err)
	}

	memCost, err = strconv.Atoi(parts[4])
	if err != nil {
		return nil, nil, nil, nil, 0, 0, fmt.Errorf("parsing memCost: %w", err)
	}

	passwordHash, err = base64.StdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, nil, 0, 0, fmt.Errorf("decoding password hash: %w", err)
	}

	return signerKey, saltSeparator, salt, passwordHash, rounds, memCost, nil
}
