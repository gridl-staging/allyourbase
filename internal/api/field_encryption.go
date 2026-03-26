package api

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/config"
)

// Encrypter encrypts and decrypts plaintext bytes.
//
// Implementations in this repository include *vault.Vault.
type Encrypter interface {
	Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error)
	Decrypt(ciphertext, nonce []byte) ([]byte, error)
}

// FieldEncryptor handles API-layer encryption and decryption for configured columns.
type FieldEncryptor struct {
	enc      Encrypter
	colIndex map[string]map[string]struct{}
}

// NewFieldEncryptor creates a new helper from config entries.
func NewFieldEncryptor(enc Encrypter, cfgs []config.EncryptedColumnConfig) *FieldEncryptor {
	index := make(map[string]map[string]struct{})
	for _, cfg := range cfgs {
		table := strings.TrimSpace(strings.ToLower(cfg.Table))
		if table == "" {
			continue
		}
		set, ok := index[table]
		if !ok {
			set = make(map[string]struct{})
			index[table] = set
		}
		for _, col := range cfg.Columns {
			name := strings.TrimSpace(strings.ToLower(col))
			if name != "" {
				set[name] = struct{}{}
			}
		}
	}

	return &FieldEncryptor{enc: enc, colIndex: index}
}

// NewFieldEncryptorFromConfig creates a helper from config and returns nil when there
// is no configured encryption to apply.
func NewFieldEncryptorFromConfig(enc Encrypter, cfgs []config.EncryptedColumnConfig) *FieldEncryptor {
	if enc == nil || len(cfgs) == 0 {
		return nil
	}
	return NewFieldEncryptor(enc, cfgs)
}

// IsEncryptedColumn returns true if table+column is configured for encryption.
func (f *FieldEncryptor) IsEncryptedColumn(table, column string) bool {
	if f == nil {
		return false
	}
	columns := f.colIndex[normalizeName(table)]
	if len(columns) == 0 {
		return false
	}
	_, ok := columns[normalizeName(column)]
	return ok
}

// EncryptRecord encrypts configured string values in-place and replaces them with
// stored []byte payloads.
func (f *FieldEncryptor) EncryptRecord(table string, data map[string]any) error {
	if f == nil || f.enc == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}

	columns := f.colIndex[normalizeName(table)]
	if len(columns) == 0 {
		return nil
	}

	for key, value := range data {
		if _, ok := columns[normalizeName(key)]; !ok {
			continue
		}
		if value == nil {
			continue
		}

		plaintext, ok := value.(string)
		if !ok {
			// Already-encrypted bytes or preexisting non-string formats are left untouched.
			continue
		}

		ciphertext, nonce, err := f.enc.Encrypt([]byte(plaintext))
		if err != nil {
			return err
		}
		data[key] = encodeEncryptedBlob(nonce, ciphertext)
	}
	return nil
}

// DecryptRecord decrypts configured encrypted []byte values back to strings in-place.
func (f *FieldEncryptor) DecryptRecord(table string, data map[string]any) error {
	if f == nil || f.enc == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}

	columns := f.colIndex[normalizeName(table)]
	if len(columns) == 0 {
		return nil
	}

	for key, value := range data {
		if _, ok := columns[normalizeName(key)]; !ok {
			continue
		}
		if value == nil {
			continue
		}

		blob, ok := value.([]byte)
		if !ok {
			// Plaintext values that have already been returned as text stay as-is.
			continue
		}

		nonce, ciphertext, err := decodeEncryptedBlob(blob)
		if err != nil {
			return err
		}
		plaintext, err := f.enc.Decrypt(ciphertext, nonce)
		if err != nil {
			return err
		}
		data[key] = string(plaintext)
	}
	return nil
}

// ValidateFilter rejects encrypted columns in filter expressions.
func (f *FieldEncryptor) ValidateFilter(table, filter string) error {
	if f == nil {
		return nil
	}
	if strings.TrimSpace(filter) == "" {
		return nil
	}

	tokens, err := tokenize(filter)
	if err != nil {
		return err
	}
	for _, tok := range tokens {
		if tok.kind != tokIdent {
			continue
		}
		if f.IsEncryptedColumn(table, tok.value) {
			return fmt.Errorf("column %q is encrypted and not searchable", tok.value)
		}
	}
	return nil
}

// RotateKey re-encrypts encrypted columns for a row with newEnc.
func (f *FieldEncryptor) RotateKey(newEnc Encrypter, table string, data map[string]any) error {
	if f == nil || f.enc == nil {
		return nil
	}
	if newEnc == nil {
		return errors.New("new encrypter is required")
	}
	columns := f.colIndex[normalizeName(table)]
	if len(columns) == 0 {
		return nil
	}

	for key, value := range data {
		if _, ok := columns[normalizeName(key)]; !ok {
			continue
		}
		if value == nil {
			continue
		}
		blob, ok := value.([]byte)
		if !ok {
			continue
		}

		nonce, ciphertext, err := decodeEncryptedBlob(blob)
		if err != nil {
			return err
		}
		plaintext, err := f.enc.Decrypt(ciphertext, nonce)
		if err != nil {
			return err
		}
		newCiphertext, newNonce, err := newEnc.Encrypt(plaintext)
		if err != nil {
			return err
		}
		data[key] = encodeEncryptedBlob(newNonce, newCiphertext)
	}
	return nil
}

func encodeEncryptedBlob(nonce, ciphertext []byte) []byte {
	blob := make([]byte, 4+len(nonce)+len(ciphertext))
	// binary.BigEndian is deterministic and easy for variable-length nonce support.
	//nolint:gosec // fixed 4-byte length field is intentional and bounded by blob size checks.
	binary.BigEndian.PutUint32(blob[:4], uint32(len(nonce)))
	copy(blob[4:], nonce)
	copy(blob[4+len(nonce):], ciphertext)
	return blob
}

func decodeEncryptedBlob(blob []byte) ([]byte, []byte, error) {
	if len(blob) < 4 {
		return nil, nil, fmt.Errorf("invalid encrypted payload: too short")
	}
	nonceLen := int(binary.BigEndian.Uint32(blob[:4]))
	if nonceLen < 0 || len(blob) < 4+nonceLen {
		return nil, nil, fmt.Errorf("invalid encrypted payload: truncated nonce")
	}
	nonce := make([]byte, nonceLen)
	ciphertext := make([]byte, len(blob)-4-nonceLen)
	copy(nonce, blob[4:4+nonceLen])
	copy(ciphertext, blob[4+nonceLen:])
	return nonce, ciphertext, nil
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
