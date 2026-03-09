package pbmigrate

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestParseAuthUsers(t *testing.T) {
	t.Parallel()
	t.Run("basic auth user", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
			{Name: "verified", Type: "bool", System: false},
		}

		records := []PBRecord{
			{
				ID: "abc123xyz456789",
				Data: map[string]interface{}{
					"email":        "user@example.com",
					"passwordHash": "$2a$10$hashedhashhashedhashedhashhedhashhashhashhash",
					"verified":     true,
					"created":      "2024-01-01 00:00:00.000Z",
					"updated":      "2024-01-02 00:00:00.000Z",
				},
			},
		}

		users, err := parseAuthUsers(records, schema)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(users))

		user := users[0]
		testutil.Equal(t, "abc123xyz456789", user.ID)
		testutil.Equal(t, "user@example.com", user.Email)
		testutil.Equal(t, "$2a$10$hashedhashhashedhashedhashhedhashhashhashhash", user.PasswordHash)
		testutil.True(t, user.Verified)
		testutil.Equal(t, "2024-01-01 00:00:00.000Z", user.CreatedAt)
		testutil.Equal(t, "2024-01-02 00:00:00.000Z", user.UpdatedAt)
	})

	t.Run("verified as integer", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
			{Name: "verified", Type: "bool", System: false},
		}

		records := []PBRecord{
			{
				ID: "test123",
				Data: map[string]interface{}{
					"email":        "user@example.com",
					"passwordHash": "bcrypthash",
					"verified":     int64(1), // SQLite might return as int
					"created":      "2024-01-01 00:00:00.000Z",
					"updated":      "2024-01-01 00:00:00.000Z",
				},
			},
		}

		users, err := parseAuthUsers(records, schema)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(users))
		testutil.True(t, users[0].Verified)
	})

	t.Run("unverified user", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
			{Name: "verified", Type: "bool", System: false},
		}

		records := []PBRecord{
			{
				ID: "test123",
				Data: map[string]interface{}{
					"email":        "user@example.com",
					"passwordHash": "bcrypthash",
					"verified":     false,
					"created":      "2024-01-01 00:00:00.000Z",
					"updated":      "2024-01-01 00:00:00.000Z",
				},
			},
		}

		users, err := parseAuthUsers(records, schema)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(users))
		testutil.False(t, users[0].Verified)
	})

	t.Run("custom fields extracted", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
			{Name: "verified", Type: "bool", System: false},
			{Name: "name", Type: "text", System: false},
			{Name: "avatar", Type: "file", System: false},
			{Name: "role", Type: "select", System: false},
		}

		records := []PBRecord{
			{
				ID: "test123",
				Data: map[string]interface{}{
					"email":        "user@example.com",
					"passwordHash": "bcrypthash",
					"verified":     true,
					"name":         "John Doe",
					"avatar":       "avatar.jpg",
					"role":         "admin",
					"created":      "2024-01-01 00:00:00.000Z",
					"updated":      "2024-01-01 00:00:00.000Z",
				},
			},
		}

		users, err := parseAuthUsers(records, schema)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(users))

		user := users[0]
		testutil.Equal(t, 3, len(user.CustomFields))
		testutil.Equal(t, "John Doe", user.CustomFields["name"])
		testutil.Equal(t, "avatar.jpg", user.CustomFields["avatar"])
		testutil.Equal(t, "admin", user.CustomFields["role"])
	})

	t.Run("missing email error", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "passwordHash", Type: "text", System: false},
		}

		records := []PBRecord{
			{
				ID: "test123",
				Data: map[string]interface{}{
					"passwordHash": "bcrypthash",
				},
			},
		}

		_, err := parseAuthUsers(records, schema)
		testutil.ErrorContains(t, err, "missing or invalid email")
	})

	t.Run("missing password hash error", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
		}

		records := []PBRecord{
			{
				ID: "test123",
				Data: map[string]interface{}{
					"email": "user@example.com",
				},
			},
		}

		_, err := parseAuthUsers(records, schema)
		testutil.ErrorContains(t, err, "missing password hash")
	})

	t.Run("alternative password field name", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
			{Name: "password", Type: "text", System: false},
		}

		records := []PBRecord{
			{
				ID: "test123",
				Data: map[string]interface{}{
					"email":    "user@example.com",
					"password": "bcrypthash",
				},
			},
		}

		users, err := parseAuthUsers(records, schema)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(users))
		testutil.Equal(t, "bcrypthash", users[0].PasswordHash)
	})

	t.Run("multiple users", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
			{Name: "verified", Type: "bool", System: false},
		}

		records := []PBRecord{
			{
				ID: "user1",
				Data: map[string]interface{}{
					"email":        "user1@example.com",
					"passwordHash": "hash1",
					"verified":     true,
				},
			},
			{
				ID: "user2",
				Data: map[string]interface{}{
					"email":        "user2@example.com",
					"passwordHash": "hash2",
					"verified":     false,
				},
			},
			{
				ID: "user3",
				Data: map[string]interface{}{
					"email":        "user3@example.com",
					"passwordHash": "hash3",
					"verified":     true,
				},
			},
		}

		users, err := parseAuthUsers(records, schema)
		testutil.NoError(t, err)
		testutil.Equal(t, 3, len(users))
		testutil.Equal(t, "user1@example.com", users[0].Email)
		testutil.Equal(t, "user2@example.com", users[1].Email)
		testutil.Equal(t, "user3@example.com", users[2].Email)
	})
}

func TestIsStandardAuthField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expected bool
	}{
		{"email", true},
		{"passwordHash", true},
		{"password", true},
		{"verified", true},
		{"emailVisibility", true},
		{"tokenKey", true},
		{"lastResetSentAt", true},
		{"lastVerificationSentAt", true},
		{"name", false},
		{"avatar", false},
		{"role", false},
		{"bio", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isStandardAuthField(tt.name)
			testutil.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCustomFields(t *testing.T) {
	t.Parallel()
	t.Run("no custom fields", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "id", Type: "text", System: true},
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
			{Name: "verified", Type: "bool", System: false},
		}

		custom := getCustomFields(schema)
		testutil.Equal(t, 0, len(custom))
	})

	t.Run("with custom fields", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "id", Type: "text", System: true},
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
			{Name: "verified", Type: "bool", System: false},
			{Name: "name", Type: "text", System: false},
			{Name: "avatar", Type: "file", System: false},
			{Name: "role", Type: "select", System: false},
		}

		custom := getCustomFields(schema)
		testutil.Equal(t, 3, len(custom))
		testutil.Equal(t, "name", custom[0].Name)
		testutil.Equal(t, "avatar", custom[1].Name)
		testutil.Equal(t, "role", custom[2].Name)
	})

	t.Run("filters system fields", func(t *testing.T) {
		t.Parallel()
		schema := []PBField{
			{Name: "id", Type: "text", System: true},
			{Name: "created", Type: "date", System: true},
			{Name: "updated", Type: "date", System: true},
			{Name: "email", Type: "email", System: false},
			{Name: "passwordHash", Type: "text", System: false},
			{Name: "name", Type: "text", System: false},
		}

		custom := getCustomFields(schema)
		testutil.Equal(t, 1, len(custom))
		testutil.Equal(t, "name", custom[0].Name)
	})
}
