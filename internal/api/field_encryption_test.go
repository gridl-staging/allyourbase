package api

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

// --- fake vault for testing ---

// fakeEncrypter is a minimal Encrypter that XOR-encrypts with a single byte key.
// It uses a fixed nonce so results are deterministic in tests.
// NOT cryptographically secure — testing only.
type fakeEncrypter struct {
	key   byte
	errOn string // if non-empty, return an error when plaintext equals this
}

const fakeNonceLen = 4

func (f *fakeEncrypter) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	if f.errOn != "" && string(plaintext) == f.errOn {
		return nil, nil, errors.New("fake encrypt error")
	}
	ct := make([]byte, len(plaintext))
	for i, b := range plaintext {
		ct[i] = b ^ f.key
	}
	nonce = make([]byte, fakeNonceLen)
	return ct, nonce, nil
}

func (f *fakeEncrypter) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	if len(nonce) != fakeNonceLen {
		return nil, errors.New("fake decrypt: bad nonce")
	}
	pt := make([]byte, len(ciphertext))
	for i, b := range ciphertext {
		pt[i] = b ^ f.key
	}
	return pt, nil
}

// encodeBlob serialises nonce+ciphertext into the on-disk format used by FieldEncryptor.
func encodeBlob(nonce, ciphertext []byte) []byte {
	buf := make([]byte, 4+len(nonce)+len(ciphertext))
	binary.BigEndian.PutUint32(buf, uint32(len(nonce)))
	copy(buf[4:], nonce)
	copy(buf[4+len(nonce):], ciphertext)
	return buf
}

// --- helpers ---

func newTestEncryptor(enc Encrypter, cfgs ...config.EncryptedColumnConfig) *FieldEncryptor {
	return NewFieldEncryptor(enc, cfgs)
}

// --- IsEncryptedColumn ---

func TestIsEncryptedColumnTrue(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0xAB},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn", "email"}},
	)
	testutil.True(t, fe.IsEncryptedColumn("users", "ssn"))
	testutil.True(t, fe.IsEncryptedColumn("users", "email"))
}

func TestIsEncryptedColumnFalse(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0xAB},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	testutil.False(t, fe.IsEncryptedColumn("users", "name"))
	testutil.False(t, fe.IsEncryptedColumn("orders", "ssn")) // different table
}

func TestIsEncryptedColumnEmptyConfig(t *testing.T) {
	t.Parallel()
	fe := NewFieldEncryptor(&fakeEncrypter{key: 0xAB}, nil)
	testutil.False(t, fe.IsEncryptedColumn("users", "ssn"))
}

// --- EncryptRecord ---

func TestEncryptRecordEncryptsConfiguredColumns(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	data := map[string]any{
		"id":  "123",
		"ssn": "999-00-1234",
	}
	err := fe.EncryptRecord("users", data)
	testutil.NoError(t, err)

	// id should be unchanged
	testutil.Equal(t, "123", data["id"])

	// ssn should now be []byte (not the original string)
	blob, ok := data["ssn"].([]byte)
	testutil.True(t, ok, "ssn should be []byte after encryption")
	testutil.True(t, len(blob) > 0, "encrypted blob should be non-empty")
}

func TestEncryptRecordSkipsNonConfiguredColumns(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	data := map[string]any{
		"name": "Alice",
		"age":  30,
	}
	err := fe.EncryptRecord("users", data)
	testutil.NoError(t, err)
	testutil.Equal(t, "Alice", data["name"])
	testutil.Equal(t, 30, data["age"])
}

func TestEncryptRecordSkipsNilValues(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	data := map[string]any{"ssn": nil}
	err := fe.EncryptRecord("users", data)
	testutil.NoError(t, err)
	testutil.Equal(t, nil, data["ssn"])
}

func TestEncryptRecordUnknownTable(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	data := map[string]any{"ssn": "secret"}
	err := fe.EncryptRecord("orders", data)
	testutil.NoError(t, err)
	// ssn unchanged because "orders" has no encrypted columns
	testutil.Equal(t, "secret", data["ssn"])
}

// --- DecryptRecord ---

func TestDecryptRecordDecryptsBytea(t *testing.T) {
	t.Parallel()
	enc := &fakeEncrypter{key: 0x42}
	fe := newTestEncryptor(enc,
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)

	// Pre-encrypt a value manually to simulate what's stored in DB.
	ct, nonce, err := enc.Encrypt([]byte("999-00-1234"))
	testutil.NoError(t, err)
	blob := encodeBlob(nonce, ct)

	data := map[string]any{
		"id":  "abc",
		"ssn": blob,
	}
	err = fe.DecryptRecord("users", data)
	testutil.NoError(t, err)

	testutil.Equal(t, "999-00-1234", data["ssn"])
	testutil.Equal(t, "abc", data["id"])
}

func TestDecryptRecordSkipsNonBytea(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	// If the column value is already a string (e.g. not yet migrated), leave it alone.
	data := map[string]any{"ssn": "plaintext-not-bytes"}
	err := fe.DecryptRecord("users", data)
	testutil.NoError(t, err)
	testutil.Equal(t, "plaintext-not-bytes", data["ssn"])
}

func TestDecryptRecordSkipsNilValues(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	data := map[string]any{"ssn": nil}
	err := fe.DecryptRecord("users", data)
	testutil.NoError(t, err)
	testutil.Equal(t, nil, data["ssn"])
}

