// Package graphql Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/graphql/schema.go.
package graphql

import (
	"strings"
	"sync"

	gql "github.com/graphql-go/graphql"

	"github.com/allyourbase/ayb/internal/schema"
)

var buildSchemaMu sync.Mutex

// ResolverFactory creates a field resolver for a given table.
// When nil is passed to BuildSchemaWithFactories, resolvers remain nil (schema-only mode).
type ResolverFactory func(tbl *schema.Table, cache *schema.SchemaCache) gql.FieldResolveFn

// SubscriptionResolverFactory creates a subscription field resolver for a given table.
// When nil is passed, subscription resolvers remain nil (schema-only mode).
type SubscriptionResolverFactory func(tbl *schema.Table) gql.FieldResolveFn

func BuildSchemaWithFactories(
	cache *schema.SchemaCache,
	queryFactory ResolverFactory,
	mutationFactory MutationResolverFactory,
	relFactory ...RelationshipResolverFactory,
) (*gql.Schema, error) {
	var relationshipFactory RelationshipResolverFactory
	if len(relFactory) > 0 {
		relationshipFactory = relFactory[0]
	}
	return buildSchema(cache, queryFactory, mutationFactory, relationshipFactory, nil)
}

// constructs a complete GraphQL schema from the provided schema cache and resolver factories. It synchronizes access with a mutex and populates query, mutation, and subscription types with appropriate fields and resolvers.
func buildSchema(
	cache *schema.SchemaCache,
	queryFactory ResolverFactory,
	mutationFactory MutationResolverFactory,
	relFactory RelationshipResolverFactory,
	subscriptionFactory SubscriptionResolverFactory,
) (*gql.Schema, error) {
	buildSchemaMu.Lock()
	defer buildSchemaMu.Unlock()

	resetEnumTypeCacheUnsafe()
	cache = normalizeSchemaCache(cache)
	tables := filterBuildSchemaTables(cache.TableList())
	objectTypes := buildSchemaObjectTypes(tables, relFactory)
	whereInputs, orderByInputs := buildSchemaInputTypes(tables)
	queryFields := buildSchemaQueryFields(tables, cache, objectTypes, whereInputs, orderByInputs, queryFactory)
	mutationFields := buildMutationFields(cache, objectTypes, whereInputs, mutationFactory)
	subscriptionFields := buildSchemaSubscriptionFields(tables, objectTypes, whereInputs, subscriptionFactory)

	queryType := gql.NewObject(gql.ObjectConfig{Name: "Query", Fields: queryFields})
	cfg := gql.SchemaConfig{Query: queryType}
	if len(mutationFields) > 0 {
		cfg.Mutation = gql.NewObject(gql.ObjectConfig{Name: "Mutation", Fields: mutationFields})
	}
	if len(subscriptionFields) > 0 {
		cfg.Subscription = gql.NewObject(gql.ObjectConfig{Name: "Subscription", Fields: subscriptionFields})
	}

	schemaValue, err := gql.NewSchema(cfg)
	if err != nil {
		return nil, err
	}
	return &schemaValue, nil
}

func normalizeSchemaCache(cache *schema.SchemaCache) *schema.SchemaCache {
	if cache == nil {
		return &schema.SchemaCache{}
	}
	return cache
}

func filterBuildSchemaTables(tables []*schema.Table) []*schema.Table {
	filtered := make([]*schema.Table, 0, len(tables))
	for _, tbl := range tables {
		if skipTable(tbl) {
			continue
		}
		filtered = append(filtered, tbl)
	}
	return filtered
}

func buildSchemaObjectTypes(tables []*schema.Table, relFactory RelationshipResolverFactory) map[string]*gql.Object {
	objectTypes := make(map[string]*gql.Object, len(tables))
	for _, tbl := range tables {
		key := tableKey(tbl.Schema, tbl.Name)
		tblCopy := tbl
		objectTypes[key] = gql.NewObject(gql.ObjectConfig{
			Name: tblCopy.Name,
			Fields: gql.FieldsThunk(func() gql.Fields {
				return buildObjectFields(tblCopy, objectTypes, relFactory)
			}),
		})
	}
	return objectTypes
}

func buildSchemaInputTypes(tables []*schema.Table) (map[string]*gql.InputObject, map[string]*gql.InputObject) {
	whereInputs := make(map[string]*gql.InputObject, len(tables))
	orderByInputs := make(map[string]*gql.InputObject, len(tables))
	for _, tbl := range tables {
		key := tableKey(tbl.Schema, tbl.Name)
		whereInputs[key] = buildWhereInput(tbl)
		orderByInputs[key] = buildOrderByInput(tbl)
	}
	return whereInputs, orderByInputs
}

