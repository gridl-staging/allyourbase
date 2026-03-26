// Package graphql Builds GraphQL mutation types and resolvers for database tables. Constructs insert, update, and delete fields with proper input types and handles unique constraint enums for conflict resolution.
package graphql

import (
	"sort"
	"strconv"
	"strings"

	gql "github.com/graphql-go/graphql"

	"github.com/allyourbase/ayb/internal/schema"
)

// MutationResolverFactory creates a field resolver for a given table + mutation operation.
// When nil is passed, mutation resolvers remain nil (schema-only mode).
type MutationResolverFactory func(tbl *schema.Table, op string) gql.FieldResolveFn

// Builds GraphQL mutation fields for insert, update, and delete operations on each table. Each table generates insert_{table}, update_{table}, and delete_{table} fields configured with appropriate input types and optional resolvers.
func buildMutationFields(
	cache *schema.SchemaCache,
	objectTypes map[string]*gql.Object,
	whereInputs map[string]*gql.InputObject,
	resolverFactory ...MutationResolverFactory,
) gql.Fields {
	if cache == nil {
		return gql.Fields{}
	}

	var factory MutationResolverFactory
	if len(resolverFactory) > 0 {
		factory = resolverFactory[0]
	}

	fields := gql.Fields{}
	for _, tbl := range cache.TableList() {
		if skipMutationTable(tbl) {
			continue
		}

		key := tableKey(tbl.Schema, tbl.Name)
		rowType := objectTypes[key]
		whereInput := whereInputs[key]
		if rowType == nil || whereInput == nil {
			continue
		}

		insertInput := buildInsertInput(tbl)
		setInput := buildSetInput(tbl)
		onConflictInput := buildOnConflictInput(tbl)
		responseType := buildMutationResponse(tbl, rowType)

		insertField := &gql.Field{
			Type: responseType,
			Args: gql.FieldConfigArgument{
				"objects": &gql.ArgumentConfig{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(insertInput)))},
			},
		}
		if onConflictInput != nil {
			insertField.Args["on_conflict"] = &gql.ArgumentConfig{Type: onConflictInput}
		}
		if factory != nil {
			insertField.Resolve = factory(tbl, "insert")
		}
		fields["insert_"+tbl.Name] = insertField

		updateField := &gql.Field{
			Type: responseType,
			Args: gql.FieldConfigArgument{
				"where": &gql.ArgumentConfig{Type: gql.NewNonNull(whereInput)},
				"_set":  &gql.ArgumentConfig{Type: setInput},
				"_inc":  &gql.ArgumentConfig{Type: setInput},
				"_append": &gql.ArgumentConfig{
					Type: setInput,
				},
				"_prepend": &gql.ArgumentConfig{
					Type: setInput,
				},
			},
		}
		if factory != nil {
			updateField.Resolve = factory(tbl, "update")
		}
		fields["update_"+tbl.Name] = updateField

		deleteField := &gql.Field{
			Type: responseType,
			Args: gql.FieldConfigArgument{
				"where": &gql.ArgumentConfig{Type: gql.NewNonNull(whereInput)},
			},
		}
		if factory != nil {
			deleteField.Resolve = factory(tbl, "delete")
		}
		fields["delete_"+tbl.Name] = deleteField
	}

	return fields
}

func pgToGraphQLInput(col *schema.Column) gql.Input {
	return pgToGraphQL(col).(gql.Input)
}

