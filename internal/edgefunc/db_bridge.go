package edgefunc

import (
	"context"
	"fmt"

	"github.com/dop251/goja"
)

// QueryExecutor executes database queries on behalf of edge functions.
type QueryExecutor interface {
	Execute(ctx context.Context, query Query) (QueryResult, error)
}

// SpatialQueryExecutor executes parameterized raw SQL for spatial helper functions.
type SpatialQueryExecutor interface {
	QueryRaw(ctx context.Context, sql string, args ...any) (QueryResult, error)
}

// Query represents a database query built by JS code via the ayb.db API.
type Query struct {
	Table   string
	Action  string // "select", "insert", "update", "delete"
	Columns string
	Filters []Filter
	Data    map[string]interface{}
}

// Filter is a single WHERE clause predicate.
type Filter struct {
	Column string
	Op     string // "eq", "neq", "gt", "lt", "gte", "lte"
	Value  interface{}
}

// QueryResult holds rows returned by a query.
type QueryResult struct {
	Rows []map[string]interface{}
}

// WithQueryExecutor sets the DB query executor for ayb.db support.
func WithQueryExecutor(qe QueryExecutor) GojaOption {
	return func(r *GojaRuntime) { r.queryExecutor = qe }
}

// registerDBBridge injects ayb.db.from(table) into the Goja VM.
// Returns nil immediately if no QueryExecutor is configured (ayb.db won't be available).
func registerDBBridge(vm *goja.Runtime, ctx context.Context, qe QueryExecutor) error {
	if qe == nil {
		return nil
	}

	db := vm.NewObject()
	_ = db.Set("from", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("ayb.db.from() requires a table name"))
		}
		table := call.Arguments[0].String()
		return newQueryBuilder(vm, ctx, qe, table)
	})

	ayb := vm.NewObject()
	_ = ayb.Set("db", db)
	return vm.Set("ayb", ayb)
}

// copyQuery returns a shallow copy of q with its own Filters slice,
// so each branch of a forked chain is independent.
func copyQuery(q Query) Query {
	cp := q
	cp.Filters = make([]Filter, len(q.Filters))
	copy(cp.Filters, q.Filters)
	if q.Data != nil {
		cp.Data = make(map[string]interface{}, len(q.Data))
		for k, v := range q.Data {
			cp.Data[k] = v
		}
	}
	return cp
}

// newQueryBuilder creates a chainable query builder JS object.
func newQueryBuilder(vm *goja.Runtime, ctx context.Context, qe QueryExecutor, table string) goja.Value {
	q := Query{Table: table}
	return buildChainObject(vm, ctx, qe, q)
}

// filterOps lists all supported filter operations for the query builder.
var filterOps = []string{"eq", "neq", "gt", "lt", "gte", "lte"}

// buildChainObject creates a JS object with chainable methods for the query builder.
// Supports: select, insert, update, delete, eq/neq/gt/lt/gte/lte, execute.
// Each method copies the query so forked chains are isolated.
func buildChainObject(vm *goja.Runtime, ctx context.Context, qe QueryExecutor, q Query) goja.Value {
	obj := vm.NewObject()

	_ = obj.Set("select", func(call goja.FunctionCall) goja.Value {
		next := copyQuery(q)
		next.Action = "select"
		if len(call.Arguments) > 0 {
			next.Columns = call.Arguments[0].String()
		}
		return buildChainObject(vm, ctx, qe, next)
	})

	_ = obj.Set("insert", func(call goja.FunctionCall) goja.Value {
		next := copyQuery(q)
		next.Action = "insert"
		if len(call.Arguments) > 0 {
			exported := call.Arguments[0].Export()
			if m, ok := exported.(map[string]interface{}); ok {
				next.Data = m
			}
		}
		return buildChainObject(vm, ctx, qe, next)
	})

	_ = obj.Set("update", func(call goja.FunctionCall) goja.Value {
		next := copyQuery(q)
		next.Action = "update"
		if len(call.Arguments) > 0 {
			exported := call.Arguments[0].Export()
			if m, ok := exported.(map[string]interface{}); ok {
				next.Data = m
			}
		}
		return buildChainObject(vm, ctx, qe, next)
	})

	_ = obj.Set("delete", func(call goja.FunctionCall) goja.Value {
		next := copyQuery(q)
		next.Action = "delete"
		return buildChainObject(vm, ctx, qe, next)
	})

	for _, op := range filterOps {
		op := op // capture loop variable
		_ = obj.Set(op, func(call goja.FunctionCall) goja.Value {
			next := copyQuery(q)
			if len(call.Arguments) >= 2 {
				next.Filters = append(next.Filters, Filter{
					Column: call.Arguments[0].String(),
					Op:     op,
					Value:  call.Arguments[1].Export(),
				})
			}
			return buildChainObject(vm, ctx, qe, next)
		})
	}

	_ = obj.Set("execute", func(call goja.FunctionCall) goja.Value {
		result, err := qe.Execute(ctx, q)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("ayb.db query failed: %w", err)))
		}
		return vm.ToValue(result.Rows)
	})

	return obj
}
