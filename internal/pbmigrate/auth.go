package pbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// AuthUser represents a PocketBase auth user record
type AuthUser struct {
	ID           string
	Email        string
	PasswordHash string
	Verified     bool
	CreatedAt    string
	UpdatedAt    string
	CustomFields map[string]interface{} // Additional fields from auth collection schema
}

// migrateAuthUsers migrates PocketBase auth users to _ayb_users table
func (m *Migrator) migrateAuthUsers(ctx context.Context, tx *sql.Tx, collections []PBCollection) error {
	// Find auth collections
	var authCollections []PBCollection
	for _, coll := range collections {
		if coll.Type == "auth" && !coll.System {
			authCollections = append(authCollections, coll)
		}
	}

	if len(authCollections) == 0 {
		if m.verbose {
			fmt.Fprintln(m.output, "  No auth collections found")
		}
		return nil
	}

	fmt.Fprintln(m.output, "Migrating auth users...")

	for _, coll := range authCollections {
		count, err := m.reader.CountRecords(coll.Name)
		if err != nil {
			return fmt.Errorf("failed to count users in %s: %w", coll.Name, err)
		}

		if count == 0 {
			if m.verbose {
				fmt.Fprintf(m.output, "  %s: 0 users (skipping)\n", coll.Name)
			}
			continue
		}

		// Read user records
		records, err := m.reader.ReadRecords(coll.Name, coll.Schema)
		if err != nil {
			return fmt.Errorf("failed to read users from %s: %w", coll.Name, err)
		}

		// Parse users
		users, err := parseAuthUsers(records, coll.Schema)
		if err != nil {
			return fmt.Errorf("failed to parse auth users: %w", err)
		}

		// Insert users and track ID mappings
		userIDMap := make(map[string]string) // pb_id -> ayb_id
		for _, user := range users {
			if !m.opts.DryRun {
				aybID, err := m.insertAuthUser(ctx, tx, user)
				if err != nil {
					return fmt.Errorf("failed to insert user %s: %w", user.Email, err)
				}
				userIDMap[user.ID] = aybID
			}
			m.stats.AuthUsers++
		}

		// Create user_profiles table if there are custom fields
		customFields := getCustomFields(coll.Schema)
		if len(customFields) > 0 {
			if !m.opts.DryRun {
				if err := m.createUserProfilesTable(ctx, tx, coll.Name, customFields); err != nil {
					return fmt.Errorf("failed to create user_profiles table: %w", err)
				}

				// Insert custom field data
				for _, user := range users {
					aybID := userIDMap[user.ID]
					if err := m.insertUserProfile(ctx, tx, coll.Name, aybID, user, customFields); err != nil {
						return fmt.Errorf("failed to insert user profile for %s: %w", user.Email, err)
					}
				}
			}
		}

		fmt.Fprintf(m.output, "  %s: %d users → _ayb_users\n", coll.Name, len(users))
	}

	fmt.Fprintln(m.output, "")
	return nil
}

// parseAuthUsers converts PBRecords to AuthUser structs
func parseAuthUsers(records []PBRecord, schema []PBField) ([]AuthUser, error) {
	users := make([]AuthUser, 0, len(records))

	for _, record := range records {
		user := AuthUser{
			ID:           record.ID,
			CustomFields: make(map[string]interface{}),
		}

		// Extract standard auth fields
		if email, ok := record.Data["email"].(string); ok {
			user.Email = email
		} else {
			return nil, fmt.Errorf("missing or invalid email for user %s", record.ID)
		}

		if hash, ok := record.Data["passwordHash"].(string); ok {
			user.PasswordHash = hash
		} else if hash, ok := record.Data["password"].(string); ok {
			// Try alternative field name
			user.PasswordHash = hash
		} else {
			return nil, fmt.Errorf("missing password hash for user %s", user.Email)
		}

		// Verified status (could be bool or int)
		if verified, ok := record.Data["verified"].(bool); ok {
			user.Verified = verified
		} else if verified, ok := record.Data["verified"].(int64); ok {
			user.Verified = verified != 0
		} else if verified, ok := record.Data["verified"].(int); ok {
			user.Verified = verified != 0
		}

		// Timestamps
		if created, ok := record.Data["created"].(string); ok {
			user.CreatedAt = created
		}
		if updated, ok := record.Data["updated"].(string); ok {
			user.UpdatedAt = updated
		}

		// Extract custom fields
		for _, field := range schema {
			if field.System {
				continue
			}
			// Skip standard auth fields
			if isStandardAuthField(field.Name) {
				continue
			}
			if val, ok := record.Data[field.Name]; ok {
				user.CustomFields[field.Name] = val
			}
		}

		users = append(users, user)
	}

	return users, nil
}

