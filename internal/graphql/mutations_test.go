package graphql

import (
	"testing"

	gql "github.com/graphql-go/graphql"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func unwrapInputType(in gql.Input) gql.Input {
	if nn, ok := in.(*gql.NonNull); ok {
		if typed, ok := nn.OfType.(gql.Input); ok {
			return typed
		}
	}
	return in
}

func asInputObject(t *testing.T, in gql.Input, msg string) *gql.InputObject {
	t.Helper()
	obj, ok := unwrapInputType(in).(*gql.InputObject)
	testutil.True(t, ok, msg)
	return obj
}

func mutationField(t *testing.T, s *gql.Schema, name string) *gql.FieldDefinition {
	t.Helper()
	mutation := s.MutationType()
	testutil.NotNil(t, mutation)
	field := mutation.Fields()[name]
	testutil.NotNil(t, field)
	return field
}

func TestBuildSchemaMutationFieldsSingleTable(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "serial", IsPrimaryKey: true, DefaultExpr: "nextval('posts_id_seq'::regclass)"},
			{Name: "title", TypeName: "text"},
			{Name: "views", TypeName: "integer"},
			{Name: "is_published", TypeName: "boolean", IsNullable: true},
			{Name: "created_at", TypeName: "timestamptz", DefaultExpr: "now()"},
		},
		PrimaryKey: []string{"id"},
		Indexes: []*schema.Index{
			{Name: "posts_pkey", IsPrimary: true, IsUnique: true},
			{Name: "posts_title_key", IsUnique: true},
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	testutil.NotNil(t, s.MutationType().Fields()["insert_posts"])
	testutil.NotNil(t, s.MutationType().Fields()["update_posts"])
	testutil.NotNil(t, s.MutationType().Fields()["delete_posts"])
}

func TestBuildSchemaInsertInputTypesAndRequiredness(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "serial", IsPrimaryKey: true, DefaultExpr: "nextval('posts_id_seq'::regclass)"},
			{Name: "title", TypeName: "text"},
			{Name: "views", TypeName: "integer"},
			{Name: "is_published", TypeName: "boolean", IsNullable: true},
			{Name: "created_at", TypeName: "timestamptz", DefaultExpr: "now()"},
		},
		PrimaryKey: []string{"id"},
		Indexes:    []*schema.Index{{Name: "posts_pkey", IsPrimary: true, IsUnique: true}},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	insert := mutationField(t, s, "insert_posts")
	objectsArg := findArg(insert, "objects")
	testutil.NotNil(t, objectsArg)

	objectsType, ok := objectsArg.Type.(*gql.NonNull)
	testutil.True(t, ok, "objects should be non-null")
	objectsList, ok := objectsType.OfType.(*gql.List)
	testutil.True(t, ok, "objects should be a list")
	objectsElem, ok := objectsList.OfType.(*gql.NonNull)
	testutil.True(t, ok, "objects list should contain non-null items")

	insertInput := asInputObject(t, objectsElem.OfType.(gql.Input), "insert input should be InputObject")
	fields := insertInput.Fields()

	_, idRequired := fields["id"].Type.(*gql.NonNull)
	testutil.False(t, idRequired, "id with default should be optional")
	_, titleRequired := fields["title"].Type.(*gql.NonNull)
	testutil.True(t, titleRequired, "title should be required")
	_, viewsRequired := fields["views"].Type.(*gql.NonNull)
	testutil.True(t, viewsRequired, "views should be required")
	_, publishedRequired := fields["is_published"].Type.(*gql.NonNull)
	testutil.False(t, publishedRequired, "nullable column should be optional")
	_, createdAtRequired := fields["created_at"].Type.(*gql.NonNull)
	testutil.False(t, createdAtRequired, "defaulted column should be optional")

	titleType := unwrapInputType(fields["title"].Type)
	viewsType := unwrapInputType(fields["views"].Type)
	publishedType := unwrapInputType(fields["is_published"].Type)
	createdAtType := unwrapInputType(fields["created_at"].Type)
	testutil.True(t, titleType == gql.String, "title should map to String")
	testutil.True(t, viewsType == gql.Int, "views should map to Int")
	testutil.True(t, publishedType == gql.Boolean, "is_published should map to Boolean")
	testutil.True(t, createdAtType == gql.DateTime, "created_at should map to DateTime")
}

func TestBuildSchemaSetInputAllOptional(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "title", TypeName: "text"},
			{Name: "is_published", TypeName: "boolean", IsNullable: true},
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	update := mutationField(t, s, "update_posts")
	setArg := findArg(update, "_set")
	testutil.NotNil(t, setArg)

	setInput := asInputObject(t, setArg.Type, "_set should be InputObject")
	for name, field := range setInput.Fields() {
		_, isNonNull := field.Type.(*gql.NonNull)
		testutil.False(t, isNonNull, "_set field %s should be optional", name)
	}
}

