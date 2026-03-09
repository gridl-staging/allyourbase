package cli

import (
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/allyourbase/ayb/internal/scaffold"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Create a new AYB-backed project",
	Long: fmt.Sprintf(`Scaffold a new project with AYB configuration, schema, SDK client,
and context files for AI coding tools.

Available templates: %s

Examples:
  ayb init my-app                         # React (default)
  ayb init my-app --template next         # Next.js
  ayb init my-app --template express      # Express/Node backend
  ayb init my-app --template plain        # Minimal TypeScript`, strings.Join(templateNames(), ", ")),
	Args: cobra.ExactArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringP("template", "t", "react",
		fmt.Sprintf("Project template (%s)", strings.Join(templateNames(), ", ")))
}

func templateNames() []string {
	templates := scaffold.ValidTemplates()
	names := make([]string, len(templates))
	for i, t := range templates {
		names[i] = string(t)
	}
	return names
}

func runInit(cmd *cobra.Command, args []string) error {
	name := args[0]
	tmpl, _ := cmd.Flags().GetString("template")

	if !scaffold.IsValidTemplate(tmpl) {
		return fmt.Errorf("unknown template %q (available: %s)", tmpl, strings.Join(templateNames(), ", "))
	}

	useColor := colorEnabled()
	if useColor {
		fmt.Printf("%s Creating %s project: %s\n",
			ui.BrandEmoji,
			bold(tmpl, true),
			boldCyan(name, true))
	} else {
		fmt.Printf("%s Creating %s project: %s\n", ui.BrandEmoji, tmpl, name)
	}

	err := scaffold.Run(scaffold.Options{
		Name:     name,
		Template: scaffold.Template(tmpl),
	})
	if err != nil {
		return err
	}

	fmt.Println()
	if useColor {
		fmt.Printf("  %s %s\n\n", green(ui.SymbolCheck, true), bold("Project created!", true))
	} else {
		fmt.Printf("  %s Done!\n\n", ui.SymbolCheck)
	}
	fmt.Printf("  %s\n", dim("Next steps:", useColor))
	fmt.Printf("  cd %s\n", name)
	fmt.Printf("  ayb start\n")
	fmt.Printf("  ayb sql < schema.sql\n")
	fmt.Printf("  npm install\n")
	fmt.Printf("  npm run dev\n")

	return nil
}
