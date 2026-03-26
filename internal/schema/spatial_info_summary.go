package schema

// SpatialInfoSummary is a reusable PostGIS summary for tool surfaces that expose spatial metadata.
type SpatialInfoSummary struct {
	HasPostGIS     bool               `json:"hasPostGIS"`
	PostGISVersion string             `json:"postGISVersion,omitempty"`
	Tables         []SpatialInfoTable `json:"tables"`
}

// SpatialInfoTable summarizes one table with spatial columns.
type SpatialInfoTable struct {
	Schema                     string              `json:"schema"`
	Table                      string              `json:"table"`
	SpatialColumns             []SpatialInfoColumn `json:"spatialColumns"`
	ColumnsMissingSpatialIndex []string            `json:"columnsMissingSpatialIndex,omitempty"`
}

// SpatialInfoColumn summarizes one spatial column and its geometry metadata.
type SpatialInfoColumn struct {
	Column       string `json:"column"`
	GeometryType string `json:"geometryType,omitempty"`
	SRID         int    `json:"srid,omitempty"`
	IsGeography  bool   `json:"isGeography"`
}

// BuildSpatialInfoSummary reshapes schema metadata into a shared spatial summary.
func BuildSpatialInfoSummary(cache *SchemaCache) SpatialInfoSummary {
	summary := SpatialInfoSummary{Tables: make([]SpatialInfoTable, 0)}
	if cache == nil {
		return summary
	}

	summary.HasPostGIS = cache.HasPostGIS
	summary.PostGISVersion = cache.PostGISVersion
	for _, table := range cache.TableList() {
		if table == nil || !table.HasGeometry() {
			continue
		}

		tableSummary := SpatialInfoTable{
			Schema:         table.Schema,
			Table:          table.Name,
			SpatialColumns: make([]SpatialInfoColumn, 0),
		}
		for _, column := range table.Columns {
			if column == nil || !column.IsGeometry {
				continue
			}
			tableSummary.SpatialColumns = append(tableSummary.SpatialColumns, SpatialInfoColumn{
				Column:       column.Name,
				GeometryType: column.GeometryType,
				SRID:         column.SRID,
				IsGeography:  column.IsGeography,
			})
		}

		missingIndexes := table.SpatialColumnsWithoutIndex()
		tableSummary.ColumnsMissingSpatialIndex = make([]string, 0, len(missingIndexes))
		for _, column := range missingIndexes {
			if column == nil {
				continue
			}
			tableSummary.ColumnsMissingSpatialIndex = append(tableSummary.ColumnsMissingSpatialIndex, column.Name)
		}
		summary.Tables = append(summary.Tables, tableSummary)
	}

	return summary
}

// ToMap converts the summary to a JS-friendly payload for the edge-function bridge.
func (summary SpatialInfoSummary) ToMap() map[string]any {
	tablePayload := make([]map[string]any, 0, len(summary.Tables))
	for _, table := range summary.Tables {
		columnPayload := make([]map[string]any, 0, len(table.SpatialColumns))
		for _, column := range table.SpatialColumns {
			columnPayload = append(columnPayload, map[string]any{
				"column":       column.Column,
				"geometryType": column.GeometryType,
				"srid":         column.SRID,
				"isGeography":  column.IsGeography,
			})
		}
		tablePayload = append(tablePayload, map[string]any{
			"schema":                     table.Schema,
			"table":                      table.Table,
			"spatialColumns":             columnPayload,
			"columnsMissingSpatialIndex": table.ColumnsMissingSpatialIndex,
		})
	}

	payload := map[string]any{
		"hasPostGIS": summary.HasPostGIS,
		"tables":     tablePayload,
	}
	if summary.PostGISVersion != "" {
		payload["postGISVersion"] = summary.PostGISVersion
	}
	return payload
}
