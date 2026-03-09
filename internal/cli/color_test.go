package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// ansiAttr returns true if the string contains the given SGR attribute, either
// as a standalone sequence "\033[<n>m" or embedded in a combined sequence
// "\033[...;<n>m" / "\033[<n>;...m".
func ansiAttr(s, attr string) bool {
	return strings.Contains(s, "\033["+attr+"m") ||
		strings.Contains(s, "\033["+attr+";") ||
		strings.Contains(s, ";"+attr+"m") ||
		strings.Contains(s, ";"+attr+";")
}

// TestAnsiAttrHelper verifies all four match conditions of the ansiAttr helper.
func TestAnsiAttrHelper(t *testing.T) {
	tests := []struct {
		name string
		s    string
		attr string
		want bool
	}{
		// Condition 1: standalone "\033[<n>m"
		{"standalone bold", "\033[1m", "1", true},
		{"standalone dim", "\033[2mtest\033[0m", "2", true},
		{"standalone cyan", "\033[36m", "36", true},
		// Condition 2: attr at start of combined "\033[<n>;..."
		{"combined bold at start", "\033[1;36m", "1", true},
		// Condition 3: attr at end of combined ";...;<n>m"
		{"combined cyan at end", "\033[1;36m", "36", true},
		{"combined green at end", "\033[1;32m", "32", true},
		// Condition 4: attr in middle ";...;<n>;..."
		{"attr in middle", "\033[1;2;36m", "2", true},
		// No false positives: "1" should not match "31" (red)
		{"no false positive 1 vs 31", "\033[31m", "1", false},
		{"no false positive 2 vs 32", "\033[32m", "2", false},
		{"no false positive 3 vs 33", "\033[33m", "3", false},
		// Empty / unrelated strings
		{"no match", "plain text", "1", false},
		{"empty", "", "36", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ansiAttr(tt.s, tt.attr)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestStyledWithColor(t *testing.T) {
	result := styled("hello", "\033[1m", true)
	testutil.Contains(t, result, "hello")
	testutil.True(t, strings.HasPrefix(result, "\033[1m"), "expected output to start with bold code \\033[1m")
}

func TestStyledWithoutColor(t *testing.T) {
	result := styled("hello", "\033[1m", false)
	testutil.Equal(t, "hello", result)
}

func TestBold(t *testing.T) {
	r := bold("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, ansiAttr(r, "1"), "expected bold (SGR 1) in bold output")
	testutil.Equal(t, "test", bold("test", false))
}

func TestDim(t *testing.T) {
	r := dim("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, ansiAttr(r, "2"), "expected faint/dim (SGR 2) in dim output")
	testutil.Equal(t, "test", dim("test", false))
}

func TestCyan(t *testing.T) {
	r := cyan("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, ansiAttr(r, "36"), "expected cyan foreground (SGR 36) in cyan output")
	testutil.Equal(t, "test", cyan("test", false))
}

func TestGreen(t *testing.T) {
	r := green("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, ansiAttr(r, "32"), "expected green foreground (SGR 32) in green output")
	testutil.Equal(t, "test", green("test", false))
}

func TestBoldCyan(t *testing.T) {
	r := boldCyan("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, ansiAttr(r, "1"), "expected bold (SGR 1) in boldCyan output")
	testutil.True(t, ansiAttr(r, "36"), "expected cyan foreground (SGR 36) in boldCyan output")
	testutil.Equal(t, "test", boldCyan("test", false))
}

func TestBoldGreen(t *testing.T) {
	r := boldGreen("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, ansiAttr(r, "1"), "expected bold (SGR 1) in boldGreen output")
	testutil.True(t, ansiAttr(r, "32"), "expected green foreground (SGR 32) in boldGreen output")
	testutil.Equal(t, "test", boldGreen("test", false))
}

func TestStyledEmptyString(t *testing.T) {
	r := styled("", "\033[1m", true)
	testutil.True(t, strings.HasPrefix(r, "\033[1m"), "expected output to start with bold code \\033[1m")
	testutil.Equal(t, "", styled("", "\033[1m", false))
}

func TestColorEnabledRespectsNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	testutil.False(t, colorEnabled())
}

func TestColorEnabledFdRespectsNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	// NO_COLOR is set (even to empty string), so color should be disabled.
	testutil.False(t, colorEnabledFd(os.Stderr.Fd()))
}

func TestColorEnabledNO_COLORNotSet(t *testing.T) {
	// When NO_COLOR is not set, colorEnabled depends on terminal detection.
	// In tests, stderr is not a terminal, so both return false.
	// Use t.Setenv first to snapshot+restore, then unset for this test.
	t.Setenv("NO_COLOR", "placeholder")
	os.Unsetenv("NO_COLOR")
	testutil.False(t, colorEnabled())
	testutil.False(t, colorEnabledFd(os.Stderr.Fd()))
}

func TestYellow(t *testing.T) {
	r := yellow("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, ansiAttr(r, "33"), "expected yellow foreground (SGR 33) in yellow output")
	testutil.Equal(t, "test", yellow("test", false))
}
