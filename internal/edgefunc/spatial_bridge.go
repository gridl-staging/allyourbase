// Package edgefunc Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/edgefunc/spatial_bridge.go.
package edgefunc

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dop251/goja"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/spatial"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

type spatialCallOptions struct {
	Limit  int
	Offset int
}

type spatialFilterExecutor struct {
	VM            *goja.Runtime
	Context       context.Context
	QueryExecutor SpatialQueryExecutor
}

type nearCallParams struct {
	TableName  string
	ColumnName string
	Longitude  float64
	Latitude   float64
	Distance   float64
	Options    spatialCallOptions
}

type withinCallParams struct {
	TableName  string
	ColumnName string
	GeoJSON    string
	Options    spatialCallOptions
}

type bboxCallParams struct {
	TableName  string
	ColumnName string
	MinLng     float64
	MinLat     float64
	MaxLng     float64
	MaxLat     float64
	Options    spatialCallOptions
}

// setupSpatial registers ayb.spatial helpers on the Goja runtime.
func setupSpatial(vm *goja.Runtime, sqe SpatialQueryExecutor, cacheGetter func() *schema.SchemaCache) error {
	return setupSpatialWithContext(vm, context.Background(), sqe, cacheGetter)
}

// TODO: Document setupSpatialWithContext.
func setupSpatialWithContext(
	vm *goja.Runtime,
	ctx context.Context,
	sqe SpatialQueryExecutor,
	cacheGetter func() *schema.SchemaCache,
) error {
	if sqe == nil || cacheGetter == nil {
		return nil
	}

	cache := cacheGetter()
	if cache == nil || !cache.HasPostGIS {
		return nil
	}

	aybValue := vm.Get("ayb")
	if goja.IsUndefined(aybValue) || goja.IsNull(aybValue) {
		return nil
	}
	aybObj := aybValue.ToObject(vm)
	filterExecutor := spatialFilterExecutor{
		VM:            vm,
		Context:       ctx,
		QueryExecutor: sqe,
	}

	spatialObj := vm.NewObject()
	registerSpatialNearMethod(vm, spatialObj, cache, filterExecutor)
	registerSpatialWithinMethod(vm, spatialObj, cache, filterExecutor)
	registerSpatialBBoxMethod(vm, spatialObj, cache, filterExecutor)
	registerSpatialInfoMethod(vm, spatialObj, cacheGetter)
	return aybObj.Set("spatial", spatialObj)
}

// TODO: Document registerSpatialNearMethod.
func registerSpatialNearMethod(
	vm *goja.Runtime,
	spatialObj *goja.Object,
	cache *schema.SchemaCache,
	filterExecutor spatialFilterExecutor,
) {
	_ = spatialObj.Set("near", func(call goja.FunctionCall) goja.Value {
		params, err := parseNearCallParams(vm, call)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		table, column, err := resolveSpatialTableAndColumn(cache, params.TableName, params.ColumnName)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		filter := spatial.NearFilter{
			Column:    column,
			Longitude: params.Longitude,
			Latitude:  params.Latitude,
			Distance:  params.Distance,
		}
		return filterExecutor.execute(table, filter, params.Options)
	})
}

// TODO: Document registerSpatialWithinMethod.
func registerSpatialWithinMethod(
	vm *goja.Runtime,
	spatialObj *goja.Object,
	cache *schema.SchemaCache,
	filterExecutor spatialFilterExecutor,
) {
	_ = spatialObj.Set("within", func(call goja.FunctionCall) goja.Value {
		params, err := parseWithinCallParams(vm, call)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		geometryType, err := spatial.ParseGeoJSONGeometry(params.GeoJSON)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		if geometryType != "Polygon" && geometryType != "MultiPolygon" {
			panic(vm.NewGoError(fmt.Errorf("within geojson must be Polygon or MultiPolygon, got %q", geometryType)))
		}
		table, column, err := resolveSpatialTableAndColumn(cache, params.TableName, params.ColumnName)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		filter := spatial.WithinFilter{
			Column:  column,
			GeoJSON: params.GeoJSON,
		}
		return filterExecutor.execute(table, filter, params.Options)
	})
}

// TODO: Document registerSpatialBBoxMethod.
func registerSpatialBBoxMethod(
	vm *goja.Runtime,
	spatialObj *goja.Object,
	cache *schema.SchemaCache,
	filterExecutor spatialFilterExecutor,
) {
	_ = spatialObj.Set("bbox", func(call goja.FunctionCall) goja.Value {
		params, err := parseBBoxCallParams(vm, call)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		if params.MinLng >= params.MaxLng {
			panic(vm.NewGoError(fmt.Errorf("minLng must be less than maxLng")))
		}
		if params.MinLat >= params.MaxLat {
			panic(vm.NewGoError(fmt.Errorf("minLat must be less than maxLat")))
		}
		table, column, err := resolveSpatialTableAndColumn(cache, params.TableName, params.ColumnName)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		filter := spatial.BBoxFilter{
			Column: column,
			MinLng: params.MinLng,
			MinLat: params.MinLat,
			MaxLng: params.MaxLng,
			MaxLat: params.MaxLat,
		}
		return filterExecutor.execute(table, filter, params.Options)
	})
}

