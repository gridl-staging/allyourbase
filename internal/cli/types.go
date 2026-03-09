package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/typegen"
	"github.com/spf13/cobra"
)

var typesCmd = &cobra.Command{
	Use:   "types",
	Short: "Generate typed interfaces from database schema",
	Long: `Generate typed interfaces by introspecting a running PostgreSQL database.

Supported formats:
  typescript    Generate TypeScript interfaces (.d.ts)

Example:
  ayb types typescript --database-url postgresql://user:pass@localhost:5432/mydb
  ayb types typescript --database-url postgresql://... -o src/types/ayb.d.ts`,
}

var typesTypeScriptCmd = &cobra.Command{
	Use:   "typescript",
	Short: "Generate TypeScript interfaces from database schema",
	Long: `Connect to PostgreSQL, introspect the schema, and emit TypeScript
interfaces for every user table. System tables (_ayb_*) are excluded.

Output includes:
  - An interface for each table (e.g., export interface Posts { ... })
  - A Create type that omits auto-generated columns (PK, defaults)
  - An Update type (Partial<Create>)
  - Enum union types for PostgreSQL enums`,
	RunE: runTypesTypeScript,
}

func init() {
	typesCmd.AddCommand(typesTypeScriptCmd)
	typesTypeScriptCmd.Flags().String("database-url", "", "PostgreSQL connection URL (required)")
	typesTypeScriptCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
}

func runTypesTypeScript(cmd *cobra.Command, args []string) error {
	dbURL, _ := cmd.Flags().GetString("database-url")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		// Auto-discover: use config's explicit URL, or derive embedded Postgres URL
		// if the server is running (PID file exists).
		cfg, err := config.Load("", nil)
		if err == nil {
			if cfg.Database.URL != "" {
				dbURL = cfg.Database.URL
			} else if _, _, pidErr := readAYBPID(); pidErr == nil {
				dbURL = fmt.Sprintf("postgresql://ayb:ayb@127.0.0.1:%d/ayb?sslmode=disable", cfg.Database.EmbeddedPort)
			}
		}
	}
	if dbURL == "" {
		return fmt.Errorf("--database-url is required (or set DATABASE_URL)")
	}

	output, _ := cmd.Flags().GetString("output")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger, _, _, closeLog := newLogger("error", "json")
	defer closeLog()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:      dbURL,
		MaxConns: 2,
		MinConns: 1,
	}, logger)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	sc, err := schema.BuildCache(ctx, pool.DB())
	if err != nil {
		return fmt.Errorf("introspecting schema: %w", err)
	}

	result := typegen.TypeScript(sc)

	if output == "" {
		fmt.Print(result)
		return nil
	}

	if err := os.WriteFile(output, []byte(result), 0644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Wrote %d bytes to %s\n", len(result), output)
	return nil
}
