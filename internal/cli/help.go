package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Command group IDs.
const (
	groupCore    = "core"
	groupData    = "data"
	groupAuth    = "auth"
	groupMigrate = "migrate"
	groupConfig  = "config"
)

// initHelp wires up styled help/usage rendering and command groups.
func initHelp() {
	// Register command groups.
	rootCmd.AddGroup(
		&cobra.Group{ID: groupCore, Title: "CORE"},
		&cobra.Group{ID: groupData, Title: "DATA & SCHEMA"},
		&cobra.Group{ID: groupAuth, Title: "AUTH & SECURITY"},
		&cobra.Group{ID: groupMigrate, Title: "MIGRATIONS"},
		&cobra.Group{ID: groupConfig, Title: "CONFIGURATION"},
	)

	// Assign groups.
	assign := map[string]string{
		"start":  groupCore,
		"stop":   groupCore,
		"status": groupCore,
		"demo":   groupCore,
		"logs":   groupCore,
		"stats":  groupCore,

		"sql":     groupData,
		"query":   groupData,
		"schema":  groupData,
		"rpc":     groupData,
		"db":      groupData,
		"types":   groupData,
		"storage": groupData,

		"admin":    groupAuth,
		"users":    groupAuth,
		"apikeys":  groupAuth,
		"secrets":  groupAuth,
		"webhooks": groupAuth,

		"migrate": groupMigrate,

		"config":    groupConfig,
		"init":      groupConfig,
		"mcp":       groupConfig,
		"version":   groupConfig,
		"uninstall": groupConfig,
	}
	for _, cmd := range rootCmd.Commands() {
		if gid, ok := assign[cmd.Name()]; ok {
			cmd.GroupID = gid
		}
	}

	rootCmd.SetHelpFunc(styledHelp)
	rootCmd.SetUsageFunc(styledUsage)
}

// styledHelp renders colorful help output.
func styledHelp(cmd *cobra.Command, _ []string) {
	c := colorEnabled()
	w := cmd.ErrOrStderr()

	// Description.
	if cmd.Long != "" {
		fmt.Fprintln(w)
		if cmd == rootCmd {
			fmt.Fprintf(w, "  %s %s\n", ui.BrandEmoji, boldCyan("Allyourbase", c))
			fmt.Fprintln(w)
			for _, line := range strings.Split(cmd.Long, "\n") {
				if strings.TrimSpace(line) == "" {
					fmt.Fprintln(w)
				} else if strings.HasPrefix(line, "  ") {
					// Example lines — show as code.
					fmt.Fprintf(w, "    %s\n", green(strings.TrimSpace(line), c))
				} else {
					fmt.Fprintf(w, "  %s\n", dim(line, c))
				}
			}
		} else {
			for _, line := range strings.Split(cmd.Long, "\n") {
				fmt.Fprintf(w, "  %s\n", line)
			}
		}
	} else if cmd.Short != "" {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s\n", cmd.Short)
	}

	fmt.Fprintln(w)

	// Usage.
	fmt.Fprintf(w, "%s\n", heading("USAGE", c))
	useLine := cmd.UseLine()
	if cmd.HasAvailableSubCommands() {
		useLine = cmd.CommandPath() + " [command]"
	}
	fmt.Fprintf(w, "  %s\n", useLine)
	fmt.Fprintln(w)

	// Examples.
	if cmd.Example != "" {
		fmt.Fprintf(w, "%s\n", heading("EXAMPLES", c))
		for _, line := range strings.Split(cmd.Example, "\n") {
			if strings.TrimSpace(line) == "" {
				fmt.Fprintln(w)
			} else {
				fmt.Fprintf(w, "  %s\n", green(strings.TrimSpace(line), c))
			}
		}
		fmt.Fprintln(w)
	}

	// Subcommands.
	printCommands(cmd, c)

	// Flags.
	printFlags(cmd, c)

	// Demo hints (root only).
	if cmd == rootCmd {
		fmt.Fprintf(w, "%s\n", heading("DEMOS", c))
		fmt.Fprintf(w, "  %s  %s\n", green("ayb demo kanban    ", c), dim("# Trello-lite kanban board  (port 5173)", c))
		fmt.Fprintf(w, "  %s  %s\n", green("ayb demo live-polls", c), dim("# real-time polling app     (port 5175)", c))
		fmt.Fprintln(w)
	}

	// Footer hint.
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(w, "%s\n",
			dim(fmt.Sprintf("Use \"%s [command] --help\" for more information about a command.", cmd.CommandPath()), c))
		fmt.Fprintln(w)
	}
}

// styledUsage renders just the usage section (shown on errors).
func styledUsage(cmd *cobra.Command) error {
	styledHelp(cmd, nil)
	return nil
}