func registerSpatialInfoMethod(
	vm *goja.Runtime,
	spatialObj *goja.Object,
	cacheGetter func() *schema.SchemaCache,
) {
	_ = spatialObj.Set("info", func(call goja.FunctionCall) goja.Value {
		latestCache := cacheGetter()
		return vm.ToValue(spatialInfoPayload(latestCache))
	})
}

// TODO: Document parseNearCallParams.
func parseNearCallParams(vm *goja.Runtime, call goja.FunctionCall) (nearCallParams, error) {
	tableName, err := requiredStringArgument(call, 0, "table")
	if err != nil {
		return nearCallParams{}, err
	}
	columnName, err := requiredStringArgument(call, 1, "column")
	if err != nil {
		return nearCallParams{}, err
	}
	longitude, err := requiredFloatArgument(call, 2, "lng")
	if err != nil {
		return nearCallParams{}, err
	}
	latitude, err := requiredFloatArgument(call, 3, "lat")
	if err != nil {
		return nearCallParams{}, err
	}
	if err := spatial.ValidateWGS84Point(longitude, latitude); err != nil {
		return nearCallParams{}, fmt.Errorf("near requires valid WGS84 coordinates: %w", err)
	}
	distance, err := requiredFloatArgument(call, 4, "distance")
	if err != nil {
		return nearCallParams{}, err
	}
	if distance <= 0 {
		return nearCallParams{}, fmt.Errorf("distance must be greater than 0")
	}
	opts := parseSpatialCallOptions(vm, call, 5)
	return nearCallParams{
		TableName:  tableName,
		ColumnName: columnName,
		Longitude:  longitude,
		Latitude:   latitude,
		Distance:   distance,
		Options:    opts,
	}, nil
}

// TODO: Document parseWithinCallParams.
func parseWithinCallParams(vm *goja.Runtime, call goja.FunctionCall) (withinCallParams, error) {
	tableName, err := requiredStringArgument(call, 0, "table")
	if err != nil {
		return withinCallParams{}, err
	}
	columnName, err := requiredStringArgument(call, 1, "column")
	if err != nil {
		return withinCallParams{}, err
	}
	geoJSON, err := requiredStringArgument(call, 2, "geojson")
	if err != nil {
		return withinCallParams{}, err
	}
	opts := parseSpatialCallOptions(vm, call, 3)
	return withinCallParams{
		TableName:  tableName,
		ColumnName: columnName,
		GeoJSON:    geoJSON,
		Options:    opts,
	}, nil
}

// TODO: Document parseBBoxCallParams.
func parseBBoxCallParams(vm *goja.Runtime, call goja.FunctionCall) (bboxCallParams, error) {
	tableName, err := requiredStringArgument(call, 0, "table")
	if err != nil {
		return bboxCallParams{}, err
	}
	columnName, err := requiredStringArgument(call, 1, "column")
	if err != nil {
		return bboxCallParams{}, err
	}
	minLng, err := requiredFloatArgument(call, 2, "minLng")
	if err != nil {
		return bboxCallParams{}, err
	}
	minLat, err := requiredFloatArgument(call, 3, "minLat")
	if err != nil {
		return bboxCallParams{}, err
	}
	maxLng, err := requiredFloatArgument(call, 4, "maxLng")
	if err != nil {
		return bboxCallParams{}, err
	}
	maxLat, err := requiredFloatArgument(call, 5, "maxLat")
	if err != nil {
		return bboxCallParams{}, err
	}
	opts := parseSpatialCallOptions(vm, call, 6)
	return bboxCallParams{
		TableName:  tableName,
		ColumnName: columnName,
		MinLng:     minLng,
		MinLat:     minLat,
		MaxLng:     maxLng,
		MaxLat:     maxLat,
		Options:    opts,
	}, nil
}

// TODO: Document spatialFilterExecutor.execute.
func (filterExecutor spatialFilterExecutor) execute(
	table *schema.Table,
	filter spatial.Filter,
	opts spatialCallOptions,
) goja.Value {
	spatialSQL, spatialArgs, err := filter.WhereClause(1)
	if err != nil {
		panic(filterExecutor.VM.NewGoError(err))
	}
	query, args := buildSpatialSelectSQL(table, spatialSQL, spatialArgs, opts.Limit, opts.Offset)
	result, err := filterExecutor.QueryExecutor.QueryRaw(filterExecutor.Context, query, args...)
	if err != nil {
		panic(filterExecutor.VM.NewGoError(fmt.Errorf("ayb.spatial query failed: %w", err)))
	}
	return filterExecutor.VM.ToValue(result.Rows)
}

func spatialInfoPayload(cache *schema.SchemaCache) map[string]any {
	summary := schema.BuildSpatialInfoSummary(cache)
	filteredTables := make([]schema.SpatialInfoTable, 0, len(summary.Tables))
	for _, table := range summary.Tables {
		if table.Schema != "public" {
			continue
		}
		filteredTables = append(filteredTables, table)
	}
	summary.Tables = filteredTables
	return summary.ToMap()
}

