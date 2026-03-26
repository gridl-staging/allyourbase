package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/fdw"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

type fakeFDWService struct {
	createErr error
	createReq fdw.CreateServerOpts

	listServersErr error
	listServersRes []fdw.ForeignServer

	dropServerErr     error
	dropServerName    string
	dropServerCascade bool
	dropServerCalled  bool
	importErr         error
	importServerName  string
	importReq         fdw.ImportOpts
	importRes         []fdw.ForeignTable
	listTablesErr     error
	listTablesRes     []fdw.ForeignTable
	dropTableErr      error
	dropTableSchema   string
	dropTableName     string
	dropTableCalled   bool
}

func (f *fakeFDWService) CreateServer(_ context.Context, opts fdw.CreateServerOpts) error {
	f.createReq = opts
	return f.createErr
}

func (f *fakeFDWService) ListServers(_ context.Context) ([]fdw.ForeignServer, error) {
	return f.listServersRes, f.listServersErr
}

func (f *fakeFDWService) DropServer(_ context.Context, name string, cascade bool) error {
	f.dropServerCalled = true
	f.dropServerName = name
	f.dropServerCascade = cascade
	return f.dropServerErr
}

func (f *fakeFDWService) ImportTables(_ context.Context, serverName string, opts fdw.ImportOpts) ([]fdw.ForeignTable, error) {
	f.importServerName = serverName
	f.importReq = opts
	return f.importRes, f.importErr
}

func (f *fakeFDWService) ListForeignTables(_ context.Context) ([]fdw.ForeignTable, error) {
	return f.listTablesRes, f.listTablesErr
}

func (f *fakeFDWService) DropForeignTable(_ context.Context, schemaName, tableName string) error {
	f.dropTableCalled = true
	f.dropTableSchema = schemaName
	f.dropTableName = tableName
	return f.dropTableErr
}

func fdwTestServer(svc fdwAdmin) *Server {
	s := &Server{fdwService: svc}
	r := chi.NewRouter()
	r.Route("/api/admin/fdw", func(r chi.Router) {
		r.Route("/servers", func(r chi.Router) {
			r.Get("/", s.handleAdminFDWListServers)
			r.Post("/", s.handleAdminFDWCreateServer)
			r.Post("/{name}/import", s.handleAdminFDWImportTables)
			r.Delete("/{name}", s.handleAdminFDWDropServer)
		})
		r.Route("/tables", func(r chi.Router) {
			r.Get("/", s.handleAdminFDWListTables)
			r.Delete("/{schema}/{table}", s.handleAdminFDWDropTable)
		})
	})
	s.router = r
	return s
}

func TestAdminFDWListServersReturnsEmptyWhenServiceNil(t *testing.T) {
	t.Parallel()

	s := fdwTestServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/fdw/servers", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.NotNil(t, body["servers"])
}

func TestAdminFDWCreateServerReturns503WhenServiceNil(t *testing.T) {
	t.Parallel()

	s := fdwTestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/fdw/servers", strings.NewReader(`{"name":"analytics_fdw","fdw_type":"postgres_fdw","options":{"host":"localhost","port":"5432","dbname":"app"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestAdminFDWCreateServerSuccess(t *testing.T) {
	t.Parallel()

	svc := &fakeFDWService{}
	s := fdwTestServer(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/fdw/servers", strings.NewReader(`{"name":"analytics_fdw","fdw_type":"postgres_fdw","options":{"host":"localhost","port":"5432","dbname":"app"},"user_mapping":{"user":"reporter","password":"secret"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "analytics_fdw", svc.createReq.Name)
	testutil.Equal(t, "postgres_fdw", svc.createReq.FDWType)

	var body map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.Equal(t, "analytics_fdw", body["name"])
	testutil.Equal(t, "postgres_fdw", body["type"])
}

func TestAdminFDWCreateServerInvalidNameReturns400(t *testing.T) {
	t.Parallel()

	svc := &fakeFDWService{createErr: errors.New(`identifier "bad name" contains invalid characters`)}
	s := fdwTestServer(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/fdw/servers", strings.NewReader(`{"name":"bad name","fdw_type":"postgres_fdw","options":{"host":"localhost","port":"5432","dbname":"app"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminFDWImportTablesSuccess(t *testing.T) {
	t.Parallel()

	svc := &fakeFDWService{
		importRes: []fdw.ForeignTable{
			{Schema: "public", Name: "events", ServerName: "analytics_fdw"},
		},
	}
	s := fdwTestServer(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/fdw/servers/analytics_fdw/import", strings.NewReader(`{"remote_schema":"public","local_schema":"public","table_names":["events"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "analytics_fdw", svc.importServerName)
	testutil.Equal(t, "public", svc.importReq.RemoteSchema)
	testutil.Equal(t, "public", svc.importReq.LocalSchema)
	testutil.SliceLen(t, svc.importReq.TableNames, 1)

	var body map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.NotNil(t, body["tables"])
}

func TestAdminFDWListTablesReturnsEmptyWhenServiceNil(t *testing.T) {
	t.Parallel()

	s := fdwTestServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/fdw/tables", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.NotNil(t, body["tables"])
}

func TestAdminFDWDropServerSuccessPassesCascade(t *testing.T) {
	t.Parallel()

	svc := &fakeFDWService{}
	s := fdwTestServer(svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/fdw/servers/analytics_fdw?cascade=true", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.True(t, svc.dropServerCalled)
	testutil.Equal(t, "analytics_fdw", svc.dropServerName)
	testutil.True(t, svc.dropServerCascade)
}

func TestAdminFDWDropTableSuccess(t *testing.T) {
	t.Parallel()

	svc := &fakeFDWService{}
	s := fdwTestServer(svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/fdw/tables/public/events", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.True(t, svc.dropTableCalled)
	testutil.Equal(t, "public", svc.dropTableSchema)
	testutil.Equal(t, "events", svc.dropTableName)
}
