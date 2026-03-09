package ui

import (
	"fmt"
	"strings"
)

// FormatError returns a styled error message with optional fix suggestions.
// When color is disabled, plain text is returned.
func FormatError(msg string, suggestions ...string) string {
	var b strings.Builder

	prefix := StyleBoldRed.Render("Error:")
	b.WriteString(fmt.Sprintf("%s %s\n", prefix, msg))

	if len(suggestions) > 0 {
		b.WriteString("\n")
		b.WriteString(StyleHint.Render("  Try:") + "\n")
		for _, s := range suggestions {
			b.WriteString(fmt.Sprintf("    %s %s\n", StyleHint.Render(SymbolArrow), s))
		}
	}

	return b.String()
}