// TODO: Document resolveSpatialTableAndColumn.
func resolveSpatialTableAndColumn(cache *schema.SchemaCache, tableName, columnName string) (*schema.Table, *schema.Column, error) {
	if cache == nil {
		return nil, nil, fmt.Errorf("schema cache is unavailable")
	}
	trimmedTableName := strings.TrimSpace(tableName)
	if trimmedTableName == "" {
		return nil, nil, fmt.Errorf("table name is required")
	}

	// Keep ayb.spatial aligned with ayb.db's unqualified public-table scope so
	// the spatial bridge does not broaden edge-function access into other schemas.
	table := cache.Tables["public."+trimmedTableName]
	if table == nil {
		return nil, nil, fmt.Errorf("table %q not found in public schema", tableName)
	}
	column := table.ColumnByName(strings.TrimSpace(columnName))
	if column == nil {
		return nil, nil, fmt.Errorf("column %q not found in table %q", columnName, trimmedTableName)
	}
	if !column.IsGeometry {
		return nil, nil, fmt.Errorf("column %q is not a spatial column", columnName)
	}
	return table, column, nil
}

// TODO: Document buildSpatialSelectSQL.
func buildSpatialSelectSQL(table *schema.Table, spatialSQL string, spatialArgs []any, limit, offset int) (string, []any) {
	selectList := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		quotedName := sqlutil.QuoteIdent(column.Name)
		if column.IsGeometry {
			selectList = append(selectList, fmt.Sprintf("ST_AsGeoJSON(%s)::jsonb AS %s", quotedName, quotedName))
			continue
		}
		selectList = append(selectList, quotedName)
	}

	qualifiedTable := qualifiedTableName(table)
	sqlText := fmt.Sprintf("SELECT %s FROM %s WHERE %s", strings.Join(selectList, ", "), qualifiedTable, spatialSQL)

	args := make([]any, 0, len(spatialArgs)+2)
	args = append(args, spatialArgs...)

	if limit > 0 {
		args = append(args, limit)
		sqlText += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	if offset > 0 {
		args = append(args, offset)
		sqlText += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	return sqlText, args
}

func qualifiedTableName(table *schema.Table) string {
	if table.Schema == "" {
		return sqlutil.QuoteIdent(table.Name)
	}
	return sqlutil.QuoteIdent(table.Schema) + "." + sqlutil.QuoteIdent(table.Name)
}

func requiredStringArgument(call goja.FunctionCall, index int, name string) (string, error) {
	if len(call.Arguments) <= index {
		return "", fmt.Errorf("%s is required", name)
	}
	value := strings.TrimSpace(call.Arguments[index].String())
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

// TODO: Document requiredFloatArgument.
func requiredFloatArgument(call goja.FunctionCall, index int, name string) (float64, error) {
	if len(call.Arguments) <= index {
		return 0, fmt.Errorf("%s is required", name)
	}
	switch value := call.Arguments[index].Export().(type) {
	case float64:
		return value, nil
	case float32:
		return float64(value), nil
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case int32:
		return float64(value), nil
	case uint:
		return float64(value), nil
	case uint64:
		return float64(value), nil
	case uint32:
		return float64(value), nil
	default:
		return 0, fmt.Errorf("%s must be a number", name)
	}
}

// TODO: Document parseSpatialCallOptions.
func parseSpatialCallOptions(vm *goja.Runtime, call goja.FunctionCall, index int) spatialCallOptions {
	if len(call.Arguments) <= index || goja.IsUndefined(call.Arguments[index]) || goja.IsNull(call.Arguments[index]) {
		return spatialCallOptions{}
	}

	parsed := spatialCallOptions{}
	rawOptions := call.Arguments[index].Export()
	optionsMap, ok := rawOptions.(map[string]any)
	if !ok {
		panic(vm.NewGoError(fmt.Errorf("opts must be an object")))
	}

	if rawLimit, ok := optionsMap["limit"]; ok {
		limit, err := numericOptionToInt(rawLimit, "limit")
		if err != nil {
			panic(vm.NewGoError(err))
		}
		if limit <= 0 {
			panic(vm.NewGoError(fmt.Errorf("limit must be greater than 0")))
		}
		parsed.Limit = limit
	}
	if rawOffset, ok := optionsMap["offset"]; ok {
		offset, err := numericOptionToInt(rawOffset, "offset")
		if err != nil {
			panic(vm.NewGoError(err))
		}
		if offset < 0 {
			panic(vm.NewGoError(fmt.Errorf("offset must be greater than or equal to 0")))
		}
		parsed.Offset = offset
	}
	return parsed
}

// TODO: Document numericOptionToInt.
func numericOptionToInt(raw any, optionName string) (int, error) {
	switch value := raw.(type) {
	case int:
		return value, nil
	case int64:
		return int(value), nil
	case float64:
		if value != float64(int(value)) {
			return 0, fmt.Errorf("%s must be an integer", optionName)
		}
		return int(value), nil
	case string:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", optionName)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be an integer", optionName)
	}
}
