// Package cli implements branch management commands for creating, listing, deleting, and comparing database branches. Branches are isolated PostgreSQL databases that share the same schema and data at the point of creation.
package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"

	"github.com/allyourbase/ayb/internal/branching"
	"github.com/spf13/cobra"
)

var branchCmd = &cobra.Command{
	Use:   "branch",
	Short: "Database branch management commands",
	Long: `Create, list, and delete database branches. Branches are isolated copies of your
database that share the same schema and data at the point of creation.

Examples:
  ayb branch create feature-auth
  ayb branch list
  ayb branch delete feature-auth --yes
  ayb branch diff feature-auth staging`,
}

var branchCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new database branch",
	Long: `Create a new database branch by cloning the source database via pg_dump/psql.
The branch is an isolated PostgreSQL database named ayb_branch_{name}.

Examples:
  ayb branch create feature-auth
  ayb branch create feature-auth --from postgres://localhost/other_db`,
	Args: cobra.ExactArgs(1),
	RunE: runBranchCreate,
}

var branchListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all database branches",
	Long: `List all database branches with their status and metadata.

Examples:
  ayb branch list
  ayb branch list --output json`,
	RunE: runBranchList,
}

var branchDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a database branch",
	Long: `Delete a database branch. Terminates active connections, drops the branch
database, and removes metadata.

Requires --yes to confirm, or --force for failed/orphan cleanup.

Examples:
  ayb branch delete feature-auth --yes
  ayb branch delete broken-branch --force`,
	Args: cobra.ExactArgs(1),
	RunE: runBranchDelete,
}

var branchDiffCmd = &cobra.Command{
	Use:   "diff <branchA> <branchB>",
	Short: "Show schema differences between two branches",
	Long: `Compare schemas of two database branches using the schema diff engine.

Examples:
  ayb branch diff feature-auth staging
  ayb branch diff feature-auth staging --output json
  ayb branch diff feature-auth staging --output sql`,
	Args: cobra.ExactArgs(2),
	RunE: runBranchDiff,
}

func init() {
	branchCreateCmd.Flags().String("database-url", "", "Source database URL (overrides config)")
	branchCreateCmd.Flags().String("config", "", "Path to ayb.toml config file")
	branchCreateCmd.Flags().String("from", "", "Source database URL to branch from (defaults to configured database)")

	branchListCmd.Flags().String("database-url", "", "Database URL (overrides config)")
	branchListCmd.Flags().String("config", "", "Path to ayb.toml config file")

	branchDeleteCmd.Flags().String("database-url", "", "Database URL (overrides config)")
	branchDeleteCmd.Flags().String("config", "", "Path to ayb.toml config file")
	branchDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	branchDeleteCmd.Flags().Bool("force", false, "Force delete (for failed/orphan cleanup)")

	branchDiffCmd.Flags().String("database-url", "", "Database URL (overrides config)")
	branchDiffCmd.Flags().String("config", "", "Path to ayb.toml config file")

	branchCmd.AddCommand(branchCreateCmd)
	branchCmd.AddCommand(branchListCmd)
	branchCmd.AddCommand(branchDeleteCmd)
	branchCmd.AddCommand(branchDiffCmd)
	rootCmd.AddCommand(branchCmd)
}

