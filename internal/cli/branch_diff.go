// Package cli Provides utilities for comparing database schemas between branches and formatting the differences for output.
package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/allyourbase/ayb/internal/schemadiff"
	"github.com/jackc/pgx/v5/pgxpool"
)

func buildSchemaSnapshot(ctx context.Context, pool *pgxpool.Pool) (*schemadiff.Snapshot, error) {
	return schemadiff.TakeSnapshot(ctx, pool)
}

func diffSnapshots(a, b *schemadiff.Snapshot) schemadiff.ChangeSet {
	return schemadiff.Diff(a, b)
}

func validateBranchDiffOutputFormat(format string) (string, error) {
	if format == "" {
		format = "table"
	}
	switch format {
	case "table", "json", "sql":
		return format, nil
	default:
		return "", fmt.Errorf("invalid output format %q, expected one of: table, json, sql", format)
	}
}

func printSQLChanges(w io.Writer, cs schemadiff.ChangeSet) error {
	sql := schemadiff.GenerateUp(cs)
	if sql == "" {
		fmt.Fprintln(w, "-- No schema differences found.")
		return nil
	}
	_, err := fmt.Fprintln(w, sql)
	return err
}

// printTableChanges writes a formatted table showing schema differences between two database branches to the provided writer.
func printTableChanges(w io.Writer, cs schemadiff.ChangeSet, branchA, branchB string) error {
	if len(cs) == 0 {
		fmt.Fprintf(w, "No schema differences between %q and %q.\n", branchA, branchB)
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Schema differences: %s → %s\n\n", branchA, branchB)
	fmt.Fprintln(tw, "TYPE\tTABLE\tDETAIL")
	for _, c := range cs {
		detail := changeDetail(c)
		table := c.TableName
		if c.SchemaName != "" && c.SchemaName != "public" {
			table = c.SchemaName + "." + c.TableName
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Type, table, detail)
	}
	return tw.Flush()
}
