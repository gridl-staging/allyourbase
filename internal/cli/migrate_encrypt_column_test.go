package cli

import "testing"

func TestValidateSQLIdentifierAllowsSchemaQualifiedTables(t *testing.T) {
	t.Helper()

	if err := validateSQLIdentifier("users"); err != nil {
		t.Fatalf("expected plain table identifier to be valid: %v", err)
	}
	if err := validateSQLIdentifier("public.users"); err != nil {
		t.Fatalf("expected schema-qualified table identifier to be valid: %v", err)
	}
}

func TestValidateSQLIdentifierRejectsInvalidIdentifiers(t *testing.T) {
	t.Helper()

	if err := validateSQLIdentifier(""); err == nil {
		t.Fatal("expected empty identifier to be rejected")
	}
	if err := validateSQLIdentifier("users table"); err == nil {
		t.Fatal("expected identifier with space to be rejected")
	}
	if err := validateSQLIdentifier("weird.name.table."); err == nil {
		t.Fatal("expected trailing dot identifier to be rejected")
	}
}

func TestQuoteQualifiedIdentifier(t *testing.T) {
	t.Helper()

	quoted, err := quoteQualifiedIdentifier("public.users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const want = `"public"."users"`
	if quoted != want {
		t.Fatalf("expected %q, got %q", want, quoted)
	}
}

func TestMigrateCommandIncludesEncryptColumnSubcommand(t *testing.T) {
	t.Helper()

	found := false
	for _, cmd := range migrateCmd.Commands() {
		if cmd.Name() == "encrypt-column" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected migrate command to include encrypt-column")
	}
}
