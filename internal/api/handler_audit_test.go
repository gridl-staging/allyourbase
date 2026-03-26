package api

import (
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

// Tests for pkMap — the helper that extracts PK values from a scanned record.

func TestPkMapSinglePK(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid"},
			{Name: "email", TypeName: "text"},
		},
	}
	record := map[string]any{
		"id":    "abc-123",
		"email": "test@example.com",
	}
	got := pkMap(tbl, record)
	testutil.Equal(t, 1, len(got))
	testutil.Equal(t, "abc-123", got["id"])
}

func TestPkMapCompositePK(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Name:       "memberships",
		PrimaryKey: []string{"user_id", "org_id"},
		Columns: []*schema.Column{
			{Name: "user_id", TypeName: "uuid"},
			{Name: "org_id", TypeName: "uuid"},
			{Name: "role", TypeName: "text"},
		},
	}
	record := map[string]any{
		"user_id": "u-1",
		"org_id":  "o-2",
		"role":    "admin",
	}
	got := pkMap(tbl, record)
	testutil.Equal(t, 2, len(got))
	testutil.Equal(t, "u-1", got["user_id"])
	testutil.Equal(t, "o-2", got["org_id"])
}

func TestPkMapMissingPKFieldReturnsNil(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns:    []*schema.Column{{Name: "id", TypeName: "uuid"}},
	}
	// Record doesn't include the PK column — result should still be a map (with nil value).
	record := map[string]any{"email": "x@y.com"}
	got := pkMap(tbl, record)
	testutil.Equal(t, 1, len(got))
	testutil.Equal(t, nil, got["id"])
}

// Tests for extractOldRecord — parses and removes _audit_old_values from a record.

func TestExtractOldRecordPresentString(t *testing.T) {
	t.Parallel()
	record := map[string]any{
		"id":                "abc",
		"name":              "Alice",
		"_audit_old_values": `{"id":"abc","name":"Old"}`,
	}
	old := extractOldRecord(record)
	// Key must be removed from record.
	_, exists := record["_audit_old_values"]
	testutil.Equal(t, false, exists)
	testutil.NotNil(t, old)
	testutil.Equal(t, "abc", old["id"])
	testutil.Equal(t, "Old", old["name"])
}

func TestExtractOldRecordPresentBytes(t *testing.T) {
	t.Parallel()
	record := map[string]any{
		"id":                "abc",
		"_audit_old_values": []byte(`{"id":"abc","name":"Old"}`),
	}
	old := extractOldRecord(record)
	testutil.NotNil(t, old)
	testutil.Equal(t, "abc", old["id"])
	testutil.Equal(t, "Old", old["name"])
	_, exists := record["_audit_old_values"]
	testutil.Equal(t, false, exists)
}

func TestExtractOldRecordPresentMap(t *testing.T) {
	t.Parallel()
	record := map[string]any{
		"id":                "abc",
		"_audit_old_values": map[string]any{"id": "abc", "name": "Old"},
	}
	old := extractOldRecord(record)
	testutil.NotNil(t, old)
	testutil.Equal(t, "abc", old["id"])
	testutil.Equal(t, "Old", old["name"])
}

func TestExtractOldRecordAbsent(t *testing.T) {
	t.Parallel()
	record := map[string]any{
		"id":   "abc",
		"name": "Alice",
	}
	old := extractOldRecord(record)
	testutil.True(t, old == nil, "expected nil old record when sentinel is absent")
	// Record unchanged.
	testutil.Equal(t, 2, len(record))
}

func TestExtractOldRecordNilRecord(t *testing.T) {
	t.Parallel()
	old := extractOldRecord(nil)
	testutil.True(t, old == nil, "expected nil old record for nil input")
}
