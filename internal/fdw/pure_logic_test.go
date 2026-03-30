package fdw

import (
	"context"
	"strings"
	"testing"
)

func TestEscapeSQLLiteral(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"it's", "it''s"},
		{"a''b", "a''''b"},           // already-escaped quotes get double-escaped
		{"", ""},                     // empty string
		{"no quotes", "no quotes"},   // no change
		{"'", "''"},                  // single quote only
		{"O'Brien's", "O''Brien''s"}, // multiple quotes
	}
	for _, tc := range tests {
		got := escapeSQLLiteral(tc.input)
		if got != tc.want {
			t.Errorf("escapeSQLLiteral(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestBuildOptionsClause(t *testing.T) {
	t.Parallel()

	t.Run("single key", func(t *testing.T) {
		got := buildOptionsClause([]string{"filename"}, map[string]string{
			"filename": "/tmp/data.csv",
		})
		if got != "filename '/tmp/data.csv'" {
			t.Fatalf("got %q, want %q", got, "filename '/tmp/data.csv'")
		}
	})

	t.Run("multiple keys sorted alphabetically", func(t *testing.T) {
		// Keys should be sorted regardless of input order.
		got := buildOptionsClause([]string{"port", "host", "dbname"}, map[string]string{
			"host":   "localhost",
			"port":   "5432",
			"dbname": "analytics",
		})
		// Expected: dbname, host, port (alphabetical)
		if !strings.HasPrefix(got, "dbname 'analytics'") {
			t.Fatalf("expected dbname first, got %q", got)
		}
		if !strings.Contains(got, "host 'localhost'") {
			t.Fatalf("missing host clause in %q", got)
		}
		if !strings.HasSuffix(got, "port '5432'") {
			t.Fatalf("expected port last, got %q", got)
		}
	})

	t.Run("escapes single quotes in values", func(t *testing.T) {
		got := buildOptionsClause([]string{"host"}, map[string]string{
			"host": "db'server",
		})
		if got != "host 'db''server'" {
			t.Fatalf("got %q, want %q", got, "host 'db''server'")
		}
	})

	t.Run("empty keys list", func(t *testing.T) {
		got := buildOptionsClause(nil, map[string]string{})
		if got != "" {
			t.Fatalf("got %q, want empty string", got)
		}
	})
}

func TestRequiredOptionKeys(t *testing.T) {
	t.Parallel()

	t.Run("postgres_fdw valid", func(t *testing.T) {
		keys, err := requiredOptionKeys(CreateServerOpts{
			FDWType: "postgres_fdw",
			Options: map[string]string{"host": "h", "port": "5432", "dbname": "d"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should return the canonical key list.
		want := []string{"dbname", "host", "port"}
		if len(keys) != len(want) {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
		for i, k := range keys {
			if k != want[i] {
				t.Fatalf("keys[%d] = %q, want %q", i, k, want[i])
			}
		}
	})

	t.Run("postgres_fdw missing host", func(t *testing.T) {
		_, err := requiredOptionKeys(CreateServerOpts{
			FDWType: "postgres_fdw",
			Options: map[string]string{"port": "5432", "dbname": "d"},
		})
		if err == nil {
			t.Fatal("expected error for missing host")
		}
		if !strings.Contains(err.Error(), "host") {
			t.Fatalf("error should mention 'host', got: %v", err)
		}
	})

	t.Run("postgres_fdw missing port", func(t *testing.T) {
		_, err := requiredOptionKeys(CreateServerOpts{
			FDWType: "postgres_fdw",
			Options: map[string]string{"host": "h", "dbname": "d"},
		})
		if err == nil {
			t.Fatal("expected error for missing port")
		}
	})

	t.Run("postgres_fdw missing dbname", func(t *testing.T) {
		_, err := requiredOptionKeys(CreateServerOpts{
			FDWType: "postgres_fdw",
			Options: map[string]string{"host": "h", "port": "5432"},
		})
		if err == nil {
			t.Fatal("expected error for missing dbname")
		}
	})

	t.Run("postgres_fdw whitespace-only option treated as missing", func(t *testing.T) {
		_, err := requiredOptionKeys(CreateServerOpts{
			FDWType: "postgres_fdw",
			Options: map[string]string{"host": "  ", "port": "5432", "dbname": "d"},
		})
		if err == nil {
			t.Fatal("expected error for whitespace-only host")
		}
	})

	t.Run("file_fdw valid", func(t *testing.T) {
		keys, err := requiredOptionKeys(CreateServerOpts{
			FDWType: "file_fdw",
			Options: map[string]string{"filename": "/tmp/data.csv"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(keys) != 1 || keys[0] != "filename" {
			t.Fatalf("keys = %v, want [filename]", keys)
		}
	})

	t.Run("file_fdw missing filename", func(t *testing.T) {
		_, err := requiredOptionKeys(CreateServerOpts{
			FDWType: "file_fdw",
			Options: map[string]string{},
		})
		if err == nil {
			t.Fatal("expected error for missing filename")
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		_, err := requiredOptionKeys(CreateServerOpts{FDWType: "oracle_fdw"})
		if err == nil {
			t.Fatal("expected error for unsupported fdw type")
		}
	})
}

func TestFdwPasswordSecretKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		serverName string
		want       string
	}{
		{"analytics_fdw", "fdw.analytics_fdw.password"},
		{"my_server", "fdw.my_server.password"},
		{"x", "fdw.x.password"},
	}
	for _, tc := range tests {
		got := fdwPasswordSecretKey(tc.serverName)
		if got != tc.want {
			t.Errorf("fdwPasswordSecretKey(%q) = %q, want %q", tc.serverName, got, tc.want)
		}
	}
}

func TestBuildImportSchemaSQL(t *testing.T) {
	t.Parallel()

	t.Run("no table filter", func(t *testing.T) {
		got := buildImportSchemaSQL("srv", "remote", "local", nil)
		want := `IMPORT FOREIGN SCHEMA "remote" FROM SERVER "srv" INTO "local"`
		if got != want {
			t.Fatalf("got:\n  %s\nwant:\n  %s", got, want)
		}
	})

	t.Run("with table filter", func(t *testing.T) {
		got := buildImportSchemaSQL("srv", "public", "local", []string{"events", "users"})
		if !strings.Contains(got, `LIMIT TO ("events", "users")`) {
			t.Fatalf("missing LIMIT TO clause in: %s", got)
		}
		if !strings.Contains(got, `FROM SERVER "srv"`) {
			t.Fatalf("missing FROM SERVER in: %s", got)
		}
	})

	t.Run("single table filter", func(t *testing.T) {
		got := buildImportSchemaSQL("srv", "public", "local", []string{"only_me"})
		if !strings.Contains(got, `LIMIT TO ("only_me")`) {
			t.Fatalf("missing single-table LIMIT TO clause in: %s", got)
		}
	})
}

func TestValidateIdentifier_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("exactly 63 chars is valid", func(t *testing.T) {
		id := strings.Repeat("a", 63)
		if err := ValidateIdentifier(id); err != nil {
			t.Fatalf("63-char identifier should be valid, got: %v", err)
		}
	})

	t.Run("64 chars is too long", func(t *testing.T) {
		id := strings.Repeat("a", 64)
		if err := ValidateIdentifier(id); err == nil {
			t.Fatal("64-char identifier should be rejected")
		}
	})

	t.Run("leading underscore is valid", func(t *testing.T) {
		if err := ValidateIdentifier("_private"); err != nil {
			t.Fatalf("leading underscore should be valid, got: %v", err)
		}
	})

	t.Run("digits after first char are valid", func(t *testing.T) {
		if err := ValidateIdentifier("col123"); err != nil {
			t.Fatalf("digits after first char should be valid, got: %v", err)
		}
	})

	t.Run("hyphen is invalid", func(t *testing.T) {
		if err := ValidateIdentifier("my-server"); err == nil {
			t.Fatal("hyphen should be invalid in PostgreSQL identifier")
		}
	})

	t.Run("space is invalid", func(t *testing.T) {
		if err := ValidateIdentifier("my server"); err == nil {
			t.Fatal("space should be invalid")
		}
	})

	t.Run("dot is invalid", func(t *testing.T) {
		if err := ValidateIdentifier("schema.table"); err == nil {
			t.Fatal("dot should be invalid — qualified names are not identifiers")
		}
	})
}

func TestCreateServer_ValidationErrors(t *testing.T) {
	t.Parallel()

	tx := &mockTx{}
	db := &mockDB{beginTx: tx}
	vs := &mockVaultStore{}
	svc := NewService(db, vs)

	t.Run("empty name", func(t *testing.T) {
		err := svc.CreateServer(context.Background(), CreateServerOpts{
			Name:    "",
			FDWType: "postgres_fdw",
			Options: map[string]string{"host": "h", "port": "5432", "dbname": "d"},
		})
		if err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("invalid fdw type", func(t *testing.T) {
		err := svc.CreateServer(context.Background(), CreateServerOpts{
			Name:    "srv",
			FDWType: "oracle_fdw",
		})
		if err == nil {
			t.Fatal("expected error for unsupported fdw type")
		}
	})

	t.Run("postgres_fdw missing user mapping user", func(t *testing.T) {
		err := svc.CreateServer(context.Background(), CreateServerOpts{
			Name:    "srv",
			FDWType: "postgres_fdw",
			Options: map[string]string{"host": "h", "port": "5432", "dbname": "d"},
			UserMapping: UserMapping{
				User:     "",
				Password: "pass",
			},
		})
		if err == nil {
			t.Fatal("expected error for empty user mapping user")
		}
	})

	t.Run("postgres_fdw missing user mapping password", func(t *testing.T) {
		err := svc.CreateServer(context.Background(), CreateServerOpts{
			Name:    "srv",
			FDWType: "postgres_fdw",
			Options: map[string]string{"host": "h", "port": "5432", "dbname": "d"},
			UserMapping: UserMapping{
				User:     "u",
				Password: "",
			},
		})
		if err == nil {
			t.Fatal("expected error for empty user mapping password")
		}
	})
}
