package fbmigrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// makeAuthExportFile creates a temporary auth export JSON file for testing.
func makeAuthExportFile(t *testing.T, export FirebaseAuthExport) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth-export.json")
	data, err := json.Marshal(export)
	testutil.NoError(t, err)
	err = os.WriteFile(path, data, 0644)
	testutil.NoError(t, err)
	return path
}

func TestParseAuthExport(t *testing.T) {
	t.Parallel()
	t.Run("valid export with 5 user types", func(t *testing.T) {
		t.Parallel()
		export := FirebaseAuthExport{
			Users: []FirebaseUser{
				{LocalID: "u1", Email: "user1@test.com", PasswordHash: "aGFzaA==", Salt: "c2FsdA==", EmailVerified: true, CreatedAt: "1700000000000"},
				{LocalID: "u2", Email: "oauth@test.com", ProviderInfo: []ProviderInfo{{ProviderID: "google.com", RawID: "g123", Email: "oauth@test.com"}}},
				{LocalID: "u3", Email: "both@test.com", PasswordHash: "aGFzaA==", Salt: "c2FsdA==", ProviderInfo: []ProviderInfo{{ProviderID: "github.com", RawID: "gh456"}}},
				{LocalID: "u4", ProviderInfo: []ProviderInfo{{ProviderID: "phone", RawID: "+1234567890"}}},
				{LocalID: "u5"},
			},
			HashConfig: FirebaseHashConfig{
				Algorithm:           "SCRYPT",
				Base64SignerKey:     "c2lnbmVy",
				Base64SaltSeparator: "c2Vw",
				Rounds:              8,
				MemCost:             14,
			},
		}

		path := makeAuthExportFile(t, export)
		users, config, err := ParseAuthExport(path)
		testutil.NoError(t, err)
		testutil.Equal(t, 5, len(users))
		testutil.Equal(t, "SCRYPT", config.Algorithm)
		testutil.Equal(t, 8, config.Rounds)
		testutil.Equal(t, 14, config.MemCost)
	})

	t.Run("file not found", func(t *testing.T) {
		t.Parallel()
		_, _, err := ParseAuthExport("/nonexistent/path.json")
		testutil.ErrorContains(t, err, "reading auth export")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		err := os.WriteFile(path, []byte("not json"), 0644)
		testutil.NoError(t, err)
		_, _, err = ParseAuthExport(path)
		testutil.ErrorContains(t, err, "parsing auth export JSON")
	})

	t.Run("empty users array", func(t *testing.T) {
		t.Parallel()
		export := FirebaseAuthExport{
			Users:      []FirebaseUser{},
			HashConfig: FirebaseHashConfig{Algorithm: "SCRYPT"},
		}
		path := makeAuthExportFile(t, export)
		users, config, err := ParseAuthExport(path)
		testutil.NoError(t, err)
		testutil.Equal(t, 0, len(users))
		testutil.Equal(t, "SCRYPT", config.Algorithm)
	})
}

func TestIsEmailUser(t *testing.T) {
	t.Parallel()
	testutil.True(t, IsEmailUser(FirebaseUser{Email: "a@b.com"}), "should be email user")
	testutil.True(t, !IsEmailUser(FirebaseUser{}), "should not be email user")
}

func TestIsPasswordUser(t *testing.T) {
	t.Parallel()
	testutil.True(t, IsPasswordUser(FirebaseUser{PasswordHash: "hash"}), "should be password user")
	testutil.True(t, !IsPasswordUser(FirebaseUser{}), "should not be password user")
}

func TestIsAnonymousUser(t *testing.T) {
	t.Parallel()
	testutil.True(t, IsAnonymousUser(FirebaseUser{LocalID: "u1"}), "no email, no providers, no password = anonymous")
	testutil.True(t, !IsAnonymousUser(FirebaseUser{Email: "a@b.com"}), "has email = not anonymous")
	testutil.True(t, !IsAnonymousUser(FirebaseUser{PasswordHash: "hash"}), "has password = not anonymous")
	testutil.True(t, !IsAnonymousUser(FirebaseUser{ProviderInfo: []ProviderInfo{{ProviderID: "google.com"}}}), "has providers = not anonymous")
}