func TestBuildSchemaOnConflictInputAndConstraintEnum(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "title", TypeName: "text"},
		},
		Indexes: []*schema.Index{
			{Name: "posts_pkey", IsPrimary: true, IsUnique: true},
			{Name: "posts_title_key", IsUnique: true},
			{Name: "posts_title_idx", IsUnique: false},
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	insert := mutationField(t, s, "insert_posts")
	onConflictArg := findArg(insert, "on_conflict")
	testutil.NotNil(t, onConflictArg)
	onConflict := asInputObject(t, onConflictArg.Type, "on_conflict should be InputObject")

	constraint := onConflict.Fields()["constraint"]
	testutil.NotNil(t, constraint)
	constraintEnum, ok := constraint.Type.(*gql.Enum)
	testutil.True(t, ok, "constraint should be an enum")
	values := constraintEnum.Values()
	testutil.Equal(t, 2, len(values))

	enumNames := map[string]bool{}
	for _, v := range values {
		enumNames[v.Name] = true
	}
	testutil.True(t, enumNames["posts_pkey"])
	testutil.True(t, enumNames["posts_title_key"])

	updateColumns := onConflict.Fields()["update_columns"]
	testutil.NotNil(t, updateColumns)
	updateColumnsList, ok := updateColumns.Type.(*gql.List)
	testutil.True(t, ok, "update_columns should be list")
	updateColumnsElem, ok := updateColumnsList.OfType.(*gql.NonNull)
	testutil.True(t, ok, "update_columns elements should be non-null")
	testutil.True(t, updateColumnsElem.OfType == gql.String, "update_columns inner type should be String")
}

func TestBuildSchemaMutationResponseShape(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "title", TypeName: "text"},
		},
		Indexes: []*schema.Index{{Name: "posts_pkey", IsPrimary: true, IsUnique: true}},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	insert := mutationField(t, s, "insert_posts")
	response, ok := insert.Type.(*gql.Object)
	testutil.True(t, ok, "insert return type should be object")

	affectedRows := response.Fields()["affected_rows"]
	testutil.NotNil(t, affectedRows)
	affectedRowsNN, ok := affectedRows.Type.(*gql.NonNull)
	testutil.True(t, ok, "affected_rows should be non-null")
	testutil.True(t, affectedRowsNN.OfType == gql.Int, "affected_rows should be Int")

	returning := response.Fields()["returning"]
	testutil.NotNil(t, returning)
	returningList, ok := returning.Type.(*gql.List)
	testutil.True(t, ok, "returning should be a list")
	retObj, ok := returningList.OfType.(*gql.Object)
	testutil.True(t, ok, "returning should list objects")
	testutil.Equal(t, "posts", retObj.Name())
}

func TestBuildSchemaSkipsSystemAndReadOnlyTablesForMutations(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "_ayb_meta", Kind: "table", Columns: []*schema.Column{{Name: "id", TypeName: "integer"}}},
		{Schema: "public", Name: "posts", Kind: "table", Columns: []*schema.Column{{Name: "id", TypeName: "integer"}}},
		{Schema: "public", Name: "posts_view", Kind: "view", Columns: []*schema.Column{{Name: "id", TypeName: "integer"}}},
		{Schema: "public", Name: "posts_mv", Kind: "materialized_view", Columns: []*schema.Column{{Name: "id", TypeName: "integer"}}},
	})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)
	testutil.NotNil(t, s.MutationType())

	mutFields := s.MutationType().Fields()
	testutil.NotNil(t, mutFields["insert_posts"])
	testutil.Nil(t, mutFields["insert__ayb_meta"])
	testutil.Nil(t, mutFields["insert_posts_view"])
	testutil.Nil(t, mutFields["insert_posts_mv"])

	queryFields := s.QueryType().Fields()
	testutil.NotNil(t, queryFields["posts"])
	testutil.NotNil(t, queryFields["posts_view"])
	testutil.NotNil(t, queryFields["posts_mv"])
}

func TestBuildSchemaInsertWithoutUniqueIndexHasNoOnConflict(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "events",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "payload", TypeName: "jsonb", IsJSON: true},
		},
		Indexes: []*schema.Index{{Name: "events_payload_idx", IsUnique: false}},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	insert := mutationField(t, s, "insert_events")
	testutil.Nil(t, findArg(insert, "on_conflict"))
}

