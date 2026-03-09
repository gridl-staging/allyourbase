// Package ui provides the Allyourbase CLI design system: styles, colors,
// symbols, and terminal-aware writers. All CLI visual output should use
// these definitions for consistency.
package ui

import (
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
)

// Brand

// BrandEmoji is the official Allyourbase brand logo.
const BrandEmoji = "\U0001F47E" // 👾

// Colors — ANSI 4-bit for maximum terminal compatibility.
// lipgloss/termenv handles degradation automatically.
var (
	ColorCyan    = lipgloss.Color("6")
	ColorGreen   = lipgloss.Color("2")
	ColorYellow  = lipgloss.Color("3")
	ColorRed     = lipgloss.Color("1")
	ColorMagenta = lipgloss.Color("5")
)

// Semantic styles — the design system.
var (
	StyleBold      = lipgloss.NewStyle().Bold(true)
	StyleDim       = lipgloss.NewStyle().Faint(true)
	StyleCyan      = lipgloss.NewStyle().Foreground(ColorCyan)
	StyleGreen     = lipgloss.NewStyle().Foreground(ColorGreen)
	StyleYellow    = lipgloss.NewStyle().Foreground(ColorYellow)
	StyleRed       = lipgloss.NewStyle().Foreground(ColorRed)
	StyleBoldCyan  = lipgloss.NewStyle().Bold(true).Foreground(ColorCyan)
	StyleBoldGreen = lipgloss.NewStyle().Bold(true).Foreground(ColorGreen)
	StyleBoldRed   = lipgloss.NewStyle().Bold(true).Foreground(ColorRed)

	// Status
	StyleSuccess = lipgloss.NewStyle().Foreground(ColorGreen)
	StyleWarning = lipgloss.NewStyle().Foreground(ColorYellow)
	StyleError   = lipgloss.NewStyle().Foreground(ColorRed)

	// Banner
	StyleBrandHeader = lipgloss.NewStyle().Bold(true).Foreground(ColorCyan)
	StyleLabel       = lipgloss.NewStyle().Bold(true).Width(10)

	// Hints and code
	StyleCode = lipgloss.NewStyle().Foreground(ColorGreen)
	StyleHint = lipgloss.NewStyle().Faint(true)
)

// Unicode status symbols — reliable across modern terminals.
const (
	SymbolCheck   = "✓"
	SymbolCross   = "✗"
	SymbolWarning = "⚠"
	SymbolDot     = "●"
	SymbolArrow   = "→"
)

// Forced-ANSI renderer — used by backward-compatible color helpers (bold, cyan, etc.)
// where the caller already decided color=true. The default renderer auto-detects
// the terminal and strips ANSI in non-TTY (e.g., tests), but these helpers need
// to unconditionally produce escape codes when asked.
var (
	forcedRenderer     *lipgloss.Renderer
	forcedRendererOnce sync.Once
)

// ForcedRenderer returns a lipgloss renderer that always produces ANSI output,
// regardless of terminal detection. Use this when the caller has already
// determined that color is appropriate (e.g., the `color bool` parameter is true).
func ForcedRenderer() *lipgloss.Renderer {
	forcedRendererOnce.Do(func() {
		forcedRenderer = lipgloss.NewRenderer(os.Stderr)
		forcedRenderer.SetColorProfile(termenv.ANSI)
	})
	return forcedRenderer
}

// StderrIsTTY returns whether stderr is connected to a terminal.
// Unlike ColorEnabled, this does not respect NO_COLOR — use it for
// deciding whether to animate spinners (which don't use color).
func StderrIsTTY() bool {
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}

// ColorEnabled returns whether stderr is a TTY that supports color.
// Respects NO_COLOR (https://no-color.org/).
func ColorEnabled() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}

// ColorEnabledFd returns whether the given fd supports color.
func ColorEnabledFd(fd uintptr) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
