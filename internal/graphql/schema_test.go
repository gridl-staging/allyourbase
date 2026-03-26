package graphql

import (
	"testing"

	gql "github.com/graphql-go/graphql"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func testCache(tables []*schema.Table) *schema.SchemaCache {
	tblMap := make(map[string]*schema.Table, len(tables))
	for _, t := range tables {
		tblMap[t.Schema+"."+t.Name] = t
	}
	return &schema.SchemaCache{Tables: tblMap, Schemas: []string{"public"}}
}

func objectTypeForQueryField(t *testing.T, s *gql.Schema, fieldName string) *gql.Object {
	t.Helper()
	f := s.QueryType().Fields()[fieldName]
	testutil.NotNil(t, f)
	listType, ok := f.Type.(*gql.List)
	testutil.True(t, ok, "query field should be list")
	obj, ok := listType.OfType.(*gql.Object)
	testutil.True(t, ok, "query field list should contain object")
	return obj
}

func unwrapOutputType(out gql.Output) gql.Output {
	if nn, ok := out.(*gql.NonNull); ok {
		return nn.OfType
	}
	return out
}

func findArg(field *gql.FieldDefinition, name string) *gql.Argument {
	if field == nil {
		return nil
	}
	for _, arg := range field.Args {
		if arg.Name() == name || arg.PrivateName == name {
			return arg
		}
	}
	return nil
}

func TestFilterBuildSchemaTablesSkipsUnsupportedCandidates(t *testing.T) {
	t.Parallel()

	tables := []*schema.Table{
		{Schema: "public", Name: "_ayb_internal", Kind: "table"},
		{Schema: "public", Name: "partitions", Kind: "partitioned_table"},
		{Schema: "public", Name: "posts", Kind: "table"},
		{Schema: "public", Name: "post_view", Kind: "view"},
		nil,
	}

	filtered := filterBuildSchemaTables(tables)
	testutil.SliceLen(t, filtered, 2)
	testutil.Equal(t, "posts", filtered[0].Name)
	testutil.Equal(t, "post_view", filtered[1].Name)
}

func TestBuildSchemaSingleTable(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", TypeName: "text"},
			{Name: "body", TypeName: "text", IsNullable: true},
			{Name: "created_at", TypeName: "timestamptz"},
		},
		PrimaryKey: []string{"id"},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)
	testutil.NotNil(t, s)

	postsObj := objectTypeForQueryField(t, s, "posts")
	testutil.Equal(t, "posts", postsObj.Name())
	fields := postsObj.Fields()
	testutil.Equal(t, 4, len(fields))

	_, idIsNonNull := fields["id"].Type.(*gql.NonNull)
	testutil.True(t, idIsNonNull, "id should be NonNull")
	_, titleIsNonNull := fields["title"].Type.(*gql.NonNull)
	testutil.True(t, titleIsNonNull, "title should be NonNull")
	_, bodyIsNonNull := fields["body"].Type.(*gql.NonNull)
	testutil.False(t, bodyIsNonNull, "body should be nullable")
}

func TestBuildSchemaEnumColumn(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "users",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "status", TypeName: "user_status", IsEnum: true, EnumValues: []string{"active", "inactive", "suspended"}},
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	usersObj := objectTypeForQueryField(t, s, "users")
	statusField := usersObj.Fields()["status"]
	testutil.NotNil(t, statusField)
	enumType, ok := unwrapOutputType(statusField.Type).(*gql.Enum)
	testutil.True(t, ok, "status should be enum")
	testutil.Equal(t, 3, len(enumType.Values()))
}

func TestBuildSchemaJSONColumn(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "events",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "metadata", TypeName: "jsonb", IsJSON: true, IsNullable: true},
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	eventsObj := objectTypeForQueryField(t, s, "events")
	metaField := eventsObj.Fields()["metadata"]
	testutil.NotNil(t, metaField)
	testutil.True(t, unwrapOutputType(metaField.Type) == JSONScalar, "metadata should use JSON scalar")
}

func TestBuildSchemaArrayColumn(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "articles",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "tags", TypeName: "text[]", IsArray: true},
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	articlesObj := objectTypeForQueryField(t, s, "articles")
	tagsField := articlesObj.Fields()["tags"]
	testutil.NotNil(t, tagsField)
	listType, ok := unwrapOutputType(tagsField.Type).(*gql.List)
	testutil.True(t, ok, "tags should be list")
	testutil.True(t, listType.OfType == gql.String, "tags inner should be string")
}