func TestBuildSchemaMutationResolverWiring(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "title", TypeName: "text"},
		},
	}})

	objectTypes := map[string]*gql.Object{}
	whereInputs := map[string]*gql.InputObject{}
	for _, tbl := range cache.TableList() {
		if skipTable(tbl) {
			continue
		}
		key := tableKey(tbl.Schema, tbl.Name)
		tblCopy := tbl
		objectTypes[key] = gql.NewObject(gql.ObjectConfig{
			Name: tblCopy.Name,
			Fields: gql.FieldsThunk(func() gql.Fields {
				return buildObjectFields(tblCopy, objectTypes, nil)
			}),
		})
		whereInputs[key] = buildWhereInput(tbl)
	}

	mutationFields := buildMutationFields(cache, objectTypes, whereInputs, func(tbl *schema.Table, op string) gql.FieldResolveFn {
		return func(p gql.ResolveParams) (interface{}, error) {
			return op + "_" + tbl.Name, nil
		}
	})
	insert := mutationFields["insert_posts"]
	update := mutationFields["update_posts"]
	deleteField := mutationFields["delete_posts"]
	testutil.NotNil(t, insert.Resolve)
	testutil.NotNil(t, update.Resolve)
	testutil.NotNil(t, deleteField.Resolve)

	mutationType := gql.NewObject(gql.ObjectConfig{Name: "Mutation", Fields: mutationFields})
	queryType := gql.NewObject(gql.ObjectConfig{
		Name:   "Query",
		Fields: gql.Fields{"_empty": &gql.Field{Type: gql.String}},
	})
	_, err := gql.NewSchema(gql.SchemaConfig{Query: queryType, Mutation: mutationType})
	testutil.NoError(t, err)
}

func TestBuildSchemaQueryFieldsUnaffectedByMutations(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "title", TypeName: "text"},
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	postsField := s.QueryType().Fields()["posts"]
	testutil.NotNil(t, postsField)
	testutil.NotNil(t, findArg(postsField, "where"))
	testutil.NotNil(t, findArg(postsField, "order_by"))
	testutil.NotNil(t, findArg(postsField, "limit"))
	testutil.NotNil(t, findArg(postsField, "offset"))
}

func TestBuildSchemaOnConflictConstraintEnumAcceptsInvalidIndexNames(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "title", TypeName: "text"},
		},
		Indexes: []*schema.Index{
			{Name: "posts-title.key", IsUnique: true},
			{Name: "posts title key", IsUnique: true},
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	insert := mutationField(t, s, "insert_posts")
	onConflictArg := findArg(insert, "on_conflict")
	testutil.NotNil(t, onConflictArg)
	onConflict := asInputObject(t, onConflictArg.Type, "on_conflict should be InputObject")
	constraint := onConflict.Fields()["constraint"]
	testutil.NotNil(t, constraint)
	constraintEnum, ok := constraint.Type.(*gql.Enum)
	testutil.True(t, ok, "constraint should be an enum")

	values := constraintEnum.Values()
	testutil.Equal(t, 2, len(values))

	valueSet := map[string]bool{}
	for _, v := range values {
		valueStr, ok := v.Value.(string)
		testutil.True(t, ok, "constraint enum values should store original index names")
		valueSet[valueStr] = true
	}
	testutil.True(t, valueSet["posts-title.key"])
	testutil.True(t, valueSet["posts title key"])
}

func TestBuildSchemaOnConflictConstraintEnumDedupesSanitizedNameCollisions(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "title", TypeName: "text"},
		},
		Indexes: []*schema.Index{
			{Name: "posts-title-key", IsUnique: true}, // -> posts_title_key
			{Name: "posts_title_key", IsUnique: true}, // -> posts_title_key (collision)
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	insert := mutationField(t, s, "insert_posts")
	onConflictArg := findArg(insert, "on_conflict")
	testutil.NotNil(t, onConflictArg)
	onConflict := asInputObject(t, onConflictArg.Type, "on_conflict should be InputObject")
	constraint := onConflict.Fields()["constraint"]
	testutil.NotNil(t, constraint)
	constraintEnum, ok := constraint.Type.(*gql.Enum)
	testutil.True(t, ok, "constraint should be an enum")

	values := constraintEnum.Values()
	testutil.Equal(t, 2, len(values))

	enumKeys := map[string]bool{}
	enumPayloads := map[string]bool{}
	for _, v := range values {
		enumKeys[v.Name] = true
		valueStr, ok := v.Value.(string)
		testutil.True(t, ok, "constraint enum values should store original index names")
		enumPayloads[valueStr] = true
	}
	testutil.True(t, enumKeys["posts_title_key"])
	testutil.True(t, enumKeys["posts_title_key_2"])
	testutil.True(t, enumPayloads["posts-title-key"])
	testutil.True(t, enumPayloads["posts_title_key"])
}
