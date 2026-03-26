// Package graphql This file implements GraphQL mutation resolvers and SQL builders for insert, update, and delete operations, with support for conflict handling, update operators, and real-time event collection.
package graphql

import (
	"context"
	"fmt"

	gql "github.com/graphql-go/graphql"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/schema"
)

func mutationResolverFactory(pool *pgxpool.Pool) MutationResolverFactory {
	return func(tbl *schema.Table, op string) gql.FieldResolveFn {
		return func(p gql.ResolveParams) (interface{}, error) {
			return resolveMutation(p.Context, tbl, pool, op, p.Args)
		}
	}
}

func resolveMutation(ctx context.Context, tbl *schema.Table, pool *pgxpool.Pool, op string, args map[string]interface{}) (interface{}, error) {
	switch op {
	case "insert":
		return resolveInsertMutation(ctx, tbl, pool, args)
	case "update":
		return resolveUpdateMutation(ctx, tbl, pool, args)
	case "delete":
		return resolveDeleteMutation(ctx, tbl, pool, args)
	default:
		return nil, fmt.Errorf("unknown mutation operation: %s", op)
	}
}

// resolveInsertMutation executes an INSERT mutation supporting single and batch inserts with optional ON CONFLICT handling, collecting mutation events for created records.
func resolveInsertMutation(ctx context.Context, tbl *schema.Table, pool *pgxpool.Pool, args map[string]interface{}) (interface{}, error) {
	objects, err := parseMutationObjectsArg(args["objects"])
	if err != nil {
		return nil, err
	}
	var onConflict map[string]interface{}
	if v, ok := args["on_conflict"]; ok && v != nil {
		cast, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("on_conflict must be an object")
		}
		onConflict = cast
	}

	if len(objects) == 0 {
		return mutationResponse(0, []map[string]any{}), nil
	}

	result, err := withRLSQueryRunner(ctx, pool, func(q queryRunner) (interface{}, error) {
		sql, sqlArgs, buildErr := buildBatchInsertStatement(tbl, objects, onConflict)
		if buildErr != nil {
			return nil, buildErr
		}
		rows, affected, queryErr := queryAndScanRows(ctx, q, sql, sqlArgs...)
		if queryErr != nil {
			return nil, fmt.Errorf("query: %w", queryErr)
		}
		collectMutationEvents(ctx, tbl, "insert", rows)
		return mutationResponse(affected, rows), nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// resolveUpdateMutation executes an UPDATE mutation supporting _set, _inc, _append, and _prepend operators, fetching old rows before the update to preserve state for mutation events.
func resolveUpdateMutation(ctx context.Context, tbl *schema.Table, pool *pgxpool.Pool, args map[string]interface{}) (interface{}, error) {
	where, err := parseMutationObjectArg(args["where"], "where")
	if err != nil {
		return nil, err
	}
	set, err := parseOptionalMutationObjectArg(args["_set"], "_set")
	if err != nil {
		return nil, err
	}
	inc, err := parseOptionalMutationObjectArg(args["_inc"], "_inc")
	if err != nil {
		return nil, err
	}
	appendJ, err := parseOptionalMutationObjectArg(args["_append"], "_append")
	if err != nil {
		return nil, err
	}
	prependJ, err := parseOptionalMutationObjectArg(args["_prepend"], "_prepend")
	if err != nil {
		return nil, err
	}
	if err := validateUpdateOperators(set, inc, appendJ, prependJ); err != nil {
		return nil, err
	}

	sql, sqlArgs, err := buildUpdateStatement(tbl, where, set, inc, appendJ, prependJ)
	if err != nil {
		return nil, err
	}

	result, err := withRLSQueryRunner(ctx, pool, func(q queryRunner) (interface{}, error) {
		var oldRows []map[string]any
		if mutationEventCollectorFromContext(ctx) != nil {
			selectSQL, selectArgs, selectErr := buildSelectStatement(tbl, where)
			if selectErr != nil {
				return nil, selectErr
			}
			selectedRows, _, queryErr := queryAndScanRows(ctx, q, selectSQL, selectArgs...)
			if queryErr != nil {
				return nil, fmt.Errorf("query old rows: %w", queryErr)
			}
			oldRows = selectedRows
		}

		rows, affected, queryErr := queryAndScanRows(ctx, q, sql, sqlArgs...)
		if queryErr != nil {
			return nil, fmt.Errorf("query: %w", queryErr)
		}
		collectUpdateMutationEvents(ctx, tbl, rows, oldRows)
		return mutationResponse(affected, rows), nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// resolveDeleteMutation executes a DELETE mutation with a where clause, collecting mutation events for the deleted records.
func resolveDeleteMutation(ctx context.Context, tbl *schema.Table, pool *pgxpool.Pool, args map[string]interface{}) (interface{}, error) {
	where, err := parseMutationObjectArg(args["where"], "where")
	if err != nil {
		return nil, err
	}

	sql, sqlArgs, err := buildDeleteStatement(tbl, where)
	if err != nil {
		return nil, err
	}

	result, err := withRLSQueryRunner(ctx, pool, func(q queryRunner) (interface{}, error) {
		rows, affected, queryErr := queryAndScanRows(ctx, q, sql, sqlArgs...)
		if queryErr != nil {
			return nil, fmt.Errorf("query: %w", queryErr)
		}
		collectMutationEvents(ctx, tbl, "delete", rows)
		return mutationResponse(affected, rows), nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func parseMutationObjectArg(v interface{}, argName string) (map[string]interface{}, error) {
	if v == nil {
		return nil, fmt.Errorf("%s is required", argName)
	}
	cast, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be an object", argName)
	}
	return cast, nil
}

func parseOptionalMutationObjectArg(v interface{}, argName string) (map[string]interface{}, error) {
	if v == nil {
		return nil, nil
	}
	return parseMutationObjectArg(v, argName)
}

func validateUpdateOperators(set map[string]interface{}, inc map[string]interface{}, appendJ map[string]interface{}, prependJ map[string]interface{}) error {
	if len(set) == 0 && len(inc) == 0 && len(appendJ) == 0 && len(prependJ) == 0 {
		return fmt.Errorf("at least one of _set, _inc, _append, _prepend must be provided")
	}
	return validateOperatorColumnOverlap(set, inc, appendJ, prependJ)
}

// validateOperatorColumnOverlap returns an error if any column is targeted by more than one update operator (_set, _inc, _append, or _prepend) in a single mutation.
func validateOperatorColumnOverlap(set map[string]interface{}, inc map[string]interface{}, appendJ map[string]interface{}, prependJ map[string]interface{}) error {
	operators := []struct {
		name   string
		values map[string]interface{}
	}{
		{name: "_set", values: set},
		{name: "_inc", values: inc},
		{name: "_append", values: appendJ},
		{name: "_prepend", values: prependJ},
	}

	for i := 0; i < len(operators); i++ {
		if len(operators[i].values) == 0 {
			continue
		}
		for j := i + 1; j < len(operators); j++ {
			if len(operators[j].values) == 0 {
				continue
			}
			for col := range operators[i].values {
				if _, exists := operators[j].values[col]; exists {
					return fmt.Errorf("column '%s' appears in both %s and %s", col, operators[i].name, operators[j].name)
				}
			}
		}
	}

	return nil
}

// parseMutationObjectsArg converts a GraphQL argument into a slice of maps representing objects to be inserted, returning an error if the argument is missing, not a list, or contains non-object items.
func parseMutationObjectsArg(v interface{}) ([]map[string]interface{}, error) {
	if v == nil {
		return nil, fmt.Errorf("objects is required")
	}
	list, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("objects must be a list")
	}
	objects := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		cast, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("objects items must be objects")
		}
		objects = append(objects, cast)
	}
	return objects, nil
}

func mutationResponse(affected int64, rows []map[string]any) map[string]any {
	if rows == nil {
		rows = []map[string]any{}
	}
	return map[string]any{
		"affected_rows": int(affected),
		"returning":     rows,
	}
}
