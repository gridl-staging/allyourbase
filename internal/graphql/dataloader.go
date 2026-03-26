// Package graphql Dataloader implements batch loading and row caching for GraphQL relationship field resolution. It manages loaders per relationship target and handles both batch and single-key queries.
package graphql

import (
	"context"
	"fmt"
	"strings"
	"sync"

	gql "github.com/graphql-go/graphql"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

type dataloaderContextKey struct{}

// Dataloader is a per-request loader registry keyed by target table/match column.
type Dataloader struct {
	pool    *pgxpool.Pool
	cache   *schema.SchemaCache
	loaders sync.Map
}

// Loader batches and caches rows by lookup key.
type Loader struct {
	keys     []interface{}
	keySet   map[string]struct{}
	results  map[interface{}][]map[string]any
	loaded   bool
	mu       sync.Mutex
	batchFn  func(ctx context.Context, keys []interface{}) (map[interface{}][]map[string]any, error)
	singleFn func(ctx context.Context, key interface{}) ([]map[string]any, error)
}

func NewDataloader(pool *pgxpool.Pool, cache *schema.SchemaCache) *Dataloader {
	return &Dataloader{
		pool:  pool,
		cache: cache,
	}
}

func ctxWithDataloader(ctx context.Context, dl *Dataloader) context.Context {
	if ctx == nil || dl == nil {
		return ctx
	}
	return context.WithValue(ctx, dataloaderContextKey{}, dl)
}

func dataloaderFromCtx(ctx context.Context) *Dataloader {
	if ctx == nil {
		return nil
	}
	dl, _ := ctx.Value(dataloaderContextKey{}).(*Dataloader)
	return dl
}

func newLoader(
	batchFn func(ctx context.Context, keys []interface{}) (map[interface{}][]map[string]any, error),
	singleFn func(ctx context.Context, key interface{}) ([]map[string]any, error),
) *Loader {
	return &Loader{
		keySet:   map[string]struct{}{},
		results:  map[interface{}][]map[string]any{},
		batchFn:  batchFn,
		singleFn: singleFn,
	}
}

func normalizeLoaderKey(v interface{}) string {
	return loaderSQLKey(v)
}

func loaderSQLKey(v interface{}) string {
	return fmt.Sprintf("%v", v)
}

func (l *Loader) Prime(key interface{}) {
	if l == nil || key == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	norm := normalizeLoaderKey(key)
	if _, exists := l.keySet[norm]; exists {
		return
	}
	l.keySet[norm] = struct{}{}
	l.keys = append(l.keys, key)
}

// Load returns rows for the given key, executing a batch query on first access and caching results for subsequent lookups.
func (l *Loader) Load(ctx context.Context, key interface{}) ([]map[string]any, error) {
	if l == nil {
		return nil, nil
	}
	if key == nil {
		return nil, nil
	}

	normKey := normalizeLoaderKey(key)
	for {
		l.mu.Lock()
		if rows, ok := l.results[normKey]; ok {
			l.mu.Unlock()
			return rows, nil
		}

		needBatch := !l.loaded
		if needBatch {
			keys := append([]interface{}{}, l.keys...)
			if _, exists := l.keySet[normKey]; !exists {
				l.keySet[normKey] = struct{}{}
				keys = append(keys, key)
			}
			l.mu.Unlock()

			batched, err := l.batchFn(ctx, keys)
			if err != nil {
				return nil, err
			}

			l.mu.Lock()
			for batchKey, rows := range batched {
				l.results[normalizeLoaderKey(batchKey)] = rows
			}
			l.loaded = true
			l.mu.Unlock()
			continue
		}
		l.mu.Unlock()

		rows, err := l.singleFn(ctx, key)
		if err != nil {
			return nil, err
		}
		l.mu.Lock()
		if existing, ok := l.results[normKey]; ok {
			l.mu.Unlock()
			return existing, nil
		}
		l.results[normKey] = rows
		l.mu.Unlock()
		return rows, nil
	}
}

func loaderRegistryKey(rel *schema.Relationship) string {
	if rel == nil {
		return ""
	}
	matchCol := ""
	if len(rel.ToColumns) > 0 {
		matchCol = rel.ToColumns[0]
	}
	return rel.ToSchema + "." + rel.ToTable + ":" + matchCol
}

// GetLoader returns a loader for the relationship, creating one if needed with batch and single-key query support.
func (d *Dataloader) GetLoader(rel *schema.Relationship) *Loader {
	if d == nil || rel == nil || len(rel.ToColumns) == 0 {
		return nil
	}

	key := loaderRegistryKey(rel)
	if cached, ok := d.loaders.Load(key); ok {
		return cached.(*Loader)
	}

	targetTbl := d.targetTable(rel)
	matchCol := rel.ToColumns[0]
	loader := newLoader(
		func(ctx context.Context, keys []interface{}) (map[interface{}][]map[string]any, error) {
			rowsByKey, err := d.fetchBatchRows(ctx, targetTbl, matchCol, keys)
			if err != nil {
				return nil, err
			}
			if targetTbl != nil {
				primeRowsForTableRelationships(d, targetTbl, flattenGroupedRows(rowsByKey))
			}
			return rowsByKey, nil
		},
		func(ctx context.Context, key interface{}) ([]map[string]any, error) {
			rows, err := d.fetchRowsForKey(ctx, targetTbl, matchCol, key)
			if err != nil {
				return nil, err
			}
			if targetTbl != nil {
				primeRowsForTableRelationships(d, targetTbl, rows)
			}
			return rows, nil
		},
	)

	actual, loaded := d.loaders.LoadOrStore(key, loader)
	if loaded {
		return actual.(*Loader)
	}
	return loader
}

func (d *Dataloader) targetTable(rel *schema.Relationship) *schema.Table {
	if d == nil || d.cache == nil || rel == nil {
		return nil
	}
	if tbl := d.cache.Tables[rel.ToSchema+"."+rel.ToTable]; tbl != nil {
		return tbl
	}
	return d.cache.TableByName(rel.ToTable)
}

// fetchBatchRows fetches rows from the database where the match column value matches any of the provided keys.
func (d *Dataloader) fetchBatchRows(
	ctx context.Context,
	tbl *schema.Table,
	matchCol string,
	keys []interface{},
) (map[interface{}][]map[string]any, error) {
	results := make(map[interface{}][]map[string]any)
	if len(keys) == 0 {
		return results, nil
	}
	if d.pool == nil {
		return nil, fmt.Errorf("database pool is nil")
	}
	if tbl == nil {
		return nil, fmt.Errorf("target table not found")
	}

	keyTexts := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == nil {
			continue
		}
		cacheKey := normalizeLoaderKey(key)
		keyTexts = append(keyTexts, loaderSQLKey(key))
		results[cacheKey] = []map[string]any{}
	}
	if len(keyTexts) == 0 {
		return results, nil
	}

	sql := fmt.Sprintf(
		`SELECT * FROM %s WHERE %s::text = ANY($1::text[])`,
		sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name),
		sqlutil.QuoteIdent(matchCol),
	)
	loaded, err := withRLSQueryRunner(ctx, d.pool, func(q queryRunner) (interface{}, error) {
		rows, _, queryErr := queryAndScanRows(ctx, q, sql, keyTexts)
		if queryErr != nil {
			return nil, fmt.Errorf("query dataloader batch: %w", queryErr)
		}
		return rows, nil
	})
	if err != nil {
		return nil, err
	}

	records := loaded.([]map[string]any)
	for _, row := range records {
		val, ok := row[matchCol]
		if !ok || val == nil {
			continue
		}
		norm := normalizeLoaderKey(val)
		results[norm] = append(results[norm], row)
	}
	return results, nil
}