// isStandardAuthField checks if a field is a standard PocketBase auth field
func isStandardAuthField(name string) bool {
	standardFields := []string{
		"email", "passwordHash", "password", "verified",
		"emailVisibility", "tokenKey", "lastResetSentAt", "lastVerificationSentAt",
	}
	for _, field := range standardFields {
		if strings.EqualFold(name, field) {
			return true
		}
	}
	return false
}

// getCustomFields returns non-system, non-standard auth fields
func getCustomFields(schema []PBField) []PBField {
	var custom []PBField
	for _, field := range schema {
		if !field.System && !isStandardAuthField(field.Name) {
			custom = append(custom, field)
		}
	}
	return custom
}

// insertAuthUser inserts a user into _ayb_users table
// PocketBase uses 15-char alphanumeric IDs, but AYB expects UUIDs.
// We generate new UUIDs and store the mapping in _ayb_pb_id_map table.
// Returns the generated AYB UUID.
func (m *Migrator) insertAuthUser(ctx context.Context, tx *sql.Tx, user AuthUser) (string, error) {
	// First, create the _ayb_users table if it doesn't exist
	createUsersTableSQL := `
		CREATE TABLE IF NOT EXISTS _ayb_users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			email_verified BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_users_email ON _ayb_users (LOWER(email));
	`
	if _, err := tx.ExecContext(ctx, createUsersTableSQL); err != nil {
		return "", fmt.Errorf("failed to create _ayb_users table: %w", err)
	}

	// Create the mapping table if it doesn't exist
	createMapTableSQL := `
		CREATE TABLE IF NOT EXISTS _ayb_pb_id_map (
			pb_id TEXT PRIMARY KEY,
			ayb_id UUID NOT NULL UNIQUE,
			collection_name TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`
	if _, err := tx.ExecContext(ctx, createMapTableSQL); err != nil {
		return "", fmt.Errorf("failed to create ID mapping table: %w", err)
	}

	// Insert user with generated UUID
	// Note: PostgreSQL's gen_random_uuid() generates the UUID on insert
	insertUserSQL := `
		INSERT INTO _ayb_users (email, password_hash, email_verified, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	var aybID string
	err := tx.QueryRowContext(ctx, insertUserSQL,
		user.Email,
		user.PasswordHash,
		user.Verified,
		user.CreatedAt,
		user.UpdatedAt,
	).Scan(&aybID)

	if err != nil {
		return "", fmt.Errorf("failed to insert user: %w", err)
	}

	// Store the mapping
	insertMapSQL := `
		INSERT INTO _ayb_pb_id_map (pb_id, ayb_id, collection_name)
		VALUES ($1, $2, $3)
	`
	_, err = tx.ExecContext(ctx, insertMapSQL, user.ID, aybID, "users")
	if err != nil {
		return "", fmt.Errorf("failed to insert ID mapping: %w", err)
	}

	return aybID, nil
}

// createUserProfilesTable creates a table for custom user fields
func (m *Migrator) createUserProfilesTable(ctx context.Context, tx *sql.Tx, collectionName string, customFields []PBField) error {
	tableName := SanitizeIdentifier(fmt.Sprintf("_ayb_user_profiles_%s", collectionName))

	// Build CREATE TABLE statement
	var columns []string
	columns = append(columns, "user_id UUID PRIMARY KEY REFERENCES _ayb_users(id) ON DELETE CASCADE")

	for _, field := range customFields {
		pgType := FieldTypeToPgType(field)
		col := fmt.Sprintf("%s %s", SanitizeIdentifier(field.Name), pgType)
		if field.Required {
			col += " NOT NULL"
		}
		columns = append(columns, col)
	}

	columns = append(columns, "created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()")
	columns = append(columns, "updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()")

	sql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)",
		tableName,
		strings.Join(columns, ", "))

	_, err := tx.ExecContext(ctx, sql)
	return err
}

// insertUserProfile inserts custom field data into user_profiles table
func (m *Migrator) insertUserProfile(ctx context.Context, tx *sql.Tx, collectionName string, aybUserID string, user AuthUser, customFields []PBField) error {
	if len(user.CustomFields) == 0 {
		return nil
	}

	tableName := SanitizeIdentifier(fmt.Sprintf("_ayb_user_profiles_%s", collectionName))

	// Build INSERT statement
	columns := []string{"user_id"}
	placeholders := []string{"$1"}
	values := []interface{}{aybUserID}

	idx := 2
	for _, field := range customFields {
		if val, ok := user.CustomFields[field.Name]; ok {
			columns = append(columns, SanitizeIdentifier(field.Name))
			placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
			values = append(values, val)
			idx++
		}
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	_, err := tx.ExecContext(ctx, query, values...)
	return err
}