// TODO: Document buildSchemaQueryFields.
func buildSchemaQueryFields(
	tables []*schema.Table,
	cache *schema.SchemaCache,
	objectTypes map[string]*gql.Object,
	whereInputs map[string]*gql.InputObject,
	orderByInputs map[string]*gql.InputObject,
	queryFactory ResolverFactory,
) gql.Fields {
	queryFields := gql.Fields{}
	for _, tbl := range tables {
		key := tableKey(tbl.Schema, tbl.Name)
		field := &gql.Field{
			Type: gql.NewList(objectTypes[key]),
			Args: gql.FieldConfigArgument{
				"where":    &gql.ArgumentConfig{Type: whereInputs[key]},
				"order_by": &gql.ArgumentConfig{Type: orderByInputs[key]},
				"limit":    &gql.ArgumentConfig{Type: gql.Int},
				"offset":   &gql.ArgumentConfig{Type: gql.Int},
			},
		}
		if cache.HasPostGIS && tbl.HasGeometry() {
			field.Args["near"] = &gql.ArgumentConfig{Type: nearInput}
			field.Args["within"] = &gql.ArgumentConfig{Type: withinInput}
			field.Args["bbox"] = &gql.ArgumentConfig{Type: bboxInput}
		}
		if queryFactory != nil {
			field.Resolve = queryFactory(tbl, cache)
		}
		queryFields[tbl.Name] = field
	}
	if len(queryFields) == 0 {
		queryFields["_empty"] = &gql.Field{Type: gql.String, Resolve: func(p gql.ResolveParams) (interface{}, error) { return nil, nil }}
	}
	return queryFields
}

// TODO: Document buildSchemaSubscriptionFields.
func buildSchemaSubscriptionFields(
	tables []*schema.Table,
	objectTypes map[string]*gql.Object,
	whereInputs map[string]*gql.InputObject,
	subscriptionFactory SubscriptionResolverFactory,
) gql.Fields {
	subscriptionFields := gql.Fields{}
	for _, tbl := range tables {
		if skipSubscriptionTable(tbl) {
			continue
		}
		key := tableKey(tbl.Schema, tbl.Name)
		rowType := objectTypes[key]
		whereInput := whereInputs[key]
		if rowType == nil || whereInput == nil {
			continue
		}

		field := &gql.Field{
			Type: rowType,
			Args: gql.FieldConfigArgument{
				"where": &gql.ArgumentConfig{Type: whereInput},
			},
		}
		if subscriptionFactory != nil {
			field.Resolve = subscriptionFactory(tbl)
		}
		subscriptionFields[tbl.Name] = field
	}
	return subscriptionFields
}

// reports whether a table should be excluded from the GraphQL schema. Tables with names starting with _ayb_, partitioned tables, and unsupported kinds are skipped.
func skipTable(tbl *schema.Table) bool {
	if tbl == nil {
		return true
	}
	if strings.HasPrefix(tbl.Name, "_ayb_") {
		return true
	}
	switch tbl.Kind {
	case "", "table", "view", "materialized_view", "foreign_table":
		return false
	case "partitioned_table":
		return true
	default:
		return true
	}
}

func skipSubscriptionTable(tbl *schema.Table) bool {
	if skipTable(tbl) {
		return true
	}
	switch tbl.Kind {
	case "view", "materialized_view":
		return true
	default:
		return false
	}
}

// converts a table's columns and relationships into GraphQL fields. Columns are mapped to their corresponding GraphQL types with proper nullability, and relationships are represented as many-to-one or one-to-many fields.
func buildObjectFields(tbl *schema.Table, objectTypes map[string]*gql.Object, relFactory RelationshipResolverFactory) gql.Fields {
	fields := gql.Fields{}
	for _, col := range tbl.Columns {
		fieldType := pgToGraphQL(col)
		if !col.IsNullable {
			fieldType = gql.NewNonNull(fieldType)
		}
		fields[col.Name] = &gql.Field{Type: fieldType}
	}

	for _, rel := range tbl.Relationships {
		target := objectTypes[tableKey(rel.ToSchema, rel.ToTable)]
		if target == nil {
			continue
		}
		fieldName := rel.FieldName
		if fieldName == "" {
			continue
		}

		switch rel.Type {
		case "many-to-one":
			relType := gql.Output(target)
			if !isRelationshipNullable(tbl, rel) {
				relType = gql.NewNonNull(relType)
			}
			field := &gql.Field{Type: relType}
			if relFactory != nil {
				field.Resolve = relFactory(tbl, rel)
			}
			fields[fieldName] = field
		case "one-to-many":
			field := &gql.Field{Type: gql.NewList(target)}
			if relFactory != nil {
				field.Resolve = relFactory(tbl, rel)
			}
			fields[fieldName] = field
		}
	}

	return fields
}

func isRelationshipNullable(tbl *schema.Table, rel *schema.Relationship) bool {
	if rel == nil || rel.Type != "many-to-one" {
		return true
	}
	for _, colName := range rel.FromColumns {
		col := tbl.ColumnByName(colName)
		if col == nil || col.IsNullable {
			return true
		}
	}
	return false
}

func tableKey(schemaName, tableName string) string {
	return schemaName + "." + tableName
}
