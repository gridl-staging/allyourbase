package backup

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestObjectKey(t *testing.T) {
	ts := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	key := ObjectKey("backups", "mydb", "abc123", ts)

	if !strings.HasPrefix(key, "backups/mydb/2026/03/15/") {
		t.Errorf("unexpected prefix: %q", key)
	}
	if !strings.Contains(key, "20260315T143000Z") {
		t.Errorf("expected timestamp in key: %q", key)
	}
	if !strings.HasSuffix(key, "abc123.sql.gz") {
		t.Errorf("expected backup id suffix: %q", key)
	}
}

func TestObjectKeyTrailingSlashInPrefix(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	key := ObjectKey("backups/", "mydb", "id1", ts)
	if strings.Contains(key, "//") {
		t.Errorf("double slash in key: %q", key)
	}
}

func TestObjectKeyZeroPaddedDate(t *testing.T) {
	ts := time.Date(2026, 1, 5, 9, 7, 3, 0, time.UTC)
	key := ObjectKey("pfx", "testdb", "uuid-1", ts)
	if !strings.Contains(key, "/2026/01/05/") {
		t.Errorf("expected zero-padded date: %q", key)
	}
	if !strings.Contains(key, "20260105T090703Z") {
		t.Errorf("expected zero-padded timestamp: %q", key)
	}
}

// --- BaseBackupKey ---

func TestBaseBackupKey(t *testing.T) {
	ts := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	key := BaseBackupKey("staging", "proj1", "db1", "0/1000000", ts)

	if !strings.HasPrefix(key, "staging/projects/proj1/db/db1/base/") {
		t.Errorf("unexpected prefix: %q", key)
	}
	if !strings.Contains(key, "2026/03/15") {
		t.Errorf("expected date in key: %q", key)
	}
	if !strings.Contains(key, "20260315T143000Z") {
		t.Errorf("expected timestamp in key: %q", key)
	}
	if !strings.HasSuffix(key, ".tar.zst") {
		t.Errorf("expected .tar.zst suffix: %q", key)
	}
	if strings.Contains(key, "//") {
		t.Errorf("double slash in key: %q", key)
	}
}

func TestBaseBackupKeyEmptyPrefix(t *testing.T) {
	ts := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	key := BaseBackupKey("", "proj1", "db1", "0/2000000", ts)

	if strings.HasPrefix(key, "/") {
		t.Errorf("key must not start with slash when prefix is empty: %q", key)
	}
	if !strings.HasPrefix(key, "projects/proj1/db/db1/base/") {
		t.Errorf("expected path to start with projects/: %q", key)
	}
}

func TestBaseBackupKeyTrailingSlashInPrefix(t *testing.T) {
	ts := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	key := BaseBackupKey("env/prod/", "proj1", "db1", "0/1000000", ts)
	if strings.Contains(key, "//") {
		t.Errorf("double slash in key: %q", key)
	}
}

func TestBaseBackupKeyZeroPaddedDate(t *testing.T) {
	ts := time.Date(2026, 1, 5, 9, 7, 3, 0, time.UTC)
	key := BaseBackupKey("pfx", "p", "d", "0/1000000", ts)
	if !strings.Contains(key, "2026/01/05") {
		t.Errorf("expected zero-padded date: %q", key)
	}
	if !strings.Contains(key, "20260105T090703Z") {
		t.Errorf("expected zero-padded timestamp: %q", key)
	}
}

// --- WALSegmentKey ---

func TestWALSegmentKey(t *testing.T) {
	key := WALSegmentKey("", "proj1", "db1", 1, "000000010000000000000001")

	expected := "projects/proj1/db/db1/wal/00000001/000000010000000000000001"
	if key != expected {
		t.Errorf("WALSegmentKey = %q; want %q", key, expected)
	}
}

func TestWALSegmentKeyWithPrefix(t *testing.T) {
	key := WALSegmentKey("staging", "proj1", "db1", 1, "000000010000000000000001")

	if !strings.HasPrefix(key, "staging/projects/proj1/db/db1/wal/") {
		t.Errorf("unexpected prefix: %q", key)
	}
	if strings.Contains(key, "//") {
		t.Errorf("double slash in key: %q", key)
	}
}

func TestWALSegmentKeyTimelineZeroPadded(t *testing.T) {
	key := WALSegmentKey("", "proj1", "db1", 3, "000000030000000000000007")
	if !strings.Contains(key, "/wal/00000003/") {
		t.Errorf("expected zero-padded timeline in path: %q", key)
	}
}

func TestWALSegmentKeyEmptyPrefixNoLeadingSlash(t *testing.T) {
	key := WALSegmentKey("", "proj1", "db1", 1, "seg1")
	if strings.HasPrefix(key, "/") {
		t.Errorf("key must not start with slash: %q", key)
	}
}

// --- ManifestKey ---

func TestManifestKey(t *testing.T) {
	ts := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	key := ManifestKey("", "proj1", "db1", ts)

	expected := "projects/proj1/db/db1/manifests/20260315T143000Z.json"
	if key != expected {
		t.Errorf("ManifestKey = %q; want %q", key, expected)
	}
}

func TestManifestKeyWithPrefix(t *testing.T) {
	ts := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	key := ManifestKey("env/prod", "proj1", "db1", ts)

	if !strings.HasPrefix(key, "env/prod/projects/proj1/db/db1/manifests/") {
		t.Errorf("unexpected prefix: %q", key)
	}
	if !strings.HasSuffix(key, ".json") {
		t.Errorf("expected .json suffix: %q", key)
	}
	if strings.Contains(key, "//") {
		t.Errorf("double slash in key: %q", key)
	}
}

func TestManifestKeyEmptyPrefixNoLeadingSlash(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	key := ManifestKey("", "p", "d", ts)
	if strings.HasPrefix(key, "/") {
		t.Errorf("key must not start with slash: %q", key)
	}
	if !strings.HasPrefix(key, "projects/") {
		t.Errorf("expected path to start with projects/: %q", key)
	}
}

// TestKeyPathFunctionsNoDoubleSlash verifies all PITR key functions with various
// prefix forms never produce double slashes.
func TestKeyPathFunctionsNoDoubleSlash(t *testing.T) {
	ts := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	prefixes := []string{"", "pfx", "pfx/", "a/b/c", "a/b/c/"}

	for _, pfx := range prefixes {
		t.Run(fmt.Sprintf("prefix=%q", pfx), func(t *testing.T) {
			for _, key := range []string{
				BaseBackupKey(pfx, "p", "d", "0/1", ts),
				WALSegmentKey(pfx, "p", "d", 1, "seg1"),
				ManifestKey(pfx, "p", "d", ts),
			} {
				if strings.Contains(key, "//") {
					t.Errorf("double slash in key=%q (prefix=%q)", key, pfx)
				}
			}
		})
	}
}
