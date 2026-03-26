package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSpatialInfo(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/schema" {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "not found"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hasPostGIS":     true,
			"postGISVersion": "3.4",
			"tables": map[string]any{
				"public.places": map[string]any{
					"schema": "public",
					"name":   "places",
					"kind":   "table",
					"columns": []any{
						map[string]any{
							"name":         "id",
							"type":         "integer",
							"isGeometry":   false,
							"isGeography":  false,
							"geometryType": "",
							"srid":         0,
						},
						map[string]any{
							"name":         "location",
							"type":         "geometry",
							"isGeometry":   true,
							"isGeography":  false,
							"geometryType": "Point",
							"srid":         4326,
						},
					},
					"indexes": []any{
						map[string]any{
							"name":    "places_pkey",
							"method":  "btree",
							"columns": []any{"id"},
						},
					},
				},
			},
		})
	}))
	t.Cleanup(ts.Close)

	c := newClient(Config{BaseURL: ts.URL})
	_, out, err := handleSpatialInfo(context.Background(), c)
	testutil.NoError(t, err)
	testutil.True(t, out.HasPostGIS)
	testutil.Equal(t, "3.4", out.PostGISVersion)
	testutil.Equal(t, 1, len(out.Tables))
	testutil.Equal(t, "places", out.Tables[0].Table)
	testutil.Equal(t, 1, len(out.Tables[0].SpatialColumns))
	testutil.Equal(t, "location", out.Tables[0].SpatialColumns[0].Column)
	testutil.Equal(t, "Point", out.Tables[0].SpatialColumns[0].GeometryType)
	testutil.Equal(t, 4326, out.Tables[0].SpatialColumns[0].SRID)
	testutil.Equal(t, 1, len(out.Tables[0].ColumnsMissingSpatialIndex))
	testutil.Equal(t, "location", out.Tables[0].ColumnsMissingSpatialIndex[0])
}

func TestSpatialQueryBuildsRESTParams(t *testing.T) {
	t.Parallel()

	var capturedQuery url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":      spatialQueryItems(15),
			"page":       1,
			"perPage":    15,
			"totalItems": 27,
			"totalPages": 2,
		})
	}))
	t.Cleanup(ts.Close)

	c := newClient(Config{BaseURL: ts.URL})
	_, out, err := handleSpatialQuery(context.Background(), c, SpatialQueryInput{
		Table:      "places",
		Column:     "location",
		FilterType: "near",
		Longitude:  ptrFloat(-73.9857),
		Latitude:   ptrFloat(40.7484),
		Distance:   ptrFloat(1000),
		Filter:     `name='empire'`,
		Sort:       "-id",
		Limit:      ptrInt(10),
		Offset:     ptrInt(5),
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 10, len(out.Items))
	testutil.Equal(t, 1, out.Page)
	testutil.Equal(t, 10, out.PerPage)
	testutil.Equal(t, 27, out.TotalItems)
	testutil.Equal(t, 3, out.TotalPages)
	testutil.Equal(t, float64(6), out.Items[0]["id"].(float64))
	testutil.Equal(t, "location,-73.9857,40.7484,1000", capturedQuery.Get("near"))
	testutil.Equal(t, "name='empire'", capturedQuery.Get("filter"))
	testutil.Equal(t, "-id", capturedQuery.Get("sort"))
	testutil.Equal(t, "1", capturedQuery.Get("page"))
	testutil.Equal(t, "15", capturedQuery.Get("perPage"))
}

func TestSpatialQueryBuildsRESTParamsForOtherFilterTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		input      SpatialQueryInput
		paramName  string
		paramValue string
	}{
		{
			name: "within",
			input: SpatialQueryInput{
				Table:      "places",
				Column:     "location",
				FilterType: "within",
				GeoJSON:    `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`,
			},
			paramName:  "within",
			paramValue: `location,{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`,
		},
		{
			name: "intersects",
			input: SpatialQueryInput{
				Table:      "places",
				Column:     "location",
				FilterType: "intersects",
				GeoJSON:    `{"type":"LineString","coordinates":[[0,0],[1,1]]}`,
			},
			paramName:  "intersects",
			paramValue: `location,{"type":"LineString","coordinates":[[0,0],[1,1]]}`,
		},
		{
			name: "bbox",
			input: SpatialQueryInput{
				Table:      "places",
				Column:     "location",
				FilterType: "bbox",
				MinLng:     ptrFloat(-74.1),
				MinLat:     ptrFloat(40.6),
				MaxLng:     ptrFloat(-73.8),
				MaxLat:     ptrFloat(40.9),
			},
			paramName:  "bbox",
			paramValue: "location,-74.1,40.6,-73.8,40.9",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var capturedQuery url.Values
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedQuery = r.URL.Query()
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items":      []any{map[string]any{"id": 1}},
					"page":       1,
					"perPage":    20,
					"totalItems": 1,
					"totalPages": 1,
				})
			}))
			t.Cleanup(ts.Close)

			c := newClient(Config{BaseURL: ts.URL})
			_, out, err := handleSpatialQuery(context.Background(), c, tc.input)
			testutil.NoError(t, err)
			testutil.Equal(t, 1, len(out.Items))
			testutil.Equal(t, tc.paramValue, capturedQuery.Get(tc.paramName))
		})
	}
}