func TestBuildSchemaIncludesForeignTables(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "external_events",
		Kind:   "foreign_table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "payload", TypeName: "text"},
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)
	testutil.NotNil(t, s)

	obj := objectTypeForQueryField(t, s, "external_events")
	testutil.Equal(t, "external_events", obj.Name())
}

func TestBuildSchemaForeignKeyRelationship(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "users",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
				{Name: "name", TypeName: "text"},
			},
			Relationships: []*schema.Relationship{{
				Type:        "one-to-many",
				FromSchema:  "public",
				FromTable:   "users",
				FromColumns: []string{"id"},
				ToSchema:    "public",
				ToTable:     "posts",
				ToColumns:   []string{"author_id"},
				FieldName:   "posts",
			}},
		},
		{
			Schema: "public",
			Name:   "posts",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
				{Name: "author_id", TypeName: "integer"},
			},
			Relationships: []*schema.Relationship{{
				Type:        "many-to-one",
				FromSchema:  "public",
				FromTable:   "posts",
				FromColumns: []string{"author_id"},
				ToSchema:    "public",
				ToTable:     "users",
				ToColumns:   []string{"id"},
				FieldName:   "author",
			}},
		},
	})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	postsObj := objectTypeForQueryField(t, s, "posts")
	authorField := postsObj.Fields()["author"]
	testutil.NotNil(t, authorField)
	_, authorOK := unwrapOutputType(authorField.Type).(*gql.Object)
	testutil.True(t, authorOK, "posts.author should be users object")

	usersObj := objectTypeForQueryField(t, s, "users")
	postsField := usersObj.Fields()["posts"]
	testutil.NotNil(t, postsField)
	listType, ok := unwrapOutputType(postsField.Type).(*gql.List)
	testutil.True(t, ok, "users.posts should be list")
	_, postsOK := listType.OfType.(*gql.Object)
	testutil.True(t, postsOK, "users.posts list should contain post object")
}

func TestBuildSchemaNullableFKRelationshipField(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "categories", Kind: "table", Columns: []*schema.Column{{Name: "id", TypeName: "integer"}}},
		{
			Schema: "public",
			Name:   "posts",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
				{Name: "category_id", TypeName: "integer", IsNullable: true},
			},
			Relationships: []*schema.Relationship{{
				Type:        "many-to-one",
				FromSchema:  "public",
				FromTable:   "posts",
				FromColumns: []string{"category_id"},
				ToSchema:    "public",
				ToTable:     "categories",
				ToColumns:   []string{"id"},
				FieldName:   "category",
			}},
		},
	})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	postsObj := objectTypeForQueryField(t, s, "posts")
	catField := postsObj.Fields()["category"]
	testutil.NotNil(t, catField)
	_, isNonNull := catField.Type.(*gql.NonNull)
	testutil.False(t, isNonNull, "nullable FK relationship should be nullable")
}

func TestBuildSchemaExcludesSystemTable(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "_ayb_migrations", Kind: "table", Columns: []*schema.Column{{Name: "id", TypeName: "integer"}}},
		{Schema: "public", Name: "posts", Kind: "table", Columns: []*schema.Column{{Name: "id", TypeName: "integer"}}},
	})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	fields := s.QueryType().Fields()
	testutil.NotNil(t, fields["posts"])
	testutil.Nil(t, fields["_ayb_migrations"])
}

func TestBuildSchemaAllNullableColumnsRemainNullable(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "metrics_view",
		Kind:   "view",
		Columns: []*schema.Column{
			{Name: "count", TypeName: "integer", IsNullable: true},
			{Name: "label", TypeName: "text", IsNullable: true},
		},
	}})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	obj := objectTypeForQueryField(t, s, "metrics_view")
	for name, field := range obj.Fields() {
		_, isNonNull := field.Type.(*gql.NonNull)
		testutil.False(t, isNonNull, "field %s should be nullable", name)
	}
}

func TestBuildSchemaQueryArgs(t *testing.T) {
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

func TestBuildSchemaSpatialArgsPresentOnlyWhenPostGISAndSpatialColumns(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "places",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "name", TypeName: "text"},
			{Name: "location", TypeName: "geometry", IsGeometry: true, SRID: 4326, GeometryType: "Point"},
		},
	}})
	cache.HasPostGIS = true

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	field := s.QueryType().Fields()["places"]
	testutil.NotNil(t, field)
	testutil.NotNil(t, findArg(field, "near"))
	testutil.NotNil(t, findArg(field, "within"))
	testutil.NotNil(t, findArg(field, "bbox"))
}