// runBranchCreate creates a new database branch by cloning the source database via pg_dump/psql. The branch is created as an isolated PostgreSQL database named ayb_branch_{name}, and the operation returns the branch record in JSON or human-readable text format.
func runBranchCreate(cmd *cobra.Command, args []string) error {
	branchName := args[0]

	if err := branching.ValidateBranchName(branchName); err != nil {
		return fmt.Errorf("invalid branch name: %w", err)
	}

	// Resolve source database URL.
	sourceURL, _ := cmd.Flags().GetString("from")
	if sourceURL == "" {
		var err error
		sourceURL, err = resolveDBURL(cmd)
		if err != nil {
			return err
		}
	}

	ctx := cmd.Context()
	pool, err := openPool(ctx, sourceURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	repo := branching.NewPgRepo(pool)
	mgr := branching.NewManager(pool, repo, logger, branching.ManagerConfig{})

	rec, err := mgr.Create(ctx, branchName, sourceURL)
	if err != nil {
		return err
	}

	if outputFormat(cmd) == "json" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(rec)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Branch %q created (database: %s)\n", rec.Name, rec.BranchDatabase)
	return nil
}

// runBranchList lists all database branches with their status, source database, branch database name, and creation time. Results are formatted as a table or JSON based on the output flag.
func runBranchList(cmd *cobra.Command, _ []string) error {
	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pool, err := openPool(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	repo := branching.NewPgRepo(pool)
	mgr := branching.NewManager(pool, repo, logger, branching.ManagerConfig{})

	branches, err := mgr.List(ctx)
	if err != nil {
		return err
	}

	if outputFormat(cmd) == "json" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(branches)
	}

	if len(branches) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No branches found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tSOURCE\tDATABASE\tCREATED")
	for _, b := range branches {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			b.Name, b.Status, b.SourceDatabase, b.BranchDatabase,
			b.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	return w.Flush()
}

// runBranchDelete deletes a database branch, terminating active connections and removing the isolated database and metadata. Requires either the --yes flag for confirmation or --force to clean up failed or orphaned branches.
func runBranchDelete(cmd *cobra.Command, args []string) error {
	branchName := args[0]

	yes, _ := cmd.Flags().GetBool("yes")
	force, _ := cmd.Flags().GetBool("force")

	if !yes && !force {
		return fmt.Errorf("confirmation required: use --yes to confirm deletion of branch %q", branchName)
	}

	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pool, err := openPool(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	repo := branching.NewPgRepo(pool)
	mgr := branching.NewManager(pool, repo, logger, branching.ManagerConfig{})

	if err := mgr.Delete(ctx, branchName); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Branch %q deleted.\n", branchName)
	return nil
}

// runBranchDiff compares the schemas of two database branches, showing differences in tables, columns, constraints, and other schema elements. The diff output can be formatted as JSON, SQL, or a table.
func runBranchDiff(cmd *cobra.Command, args []string) error {
	branchA := args[0]
	branchB := args[1]
	output, err := validateBranchDiffOutputFormat(outputFormat(cmd))
	if err != nil {
		return err
	}

	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	pool, err := openPool(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	repo := branching.NewPgRepo(pool)
	mgr := branching.NewManager(pool, repo, logger, branching.ManagerConfig{})

	// Resolve branch database URLs.
	recA, err := mgr.Get(ctx, branchA)
	if err != nil {
		return fmt.Errorf("looking up branch %q: %w", branchA, err)
	}
	if recA == nil {
		return fmt.Errorf("branch %q not found", branchA)
	}

	recB, err := mgr.Get(ctx, branchB)
	if err != nil {
		return fmt.Errorf("looking up branch %q: %w", branchB, err)
	}
	if recB == nil {
		return fmt.Errorf("branch %q not found", branchB)
	}

	urlA, err := branching.ReplaceDatabaseInURL(dbURL, recA.BranchDatabase)
	if err != nil {
		return fmt.Errorf("building URL for branch %q: %w", branchA, err)
	}
	urlB, err := branching.ReplaceDatabaseInURL(dbURL, recB.BranchDatabase)
	if err != nil {
		return fmt.Errorf("building URL for branch %q: %w", branchB, err)
	}

	// Take snapshots of both branches.
	poolA, err := openPool(ctx, urlA)
	if err != nil {
		return fmt.Errorf("connecting to branch %q: %w", branchA, err)
	}
	defer poolA.Close()

	poolB, err := openPool(ctx, urlB)
	if err != nil {
		return fmt.Errorf("connecting to branch %q: %w", branchB, err)
	}
	defer poolB.Close()

	// Use schema introspection + diff.
	schemaA, err := buildSchemaSnapshot(ctx, poolA)
	if err != nil {
		return fmt.Errorf("introspecting branch %q: %w", branchA, err)
	}

	schemaB, err := buildSchemaSnapshot(ctx, poolB)
	if err != nil {
		return fmt.Errorf("introspecting branch %q: %w", branchB, err)
	}

	changes := diffSnapshots(schemaA, schemaB)

	switch output {
	case "json":
		return json.NewEncoder(cmd.OutOrStdout()).Encode(changes)
	case "sql":
		return printSQLChanges(cmd.OutOrStdout(), changes)
	default:
		return printTableChanges(cmd.OutOrStdout(), changes, branchA, branchB)
	}
}
