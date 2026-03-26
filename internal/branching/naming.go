package branching

import (
	"fmt"
	"regexp"
	"strings"
)

// maxBranchNameLen is the maximum allowed branch name length.
// PostgreSQL limits identifiers to 63 bytes; we match that.
const maxBranchNameLen = 63

// branchNameRe allows lowercase alphanumeric with hyphens/underscores,
// but not at the start or end.
var branchNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*[a-z0-9]$`)

// reservedNames cannot be used as branch names.
var reservedNames = map[string]bool{
	"main":      true,
	"master":    true,
	"default":   true,
	"postgres":  true,
	"template0": true,
	"template1": true,
}

// protectedDatabases cannot be dropped.
var protectedDatabases = map[string]bool{
	"postgres":  true,
	"template0": true,
	"template1": true,
}

// ValidateBranchName checks that name is a valid branch name.
func ValidateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("branch name must not be empty")
	}
	if len(name) > maxBranchNameLen {
		return fmt.Errorf("branch name must be at most %d characters", maxBranchNameLen)
	}
	if reservedNames[name] {
		return fmt.Errorf("branch name %q is reserved", name)
	}
	// Single-char names are valid if alphanumeric
	if len(name) == 1 {
		if name[0] >= 'a' && name[0] <= 'z' || name[0] >= '0' && name[0] <= '9' {
			return nil
		}
		return fmt.Errorf("branch name must contain only lowercase letters, numbers, hyphens, and underscores")
	}
	if !branchNameRe.MatchString(name) {
		return fmt.Errorf("branch name must contain only lowercase letters, numbers, hyphens, and underscores, and must not start or end with a hyphen or underscore")
	}
	return nil
}

// BranchDBName returns the PostgreSQL database name for a branch.
// Hyphens are replaced with underscores for PG identifier compatibility.
func BranchDBName(name string) string {
	return "ayb_branch_" + strings.ReplaceAll(name, "-", "_")
}

// IsProtectedDatabase returns true if the database name must not be dropped.
func IsProtectedDatabase(name string) bool {
	return protectedDatabases[name]
}