// --- Round-trip ---

func TestRoundTripEncryptDecrypt(t *testing.T) {
	t.Parallel()
	enc := &fakeEncrypter{key: 0xAB}
	fe := newTestEncryptor(enc,
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn", "dob"}},
	)

	original := map[string]any{
		"id":  "u-1",
		"ssn": "123-45-6789",
		"dob": "1990-01-15",
	}

	// Encrypt modifies data in-place; make a copy for comparison.
	data := map[string]any{
		"id":  original["id"],
		"ssn": original["ssn"],
		"dob": original["dob"],
	}

	err := fe.EncryptRecord("users", data)
	testutil.NoError(t, err)
	// Verify encrypted values are not the originals.
	testutil.NotEqual(t, original["ssn"], data["ssn"])
	testutil.NotEqual(t, original["dob"], data["dob"])

	err = fe.DecryptRecord("users", data)
	testutil.NoError(t, err)
	// After decryption, values should match originals.
	testutil.Equal(t, original["ssn"], data["ssn"])
	testutil.Equal(t, original["dob"], data["dob"])
	testutil.Equal(t, original["id"], data["id"])
}

// --- Filter validation ---

func TestValidateFilterEncryptedColumnReturnsError(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	err := fe.ValidateFilter("users", "ssn='123-45-6789'")
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "ssn")
}

func TestValidateFilterNonEncryptedColumnNoError(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	err := fe.ValidateFilter("users", "name='Alice'")
	testutil.Nil(t, err)
}

func TestValidateFilterEmptyFilterNoError(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	err := fe.ValidateFilter("users", "")
	testutil.Nil(t, err)
}

func TestValidateFilterUnknownTableNoError(t *testing.T) {
	t.Parallel()
	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)
	// The "orders" table has no encrypted columns — any filter is fine.
	err := fe.ValidateFilter("orders", "ssn='secret'")
	testutil.Nil(t, err)
}

// --- HTTP handler: filter on encrypted column → 400 ---

func TestHandlerListFilterOnEncryptedColumn400(t *testing.T) {
	t.Parallel()

	sc := &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {
				Schema: "public",
				Name:   "users",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "uuid"},
					{Name: "ssn", TypeName: "bytea"},
					{Name: "name", TypeName: "text"},
				},
				PrimaryKey: []string{"id"},
			},
		},
		Schemas: []string{"public"},
	}

	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)

	ch := testCacheHolder(sc)
	h := NewHandler(nil, ch, nil, nil, nil, nil, fe)
	handler := h.Routes()

	w := doRequest(handler, "GET", "/collections/users?filter=ssn%3D'123-45-6789'", "")
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	msg, _ := resp["message"].(string)
	testutil.Contains(t, strings.ToLower(msg), "encrypted")
}

func TestHandlerListFilterOnNonEncryptedColumnPasses(t *testing.T) {
	t.Parallel()

	sc := &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {
				Schema: "public",
				Name:   "users",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "uuid"},
					{Name: "ssn", TypeName: "bytea"},
					{Name: "name", TypeName: "text"},
				},
				PrimaryKey: []string{"id"},
			},
		},
		Schemas: []string{"public"},
	}

	fe := newTestEncryptor(&fakeEncrypter{key: 0x42},
		config.EncryptedColumnConfig{Table: "users", Columns: []string{"ssn"}},
	)

	ch := testCacheHolder(sc)
	h := NewHandler(nil, ch, nil, nil, nil, nil, fe)
	handler := h.Routes()

	// Filtering on "name" (not encrypted) should not get a 400 from filter validation.
	// It will fail later (500) because we have no DB — that's fine for this test.
	w := doRequest(handler, "GET", "/collections/users?filter=name%3D'Alice'", "")
	testutil.NotEqual(t, http.StatusBadRequest, w.Code)
}

// --- Key rotation ---

func TestKeyRotation(t *testing.T) {
	t.Parallel()
	oldEnc := &fakeEncrypter{key: 0x11}
	newEnc := &fakeEncrypter{key: 0xFF}
	fe := newTestEncryptor(oldEnc,
		config.EncryptedColumnConfig{Table: "secrets", Columns: []string{"token"}},
	)

	// Encrypt with old key.
	data := map[string]any{"token": "my-secret-token"}
	err := fe.EncryptRecord("secrets", data)
	testutil.NoError(t, err)
	oldBlob, ok := data["token"].([]byte)
	testutil.True(t, ok, "token should be []byte after encryption")

	// Re-encrypt with new key via RotateKey helper.
	err = fe.RotateKey(newEnc, "secrets", data)
	testutil.NoError(t, err)
	newBlob, ok := data["token"].([]byte)
	testutil.True(t, ok, "token should be []byte after rotation")

	// Old and new blobs should differ (different key).
	testutil.NotEqual(t, string(oldBlob), string(newBlob))

	// Decrypt with new key encryptor should give back original.
	feNew := newTestEncryptor(newEnc,
		config.EncryptedColumnConfig{Table: "secrets", Columns: []string{"token"}},
	)
	err = feNew.DecryptRecord("secrets", data)
	testutil.NoError(t, err)
	testutil.Equal(t, "my-secret-token", data["token"])
}