func TestBuildSchemaSpatialArgsAbsentWithoutPostGIS(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "places",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "location", TypeName: "geometry", IsGeometry: true},
		},
	}})
	cache.HasPostGIS = false

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	field := s.QueryType().Fields()["places"]
	testutil.NotNil(t, field)
	testutil.Nil(t, findArg(field, "near"))
	testutil.Nil(t, findArg(field, "within"))
	testutil.Nil(t, findArg(field, "bbox"))
}

func TestBuildSchemaSpatialArgsAbsentWithoutSpatialColumns(t *testing.T) {
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
	cache.HasPostGIS = true

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	field := s.QueryType().Fields()["posts"]
	testutil.NotNil(t, field)
	testutil.Nil(t, findArg(field, "near"))
	testutil.Nil(t, findArg(field, "within"))
	testutil.Nil(t, findArg(field, "bbox"))
}

func TestBuildSchemaSpatialInputShapes(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "places",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "location", TypeName: "geometry", IsGeometry: true, GeometryType: "Point"},
		},
	}})
	cache.HasPostGIS = true

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	field := s.QueryType().Fields()["places"]
	testutil.NotNil(t, field)

	nearArg := findArg(field, "near")
	nearType, ok := nearArg.Type.(*gql.InputObject)
	testutil.True(t, ok, "near should be input object")
	nearFields := nearType.Fields()
	testutil.NotNil(t, nearFields["column"])
	testutil.NotNil(t, nearFields["longitude"])
	testutil.NotNil(t, nearFields["latitude"])
	testutil.NotNil(t, nearFields["distance"])
	testutil.True(t, unwrapInputType(nearFields["column"].Type) == gql.String, "near.column should be String!")
	testutil.True(t, unwrapInputType(nearFields["longitude"].Type) == gql.Float, "near.longitude should be Float!")
	testutil.True(t, unwrapInputType(nearFields["latitude"].Type) == gql.Float, "near.latitude should be Float!")
	testutil.True(t, unwrapInputType(nearFields["distance"].Type) == gql.Float, "near.distance should be Float!")

	withinArg := findArg(field, "within")
	withinType, ok := withinArg.Type.(*gql.InputObject)
	testutil.True(t, ok, "within should be input object")
	withinFields := withinType.Fields()
	testutil.NotNil(t, withinFields["column"])
	testutil.NotNil(t, withinFields["geojson"])
	testutil.True(t, unwrapInputType(withinFields["column"].Type) == gql.String, "within.column should be String!")
	testutil.True(t, unwrapInputType(withinFields["geojson"].Type) == GeoJSONScalar, "within.geojson should use GeoJSONScalar!")

	bboxArg := findArg(field, "bbox")
	bboxType, ok := bboxArg.Type.(*gql.InputObject)
	testutil.True(t, ok, "bbox should be input object")
	bboxFields := bboxType.Fields()
	for _, name := range []string{"column", "minLng", "minLat", "maxLng", "maxLat"} {
		testutil.NotNil(t, bboxFields[name])
	}
}

func TestBuildSchemaWithResolverFactory(t *testing.T) {
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

	called := false
	factory := ResolverFactory(func(tbl *schema.Table, _ *schema.SchemaCache) gql.FieldResolveFn {
		return func(p gql.ResolveParams) (interface{}, error) {
			called = true
			return []map[string]any{{"id": 1, "title": "test"}}, nil
		}
	})

	s, err := BuildSchemaWithFactories(cache, factory, nil, nil)
	testutil.NoError(t, err)
	testutil.NotNil(t, s)

	// Verify resolver is wired by executing a query
	result := gql.Do(gql.Params{
		Schema:        *s,
		RequestString: `{ posts { id title } }`,
	})
	testutil.Equal(t, 0, len(result.Errors))
	testutil.True(t, called, "resolver factory should have been called")
}

func TestBuildSchemaWithoutResolverFactory(t *testing.T) {
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

	// No factory — Stage 3 behavior preserved
	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)
	testutil.NotNil(t, s)

	// Query should succeed with nil data (no resolver)
	result := gql.Do(gql.Params{
		Schema:        *s,
		RequestString: `{ posts { id title } }`,
	})
	testutil.Equal(t, 0, len(result.Errors))
}

func TestBuildSchemaIntrospection(t *testing.T) {
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

	result := gql.Do(gql.Params{
		Schema:        *s,
		RequestString: `{ __schema { queryType { name } } }`,
	})
	testutil.NotNil(t, result)
	testutil.Equal(t, 0, len(result.Errors))
	testutil.NotNil(t, result.Data)
}

