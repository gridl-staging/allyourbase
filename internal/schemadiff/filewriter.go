package schemadiff

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// migrationFilePattern matches migration filenames like "0001_create_users.up.sql".
var migrationFilePattern = regexp.MustCompile(`^(\d+)_.*\.(up|down)\.sql$`)

// WriteMigration writes up and down SQL migration files to dir.
// It scans dir for the highest existing sequence number and writes
// "{seq+1}_{name}.up.sql" and "{seq+1}_{name}.down.sql".
// Returns the paths of the created files.
func WriteMigration(dir, name, upSQL, downSQL string) (upPath, downPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating migrations directory: %w", err)
	}

	seq, err := nextSequenceNumber(dir)
	if err != nil {
		return "", "", fmt.Errorf("scanning migrations directory: %w", err)
	}

	safeName := sanitizeName(name)
	prefix := fmt.Sprintf("%04d_%s", seq, safeName)

	upPath = filepath.Join(dir, prefix+".up.sql")
	downPath = filepath.Join(dir, prefix+".down.sql")

	if err := os.WriteFile(upPath, []byte(upSQL), 0o644); err != nil {
		return "", "", fmt.Errorf("writing up migration: %w", err)
	}
	if err := os.WriteFile(downPath, []byte(downSQL), 0o644); err != nil {
		// Clean up the up file on failure.
		_ = os.Remove(upPath)
		return "", "", fmt.Errorf("writing down migration: %w", err)
	}

	return upPath, downPath, nil
}

// nextSequenceNumber returns max(existing sequence numbers) + 1, starting at 1.
func nextSequenceNumber(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory might not exist yet — that is fine, we created it above.
		return 1, nil
	}

	var seqs []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := migrationFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err == nil {
			seqs = append(seqs, n)
		}
	}

	if len(seqs) == 0 {
		return 1, nil
	}
	sort.Ints(seqs)
	return seqs[len(seqs)-1] + 1, nil
}

// sanitizeName converts a migration name to a filesystem-safe string:
// lowercase, spaces to underscores, non-alphanumeric/underscore removed.
func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	var sb strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			sb.WriteRune(r)
		}
	}
	result := sb.String()
	if result == "" {
		return "migration"
	}
	return result
}
