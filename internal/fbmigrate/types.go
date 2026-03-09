// Package fbmigrate migrates auth users, OAuth identities, and Firestore data
// from a Firebase project to AYB.
package fbmigrate

import (
	"github.com/allyourbase/ayb/internal/migrate"
)

// FirebaseUser represents a user from a Firebase auth export.
type FirebaseUser struct {
	LocalID       string         `json:"localId"`
	Email         string         `json:"email"`
	PasswordHash  string         `json:"passwordHash"` // base64-encoded
	Salt          string         `json:"salt"`         // base64-encoded
	EmailVerified bool           `json:"emailVerified"`
	DisplayName   string         `json:"displayName"`
	ProviderInfo  []ProviderInfo `json:"providerUserInfo"`
	CreatedAt     string         `json:"createdAt"` // millisecond epoch string
	LastLoginAt   string         `json:"lastLoginAt"`
	Disabled      bool           `json:"disabled"`
}

// ProviderInfo represents an OAuth provider linked to a Firebase user.
type ProviderInfo struct {
	ProviderID  string `json:"providerId"` // "google.com", "github.com", "password"
	RawID       string `json:"rawId"`      // provider-specific user ID
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

// FirebaseHashConfig contains the project-level hash configuration from the auth export.
type FirebaseHashConfig struct {
	Algorithm           string `json:"algorithm"` // "SCRYPT"
	Base64SignerKey     string `json:"base64_signer_key"`
	Base64SaltSeparator string `json:"base64_salt_separator"`
	Rounds              int    `json:"rounds"`
	MemCost             int    `json:"mem_cost"`
}

// FirebaseAuthExport is the top-level structure of a Firebase auth export JSON file.
type FirebaseAuthExport struct {
	Users      []FirebaseUser     `json:"users"`
	HashConfig FirebaseHashConfig `json:"hash_config"`
}

// FirestoreDocument represents a single document from a Firestore export.
type FirestoreDocument struct {
	ID     string         `json:"__name__"`
	Fields map[string]any `json:"fields"`
}

// FirestoreCollection represents a named collection with its documents.
type FirestoreCollection struct {
	Name      string
	Documents []FirestoreDocument
}

// MigrationStats tracks Firebase migration progress.
type MigrationStats struct {
	Users        int      `json:"users"`
	OAuthLinks   int      `json:"oauthLinks"`
	Collections  int      `json:"collections"`
	Documents    int      `json:"documents"`
	RTDBNodes    int      `json:"rtdbNodes"`
	RTDBRecords  int      `json:"rtdbRecords"`
	StorageFiles int      `json:"storageFiles"`
	StorageBytes int64    `json:"storageBytes"`
	Skipped      int      `json:"skipped"`
	Errors       []string `json:"errors,omitempty"`
}

// MigrationOptions configures the Firebase migration process.
type MigrationOptions struct {
	AuthExportPath      string // path to Firebase auth export JSON
	FirestoreExportPath string // path to Firestore export directory
	RTDBExportPath      string // path to RTDB JSON export file
	StorageExportPath   string // path to Cloud Storage export directory
	StoragePath         string // destination path for AYB storage (default: ./ayb_storage)
	DatabaseURL         string // AYB PostgreSQL connection URL
	HashConfig          *FirebaseHashConfig
	DryRun              bool
	Verbose             bool
	Progress            migrate.ProgressReporter
}