// printCommands renders grouped or ungrouped subcommands.
func printCommands(cmd *cobra.Command, c bool) {
	if !cmd.HasAvailableSubCommands() {
		return
	}

	w := cmd.ErrOrStderr()
	groups := cmd.Groups()

	if len(groups) > 0 {
		// Grouped commands (root).
		grouped := make(map[string][]*cobra.Command)
		var ungrouped []*cobra.Command
		for _, sub := range cmd.Commands() {
			if !sub.IsAvailableCommand() {
				continue
			}
			if sub.GroupID != "" {
				grouped[sub.GroupID] = append(grouped[sub.GroupID], sub)
			} else {
				ungrouped = append(ungrouped, sub)
			}
		}
		for _, g := range groups {
			cmds := grouped[g.ID]
			if len(cmds) == 0 {
				continue
			}
			fmt.Fprintf(w, "%s\n", heading(g.Title, c))
			printCommandList(w, cmds, c)
			fmt.Fprintln(w)
		}
		if len(ungrouped) > 0 {
			fmt.Fprintf(w, "%s\n", heading("OTHER", c))
			printCommandList(w, ungrouped, c)
			fmt.Fprintln(w)
		}
	} else {
		// Ungrouped (subcommands like `ayb db`).
		var cmds []*cobra.Command
		for _, sub := range cmd.Commands() {
			if sub.IsAvailableCommand() {
				cmds = append(cmds, sub)
			}
		}
		if len(cmds) > 0 {
			fmt.Fprintf(w, "%s\n", heading("COMMANDS", c))
			printCommandList(w, cmds, c)
			fmt.Fprintln(w)
		}
	}
}

// printCommandList renders a list of commands with aligned descriptions.
func printCommandList(w io.Writer, cmds []*cobra.Command, c bool) {
	// Find max name length for alignment.
	maxLen := 0
	for _, cmd := range cmds {
		if n := len(cmd.Name()); n > maxLen {
			maxLen = n
		}
	}
	pad := maxLen + 4 // breathing room

	for _, cmd := range cmds {
		name := bold(fmt.Sprintf("%-*s", pad, cmd.Name()), c)
		fmt.Fprintf(w, "  %s%s\n", name, dim(cmd.Short, c))
	}
}

// printFlags renders local and inherited flags.
func printFlags(cmd *cobra.Command, c bool) {
	w := cmd.ErrOrStderr()

	if cmd == rootCmd {
		// Root: show all flags (persistent + local) together.
		all := cmd.Flags()
		if hasVisibleFlags(all) {
			fmt.Fprintf(w, "%s\n", heading("FLAGS", c))
			printFlagSet(w, all, c)
			fmt.Fprintln(w)
		}
		return
	}

	// Subcommands: local flags, then inherited (global) flags.
	local := cmd.LocalNonPersistentFlags()
	inherited := cmd.InheritedFlags()

	if hasVisibleFlags(local) {
		fmt.Fprintf(w, "%s\n", heading("FLAGS", c))
		printFlagSet(w, local, c)
		fmt.Fprintln(w)
	}

	if hasVisibleFlags(inherited) {
		fmt.Fprintf(w, "%s\n", heading("GLOBAL FLAGS", c))
		printFlagSet(w, inherited, c)
		fmt.Fprintln(w)
	}
}

// printFlagSet renders a pflag.FlagSet with colored flag names.
func printFlagSet(w io.Writer, fs *pflag.FlagSet, c bool) {
	// Use cobra's built-in FlagUsages for correct alignment, then colorize.
	usage := fs.FlagUsages()
	for _, line := range strings.Split(strings.TrimRight(usage, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Flag lines look like: "      --flag-name type   description"
		// Colorize the flag name portion (everything up to the description).
		fmt.Fprintf(w, "%s\n", colorizeFlag(line, c))
	}
}

// colorizeFlag applies color to a single flag usage line.
func colorizeFlag(line string, c bool) string {
	if !c {
		return line
	}
	// Find where the description starts after the flag+type.
	// pflag.FlagUsages pads to a fixed column. We colorize the flag part cyan
	// and dim the description.
	trimmed := strings.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)
	prefix := strings.Repeat(" ", indent)

	// Split at double-space boundary (pflag uses 3+ spaces between flag and desc).
	parts := splitFlagLine(trimmed)
	if len(parts) == 2 {
		return prefix + cyan(parts[0], c) + "   " + dim(parts[1], c)
	}
	return prefix + cyan(trimmed, c)
}

// splitFlagLine splits a flag usage line into [flagPart, descPart].
func splitFlagLine(s string) []string {
	// Look for 3+ spaces which pflag uses as separator.
	for i := 0; i < len(s)-2; i++ {
		if s[i] == ' ' && s[i+1] == ' ' && s[i+2] == ' ' {
			desc := strings.TrimLeft(s[i:], " ")
			if desc != "" {
				return []string{strings.TrimRight(s[:i], " "), desc}
			}
		}
	}
	return []string{s}
}

// hasVisibleFlags returns true if the flag set has any non-hidden flags.
func hasVisibleFlags(fs *pflag.FlagSet) bool {
	visible := false
	fs.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			visible = true
		}
	})
	return visible
}

// heading renders a section heading.
func heading(title string, c bool) string {
	return boldCyan(title, c)
}
