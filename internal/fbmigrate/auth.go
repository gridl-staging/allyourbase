package fbmigrate

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ParseAuthExport reads and parses a Firebase auth export JSON file.
// Returns the list of users and the project hash configuration.
func ParseAuthExport(path string) ([]FirebaseUser, *FirebaseHashConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading auth export: %w", err)
	}

	var export FirebaseAuthExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, nil, fmt.Errorf("parsing auth export JSON: %w", err)
	}

	return export.Users, &export.HashConfig, nil
}

// IsEmailUser returns true if the user has an email address (not phone-only or anonymous).
func IsEmailUser(u FirebaseUser) bool {
	return u.Email != ""
}

// IsPasswordUser returns true if the user has a password hash (not OAuth-only).
func IsPasswordUser(u FirebaseUser) bool {
	return u.PasswordHash != ""
}

// IsAnonymousUser returns true if the user has no email, no providers, and no password.
func IsAnonymousUser(u FirebaseUser) bool {
	return u.Email == "" && len(u.ProviderInfo) == 0 && u.PasswordHash == ""
}

// IsPhoneOnlyUser returns true if the user's only provider is "phone".
func IsPhoneOnlyUser(u FirebaseUser) bool {
	if u.Email != "" {
		return false
	}
	for _, p := range u.ProviderInfo {
		if p.ProviderID != "phone" {
			return false
		}
	}
	return len(u.ProviderInfo) > 0
}

// OAuthProviders extracts OAuth provider info from a user, excluding "password" and "phone".
func OAuthProviders(u FirebaseUser) []ProviderInfo {
	var providers []ProviderInfo
	for _, p := range u.ProviderInfo {
		if p.ProviderID == "password" || p.ProviderID == "phone" {
			continue
		}
		providers = append(providers, p)
	}
	return providers
}

// NormalizeProvider converts Firebase provider IDs to AYB provider names.
// "google.com" → "google", "github.com" → "github", etc.
func NormalizeProvider(providerID string) string {
	switch {
	case strings.HasPrefix(providerID, "google"):
		return "google"
	case strings.HasPrefix(providerID, "github"):
		return "github"
	case strings.HasPrefix(providerID, "facebook"):
		return "facebook"
	case strings.HasPrefix(providerID, "twitter"):
		return "twitter"
	case strings.HasPrefix(providerID, "apple"):
		return "apple"
	case strings.HasPrefix(providerID, "microsoft"):
		return "microsoft"
	default:
		// Strip ".com" suffix if present.
		return strings.TrimSuffix(providerID, ".com")
	}
}

// uuidRegex matches standard UUID format (8-4-4-4-12 hex chars).
var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// firebaseNamespace is a fixed UUID v5 namespace for generating deterministic
// UUIDs from Firebase LocalIDs. Generated as a random v4 UUID.
var firebaseNamespace = [16]byte{
	0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1,
	0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
}

// FirebaseIDToUUID converts a Firebase LocalID to a UUID suitable for
// _ayb_users.id. If the LocalID is already a valid UUID, it is returned as-is.
// Otherwise, a deterministic UUID v5 is generated from the LocalID using a
// fixed namespace, ensuring the same Firebase ID always maps to the same UUID.
func FirebaseIDToUUID(localID string) string {
	if uuidRegex.MatchString(localID) {
		return localID
	}
	return uuidV5(firebaseNamespace, localID)
}

// uuidV5 generates a UUID v5 (SHA-1 based) from a namespace UUID and a name string.
func uuidV5(namespace [16]byte, name string) string {
	h := sha1.New()
	h.Write(namespace[:])
	h.Write([]byte(name))
	sum := h.Sum(nil)

	// Set version (5) and variant (RFC 4122).
	sum[6] = (sum[6] & 0x0f) | 0x50 // version 5
	sum[8] = (sum[8] & 0x3f) | 0x80 // variant RFC 4122

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// EncodeFirebaseScryptHash encodes hash parameters into AYB's storage format.
// Format: $firebase-scrypt$<base64-params-json>$<base64-salt>$<base64-hash>
//
// The params JSON contains signerKey, saltSeparator, rounds, and memCost
// so the hash can be verified later without the original export config.
func EncodeFirebaseScryptHash(passwordHash, salt string, config *FirebaseHashConfig) string {
	if config == nil || passwordHash == "" {
		return "$none$"
	}
	// Store the hash config + user's password hash inline so verification works standalone.
	// Format: $firebase-scrypt$<signerKey>$<saltSep>$<salt>$<rounds>$<memCost>$<passwordHash>
	return fmt.Sprintf("$firebase-scrypt$%s$%s$%s$%d$%d$%s",
		config.Base64SignerKey, config.Base64SaltSeparator,
		salt, config.Rounds, config.MemCost, passwordHash)
}
