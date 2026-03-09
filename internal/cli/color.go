package cli

import (
	"github.com/allyourbase/ayb/internal/cli/ui"
)

// colorEnabled returns true if stderr is a terminal and color should be used.
// Respects the NO_COLOR environment variable (https://no-color.org/).
func colorEnabled() bool {
	return ui.ColorEnabled()
}

// colorEnabledFd returns true if the given file descriptor supports color.
// Respects the NO_COLOR environment variable (https://no-color.org/).
func colorEnabledFd(fd uintptr) bool {
	return ui.ColorEnabledFd(fd)
}

// styled wraps text with ANSI codes if color is enabled.
// Returns plain text if color is false.
// Retained for backward compatibility; new code should use ui.Style* directly.
func styled(text, code string, color bool) string {
	if !color {
		return text
	}
	return code + text + "\033[0m"
}

// The functions below use a forced-ANSI renderer so they always produce escape
// codes when color=true, even in non-TTY environments (the caller already
// made the TTY decision via the color bool parameter).

// bold returns text in bold if color is enabled.
func bold(text string, color bool) string {
	if !color {
		return text
	}
	return ui.ForcedRenderer().NewStyle().Bold(true).Render(text)
}

// dim returns text in dim if color is enabled.
func dim(text string, color bool) string {
	if !color {
		return text
	}
	return ui.ForcedRenderer().NewStyle().Faint(true).Render(text)
}

// cyan returns text in cyan if color is enabled.
func cyan(text string, color bool) string {
	if !color {
		return text
	}
	return ui.ForcedRenderer().NewStyle().Foreground(ui.ColorCyan).Render(text)
}

// green returns text in green if color is enabled.
func green(text string, color bool) string {
	if !color {
		return text
	}
	return ui.ForcedRenderer().NewStyle().Foreground(ui.ColorGreen).Render(text)
}

// yellow returns text in yellow if color is enabled.
func yellow(text string, color bool) string {
	if !color {
		return text
	}
	return ui.ForcedRenderer().NewStyle().Foreground(ui.ColorYellow).Render(text)
}

// boldCyan returns text in bold cyan if color is enabled.
func boldCyan(text string, color bool) string {
	if !color {
		return text
	}
	return ui.ForcedRenderer().NewStyle().Bold(true).Foreground(ui.ColorCyan).Render(text)
}

// boldGreen returns text in bold green if color is enabled.
func boldGreen(text string, color bool) string {
	if !color {
		return text
	}
	return ui.ForcedRenderer().NewStyle().Bold(true).Foreground(ui.ColorGreen).Render(text)
}