func TestIsPhoneOnlyUser(t *testing.T) {
	t.Parallel()
	testutil.True(t, IsPhoneOnlyUser(FirebaseUser{
		ProviderInfo: []ProviderInfo{{ProviderID: "phone", RawID: "+1234"}},
	}), "phone-only user")
	testutil.True(t, !IsPhoneOnlyUser(FirebaseUser{
		Email:        "a@b.com",
		ProviderInfo: []ProviderInfo{{ProviderID: "phone"}},
	}), "has email = not phone-only")
	testutil.True(t, !IsPhoneOnlyUser(FirebaseUser{}), "no providers at all")
	testutil.True(t, !IsPhoneOnlyUser(FirebaseUser{
		ProviderInfo: []ProviderInfo{{ProviderID: "google.com"}},
	}), "google provider = not phone-only")
}

func TestOAuthProviders(t *testing.T) {
	t.Parallel()
	t.Run("filters out password and phone", func(t *testing.T) {
		t.Parallel()
		u := FirebaseUser{
			ProviderInfo: []ProviderInfo{
				{ProviderID: "password"},
				{ProviderID: "google.com", RawID: "g1"},
				{ProviderID: "phone", RawID: "+1"},
				{ProviderID: "github.com", RawID: "gh1"},
			},
		}
		providers := OAuthProviders(u)
		testutil.Equal(t, 2, len(providers))
		testutil.Equal(t, "google.com", providers[0].ProviderID)
		testutil.Equal(t, "github.com", providers[1].ProviderID)
	})

	t.Run("empty providers", func(t *testing.T) {
		t.Parallel()
		providers := OAuthProviders(FirebaseUser{})
		testutil.Equal(t, 0, len(providers))
	})

	t.Run("only password provider", func(t *testing.T) {
		t.Parallel()
		u := FirebaseUser{
			ProviderInfo: []ProviderInfo{{ProviderID: "password"}},
		}
		providers := OAuthProviders(u)
		testutil.Equal(t, 0, len(providers))
	})
}

func TestNormalizeProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"google.com", "google"},
		{"github.com", "github"},
		{"facebook.com", "facebook"},
		{"twitter.com", "twitter"},
		{"apple.com", "apple"},
		{"microsoft.com", "microsoft"},
		{"custom.com", "custom"},
		{"saml.provider", "saml.provider"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, NormalizeProvider(tt.input))
		})
	}
}

func TestFirebaseIDToUUID(t *testing.T) {
	t.Parallel()
	t.Run("valid UUID passthrough", func(t *testing.T) {
		t.Parallel()
		id := "aaaaaaaa-0000-0000-0000-000000000001"
		testutil.Equal(t, id, FirebaseIDToUUID(id))
	})

	t.Run("non-UUID generates deterministic UUID v5", func(t *testing.T) {
		t.Parallel()
		result := FirebaseIDToUUID("abc123def456ghi789")
		// Must be valid UUID format.
		testutil.Equal(t, 36, len(result))
		testutil.True(t, result[8] == '-' && result[13] == '-' && result[18] == '-' && result[23] == '-', "should have dashes")
		// Must be deterministic.
		testutil.Equal(t, result, FirebaseIDToUUID("abc123def456ghi789"))
	})

	t.Run("different IDs produce different UUIDs", func(t *testing.T) {
		t.Parallel()
		a := FirebaseIDToUUID("firebase-user-1")
		b := FirebaseIDToUUID("firebase-user-2")
		testutil.NotEqual(t, a, b)
	})

	t.Run("short Firebase ID", func(t *testing.T) {
		t.Parallel()
		result := FirebaseIDToUUID("abc123")
		testutil.Equal(t, 36, len(result))
		testutil.Equal(t, result, FirebaseIDToUUID("abc123"))
	})
}

func TestEncodeFirebaseScryptHash(t *testing.T) {
	t.Parallel()
	t.Run("with valid config", func(t *testing.T) {
		t.Parallel()
		config := &FirebaseHashConfig{
			Base64SignerKey:     "c2lnbmVy",
			Base64SaltSeparator: "c2Vw",
			Rounds:              8,
			MemCost:             14,
		}
		result := EncodeFirebaseScryptHash("aGFzaA==", "c2FsdA==", config)
		testutil.Contains(t, result, "$firebase-scrypt$")
		testutil.Contains(t, result, "c2lnbmVy")
		testutil.Contains(t, result, "$8$14")
	})

	t.Run("nil config returns $none$", func(t *testing.T) {
		t.Parallel()
		result := EncodeFirebaseScryptHash("hash", "salt", nil)
		testutil.Equal(t, "$none$", result)
	})

	t.Run("empty hash returns $none$", func(t *testing.T) {
		t.Parallel()
		config := &FirebaseHashConfig{Rounds: 8, MemCost: 14}
		result := EncodeFirebaseScryptHash("", "salt", config)
		testutil.Equal(t, "$none$", result)
	})
}