func TestBuildSchemaEnumCacheDoesNotLeakAcrossBuilds(t *testing.T) {
	t.Parallel()
	cacheOne := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "users",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "status", TypeName: "user_status", IsEnum: true, EnumValues: []string{"active", "inactive"}},
		},
	}})

	cacheTwo := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "users",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "status", TypeName: "user_status", IsEnum: true, EnumValues: []string{"active", "inactive", "blocked"}},
		},
	}})

	sOne, err := BuildSchemaWithFactories(cacheOne, nil, nil, nil)
	testutil.NoError(t, err)
	testutil.NotNil(t, sOne)

	sTwo, err := BuildSchemaWithFactories(cacheTwo, nil, nil, nil)
	testutil.NoError(t, err)

	usersObj := objectTypeForQueryField(t, sTwo, "users")
	statusField := usersObj.Fields()["status"]
	testutil.NotNil(t, statusField)
	enumType, ok := unwrapOutputType(statusField.Type).(*gql.Enum)
	testutil.True(t, ok, "status should be enum")
	testutil.Equal(t, 3, len(enumType.Values()))
}

func TestBuildSchemaGeneratesSubscriptionFields(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "posts",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
				{Name: "title", TypeName: "text"},
			},
		},
		{
			Schema: "public",
			Name:   "authors",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
				{Name: "name", TypeName: "text"},
			},
		},
	})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)
	testutil.NotNil(t, s.SubscriptionType())
	testutil.NotNil(t, s.SubscriptionType().Fields()["posts"])
	testutil.NotNil(t, s.SubscriptionType().Fields()["authors"])
}

func TestBuildSchemaSubscriptionFieldShape(t *testing.T) {
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

	subField := s.SubscriptionType().Fields()["posts"]
	testutil.NotNil(t, subField)

	_, isList := subField.Type.(*gql.List)
	testutil.False(t, isList, "subscription field should not be a list")
	_, isObj := unwrapOutputType(subField.Type).(*gql.Object)
	testutil.True(t, isObj, "subscription field should be an object")

	testutil.NotNil(t, findArg(subField, "where"))
	testutil.Nil(t, findArg(subField, "order_by"))
	testutil.Nil(t, findArg(subField, "limit"))
	testutil.Nil(t, findArg(subField, "offset"))
}

func TestBuildSchemaSubscriptionSkipsSystemAndViews(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "_ayb_migrations",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
			},
		},
		{
			Schema: "public",
			Name:   "post_view",
			Kind:   "view",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
			},
		},
		{
			Schema: "public",
			Name:   "post_mv",
			Kind:   "materialized_view",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
			},
		},
		{
			Schema: "public",
			Name:   "posts",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
			},
		},
	})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	testutil.NotNil(t, s.SubscriptionType().Fields()["posts"])
	testutil.Nil(t, s.SubscriptionType().Fields()["_ayb_migrations"])
	testutil.Nil(t, s.SubscriptionType().Fields()["post_view"])
	testutil.Nil(t, s.SubscriptionType().Fields()["post_mv"])
}

func TestBuildSchemaSubscriptionNotAddedWhenNoSubscribableTables(t *testing.T) {
	t.Parallel()
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "_ayb_hidden",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
			},
		},
		{
			Schema: "public",
			Name:   "only_view",
			Kind:   "view",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer"},
			},
		},
	})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)
	testutil.Nil(t, s.SubscriptionType())
}

func TestBuildSchemaSubscriptionWhereTypeMatchesQuery(t *testing.T) {
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

	queryField := s.QueryType().Fields()["posts"]
	subField := s.SubscriptionType().Fields()["posts"]
	testutil.NotNil(t, queryField)
	testutil.NotNil(t, subField)

	queryWhere := findArg(queryField, "where")
	subWhere := findArg(subField, "where")
	testutil.NotNil(t, queryWhere)
	testutil.NotNil(t, subWhere)
	testutil.Equal(t, queryWhere.Type, subWhere.Type)
}

func TestBuildSchemaIntrospectionIncludesSubscriptionType(t *testing.T) {
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

	result := gql.Do(gql.Params{
		Schema:        *s,
		RequestString: `{ __schema { subscriptionType { name } } }`,
	})
	testutil.Equal(t, 0, len(result.Errors))

	data, ok := result.Data.(map[string]interface{})
	testutil.True(t, ok, "expected map result data")
	schemaData, ok := data["__schema"].(map[string]interface{})
	testutil.True(t, ok, "expected __schema object")
	subType, ok := schemaData["subscriptionType"].(map[string]interface{})
	testutil.True(t, ok, "expected subscriptionType object")
	testutil.Equal(t, "Subscription", subType["name"])
}