func skipMutationTable(tbl *schema.Table) bool {
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

func buildInsertInput(tbl *schema.Table) *gql.InputObject {
	fields := gql.InputObjectConfigFieldMap{}
	for _, col := range tbl.Columns {
		fieldType := gql.Input(pgToGraphQLInput(col))
		if !isInsertOptional(col) {
			fieldType = gql.NewNonNull(fieldType)
		}
		fields[col.Name] = &gql.InputObjectFieldConfig{Type: fieldType}
	}

	return gql.NewInputObject(gql.InputObjectConfig{
		Name:   toPascal(tbl.Name) + "InsertInput",
		Fields: fields,
	})
}

func isInsertOptional(col *schema.Column) bool {
	if col == nil {
		return true
	}
	if col.IsNullable {
		return true
	}
	// Default expressions include server-generated values such as serial/identity/now().
	return strings.TrimSpace(col.DefaultExpr) != ""
}

func buildSetInput(tbl *schema.Table) *gql.InputObject {
	fields := gql.InputObjectConfigFieldMap{}
	for _, col := range tbl.Columns {
		fields[col.Name] = &gql.InputObjectFieldConfig{Type: pgToGraphQLInput(col)}
	}

	return gql.NewInputObject(gql.InputObjectConfig{
		Name:   toPascal(tbl.Name) + "SetInput",
		Fields: fields,
	})
}

func buildOnConflictInput(tbl *schema.Table) *gql.InputObject {
	constraintEnum := buildConstraintEnum(tbl)
	if constraintEnum == nil {
		return nil
	}

	return gql.NewInputObject(gql.InputObjectConfig{
		Name: toPascal(tbl.Name) + "OnConflictInput",
		Fields: gql.InputObjectConfigFieldMap{
			"constraint":     &gql.InputObjectFieldConfig{Type: constraintEnum},
			"update_columns": &gql.InputObjectFieldConfig{Type: gql.NewList(gql.NewNonNull(gql.String))},
		},
	})
}

// Builds a GraphQL enum representing unique constraints on a table, suitable for use in ON CONFLICT clauses. Returns nil if the table has no unique or primary constraints.
func buildConstraintEnum(tbl *schema.Table) *gql.Enum {
	names := mutationConstraintNames(tbl)
	if len(names) == 0 {
		return nil
	}

	values := gql.EnumValueConfigMap{}
	usedNames := map[string]struct{}{}
	for _, name := range names {
		enumName := uniqueConstraintEnumValueName(sanitizeTypeName(name), usedNames)
		values[enumName] = &gql.EnumValueConfig{Value: name}
	}
	return gql.NewEnum(gql.EnumConfig{
		Name:   toPascal(tbl.Name) + "ConstraintEnum",
		Values: values,
	})
}

// Generates a unique enum value name based on the given base, appending numeric suffixes to resolve collisions. Updates the used map to track generated names and returns a fallback name 'Constraint' if base is empty.
func uniqueConstraintEnumValueName(base string, used map[string]struct{}) string {
	if base == "" {
		base = "Constraint"
	}

	if used == nil {
		return base
	}

	if _, exists := used[base]; !exists {
		used[base] = struct{}{}
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := base + "_" + strconv.Itoa(suffix)
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

// Returns a sorted list of constraint names for a table, including primary key and unique index constraints. If a primary key exists but is not indexed, the conventional '{table}_pkey' name is included.
func mutationConstraintNames(tbl *schema.Table) []string {
	if tbl == nil {
		return nil
	}

	names := map[string]bool{}
	hasPrimaryConstraint := false
	for _, idx := range tbl.Indexes {
		if idx == nil || idx.Name == "" {
			continue
		}
		if idx.IsUnique || idx.IsPrimary {
			names[idx.Name] = true
		}
		if idx.IsPrimary {
			hasPrimaryConstraint = true
		}
	}
	if len(tbl.PrimaryKey) > 0 && !hasPrimaryConstraint {
		names[tbl.Name+"_pkey"] = true
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func buildMutationResponse(tbl *schema.Table, objectType *gql.Object) *gql.Object {
	return gql.NewObject(gql.ObjectConfig{
		Name: toPascal(tbl.Name) + "MutationResponse",
		Fields: gql.Fields{
			"affected_rows": &gql.Field{Type: gql.NewNonNull(gql.Int)},
			"returning":     &gql.Field{Type: gql.NewList(objectType)},
		},
	})
}
