package api

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

// --- buildExportQuery tests ---

func TestBuildExportQuery_Bare(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	opts := listOpts{}
	q, args := buildExportQuery(tbl, opts, 0)
	if !strings.Contains(q, "SELECT") || !strings.Contains(q, `"public"."users"`) {
		t.Fatalf("unexpected query: %s", q)
	}
	if strings.Contains(q, "LIMIT") || strings.Contains(q, "OFFSET") {
		t.Fatalf("export query should not have LIMIT/OFFSET: %s", q)
	}
	testutil.SliceLen(t, args, 0)
}

func TestBuildExportQuery_WithFilter(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	opts := listOpts{
		filterSQL:  `"email" = $1`,
		filterArgs: []any{"test@example.com"},
	}
	q, args := buildExportQuery(tbl, opts, 0)
	if !strings.Contains(q, `"email" = $1`) {
		t.Fatalf("expected filter in query: %s", q)
	}
	if strings.Contains(q, "LIMIT") {
		t.Fatalf("export query should not have LIMIT: %s", q)
	}
	testutil.SliceLen(t, args, 1)
}

func TestBuildExportQuery_WithSort(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	opts := listOpts{
		sortSQL: `"name" ASC`,
	}
	q, _ := buildExportQuery(tbl, opts, 0)
	if !strings.Contains(q, `ORDER BY "name" ASC`) {
		t.Fatalf("expected sort in query: %s", q)
	}
}

func TestBuildExportQuery_WithSearch(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	opts := listOpts{
		searchSQL:  `to_tsvector('simple', "name") @@ websearch_to_tsquery('simple', $1)`,
		searchArgs: []any{"alice"},
		searchRank: `ts_rank(to_tsvector('simple', "name"), websearch_to_tsquery('simple', $1))`,
	}
	q, args := buildExportQuery(tbl, opts, 0)
	if !strings.Contains(q, "WHERE") {
		t.Fatalf("expected WHERE clause: %s", q)
	}
	if !strings.Contains(q, "ORDER BY") {
		t.Fatalf("expected ORDER BY from search rank: %s", q)
	}
	testutil.SliceLen(t, args, 1)
}

func TestBuildExportQuery_WithFields(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	opts := listOpts{
		fields: []string{"id", "email"},
	}
	q, _ := buildExportQuery(tbl, opts, 0)
	if !strings.Contains(q, `"id"`) || !strings.Contains(q, `"email"`) {
		t.Fatalf("expected field projection: %s", q)
	}
}

func TestBuildExportQuery_WithRowLimit(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	opts := listOpts{}
	q, _ := buildExportQuery(tbl, opts, 5)
	if !strings.Contains(q, " LIMIT 5") {
		t.Fatalf("expected export query to include row limit: %s", q)
	}
}

// --- formatCSVValue tests ---

func TestFormatCSVValue_Nil(t *testing.T) {
	testutil.Equal(t, "", formatCSVValue(nil))
}

func TestFormatCSVValue_String(t *testing.T) {
	testutil.Equal(t, "hello", formatCSVValue("hello"))
}

func TestFormatCSVValue_Bool(t *testing.T) {
	testutil.Equal(t, "true", formatCSVValue(true))
	testutil.Equal(t, "false", formatCSVValue(false))
}

func TestFormatCSVValue_Int(t *testing.T) {
	testutil.Equal(t, "42", formatCSVValue(int64(42)))
}

func TestFormatCSVValue_Float(t *testing.T) {
	testutil.Equal(t, "3.14", formatCSVValue(float64(3.14)))
}

func TestFormatCSVValue_Time(t *testing.T) {
	ts := time.Date(2024, 1, 15, 9, 30, 0, 123456789, time.UTC)
	testutil.Equal(t, ts.Format(time.RFC3339), formatCSVValue(ts))
}

func TestFormatCSVValue_Bytes(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	result := formatCSVValue(data)
	expected := base64.StdEncoding.EncodeToString(data)
	testutil.Equal(t, expected, result)
}

func TestFormatCSVValue_Number(t *testing.T) {
	testutil.Equal(t, "99", formatCSVValue(int32(99)))
}

// --- exportColumnNames tests ---

func TestExportColumnNames_AllColumns(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	names := exportColumnNames(tbl, nil)
	// Should match tbl.Columns order: id, email, name
	testutil.SliceLen(t, names, 3)
	testutil.Equal(t, "id", names[0])
	testutil.Equal(t, "email", names[1])
	testutil.Equal(t, "name", names[2])
}

func TestExportColumnNames_WithFields(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	names := exportColumnNames(tbl, []string{"email", "id"})
	// Should preserve schema order, not request order, but only include requested fields.
	testutil.SliceLen(t, names, 2)
	testutil.Equal(t, "id", names[0])
	testutil.Equal(t, "email", names[1])
}

func TestExportColumnNames_UnknownFieldsIgnored(t *testing.T) {
	tbl := testSchema().Tables["public.users"]
	names := exportColumnNames(tbl, []string{"bogus", "email"})
	testutil.SliceLen(t, names, 1)
	testutil.Equal(t, "email", names[0])
}

// --- Handler route wiring tests ---

func TestExportCSV_RouteWired(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/users/export.csv", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestExportJSON_RouteWired(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/users/export.json", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestExportCSV_UnknownTable(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/nonexistent/export.csv", "")
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestExportJSON_UnknownTable(t *testing.T) {
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/nonexistent/export.json", "")
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestExportCSV_InvalidFilter(t *testing.T) {
	sc := &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {
				Schema: "public",
				Name:   "users",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "uuid"},
					{Name: "email", TypeName: "text"},
				},
				PrimaryKey: []string{"id"},
			},
		},
		Schemas: []string{"public"},
	}
	h := testHandler(sc)
	w := doRequest(h, "GET", "/collections/users/export.csv?filter=bogus_col='x'", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}