func TestSpatialQueryReturnsEmptyItemsSliceWhenTranslatedWindowStartsPastReturnedItems(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":      spatialQueryItems(3),
			"page":       1,
			"perPage":    15,
			"totalItems": 3,
			"totalPages": 1,
		})
	}))
	t.Cleanup(ts.Close)

	c := newClient(Config{BaseURL: ts.URL})
	_, out, err := handleSpatialQuery(context.Background(), c, SpatialQueryInput{
		Table:      "places",
		Column:     "location",
		FilterType: "near",
		Longitude:  ptrFloat(-73.9857),
		Latitude:   ptrFloat(40.7484),
		Distance:   ptrFloat(1000),
		Limit:      ptrInt(10),
		Offset:     ptrInt(5),
	})
	testutil.NoError(t, err)
	testutil.True(t, out.Items != nil, "expected empty items slice instead of nil")
	testutil.Equal(t, 0, len(out.Items))
}

func TestSpatialQueryValidationErrors(t *testing.T) {
	t.Parallel()
	c := newClient(Config{BaseURL: "http://example.com"})

	_, _, err := handleSpatialQuery(context.Background(), c, SpatialQueryInput{
		Table:      "places",
		Column:     "location",
		FilterType: "near",
		Longitude:  ptrFloat(-73.9),
		Latitude:   ptrFloat(40.7),
	})
	testutil.ErrorContains(t, err, "distance")

	_, _, err = handleSpatialQuery(context.Background(), c, SpatialQueryInput{
		Table:      "places",
		Column:     "location",
		FilterType: "within",
	})
	testutil.ErrorContains(t, err, "geojson")

	_, _, err = handleSpatialQuery(context.Background(), c, SpatialQueryInput{
		Table:      "places",
		Column:     "location",
		FilterType: "bbox",
		MinLng:     ptrFloat(10),
		MinLat:     ptrFloat(0),
		MaxLng:     ptrFloat(9),
		MaxLat:     ptrFloat(1),
	})
	testutil.ErrorContains(t, err, "min_lng")

	_, _, err = handleSpatialQuery(context.Background(), c, SpatialQueryInput{
		Table:      "places",
		Column:     "location",
		FilterType: "near",
		Longitude:  ptrFloat(181),
		Latitude:   ptrFloat(40.7),
		Distance:   ptrFloat(1000),
	})
	testutil.ErrorContains(t, err, "valid WGS84 coordinates")

	_, _, err = handleSpatialQuery(context.Background(), c, SpatialQueryInput{
		Table:      "places",
		Column:     "location",
		FilterType: "invalid",
	})
	testutil.ErrorContains(t, err, "filter_type")

	_, _, err = handleSpatialQuery(context.Background(), c, SpatialQueryInput{
		Table:      "places",
		Column:     "location",
		FilterType: "near",
		Longitude:  ptrFloat(-73.9),
		Latitude:   ptrFloat(40.7),
		Distance:   ptrFloat(1000),
		Offset:     ptrInt(1),
	})
	testutil.ErrorContains(t, err, "offset requires limit")

	_, _, err = handleSpatialQuery(context.Background(), c, SpatialQueryInput{
		Table:      "places",
		Column:     "location",
		FilterType: "near",
		Longitude:  ptrFloat(-73.9),
		Latitude:   ptrFloat(40.7),
		Distance:   ptrFloat(1000),
		Limit:      ptrInt(500),
		Offset:     ptrInt(1),
	})
	testutil.ErrorContains(t, err, "cannot be represented")
}

func ptrFloat(v float64) *float64 {
	return &v
}

func ptrInt(v int) *int {
	return &v
}

func spatialQueryItems(count int) []any {
	items := make([]any, 0, count)
	for i := 1; i <= count; i++ {
		items = append(items, map[string]any{"id": i})
	}
	return items
}