// fetchRowsForKey fetches rows from the database where the match column equals the provided key.
func (d *Dataloader) fetchRowsForKey(
	ctx context.Context,
	tbl *schema.Table,
	matchCol string,
	key interface{},
) ([]map[string]any, error) {
	if key == nil {
		return nil, nil
	}
	if d.pool == nil {
		return nil, fmt.Errorf("database pool is nil")
	}
	if tbl == nil {
		return nil, fmt.Errorf("target table not found")
	}

	sql := fmt.Sprintf(
		`SELECT * FROM %s WHERE %s::text = $1`,
		sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name),
		sqlutil.QuoteIdent(matchCol),
	)
	loaded, err := withRLSQueryRunner(ctx, d.pool, func(q queryRunner) (interface{}, error) {
		rows, _, queryErr := queryAndScanRows(ctx, q, sql, loaderSQLKey(key))
		if queryErr != nil {
			return nil, fmt.Errorf("query dataloader key: %w", queryErr)
		}
		return rows, nil
	})
	if err != nil {
		return nil, err
	}
	return loaded.([]map[string]any), nil
}

func flattenGroupedRows(grouped map[interface{}][]map[string]any) []map[string]any {
	if len(grouped) == 0 {
		return nil
	}
	var rows []map[string]any
	for _, v := range grouped {
		rows = append(rows, v...)
	}
	return rows
}

// primeRowsForTableRelationships primes relationship loaders with keys extracted from the provided rows.
func primeRowsForTableRelationships(dl *Dataloader, tbl *schema.Table, rows []map[string]any) {
	if dl == nil || tbl == nil || len(rows) == 0 {
		return
	}
	for _, rel := range tbl.Relationships {
		if rel == nil || len(rel.FromColumns) == 0 || len(rel.ToColumns) == 0 {
			continue
		}
		loader := dl.GetLoader(rel)
		if loader == nil {
			continue
		}
		fromCol := rel.FromColumns[0]
		for _, row := range rows {
			key := row[fromCol]
			if key == nil {
				continue
			}
			loader.Prime(key)
		}
	}
}

// RelationshipResolverFactory creates relationship field resolvers per relationship.
type RelationshipResolverFactory func(fromTbl *schema.Table, rel *schema.Relationship) gql.FieldResolveFn

// relationshipResolverFactory returns a factory that creates GraphQL field resolvers for table relationships.
func relationshipResolverFactory(_ *pgxpool.Pool, _ *schema.SchemaCache) RelationshipResolverFactory {
	return func(_ *schema.Table, rel *schema.Relationship) gql.FieldResolveFn {
		relCopy := *rel
		return func(p gql.ResolveParams) (interface{}, error) {
			source, ok := p.Source.(map[string]any)
			if !ok || len(relCopy.FromColumns) == 0 {
				return nil, nil
			}

			key := source[relCopy.FromColumns[0]]
			if key == nil {
				if relCopy.Type == "one-to-many" {
					return []map[string]any{}, nil
				}
				return nil, nil
			}

			dl := dataloaderFromCtx(p.Context)
			if dl == nil {
				return nil, fmt.Errorf("dataloader not found in context")
			}
			loader := dl.GetLoader(&relCopy)
			if loader == nil {
				if relCopy.Type == "one-to-many" {
					return []map[string]any{}, nil
				}
				return nil, nil
			}

			rows, err := loader.Load(p.Context, key)
			if err != nil {
				return nil, err
			}

			switch strings.ToLower(relCopy.Type) {
			case "many-to-one":
				if len(rows) == 0 {
					return nil, nil
				}
				return rows[0], nil
			case "one-to-many":
				if rows == nil {
					return []map[string]any{}, nil
				}
				return rows, nil
			default:
				return nil, nil
			}
		}
	}
}
