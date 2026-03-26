package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSetTriggerServices_TypedNilCronServiceTreatedAsDisabled(t *testing.T) {
	t.Parallel()

	s := &Server{}
	var cronSvc *edgefunc.CronTriggerService

	s.SetTriggerServices(nil, cronSvc, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/functions/fn/triggers/cron", nil)
	w := httptest.NewRecorder()
	s.handleCronTriggerList(w, req)

	testutil.StatusCode(t, http.StatusServiceUnavailable, w.Code)
	testutil.Contains(t, w.Body.String(), "edge functions are not enabled")
}

func TestSetTriggerServices_TypedNilDBServiceTreatedAsDisabled(t *testing.T) {
	t.Parallel()

	s := &Server{}
	var dbSvc *edgefunc.DBTriggerService

	s.SetTriggerServices(dbSvc, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/functions/fn/triggers/db", nil)
	w := httptest.NewRecorder()
	s.handleDBTriggerList(w, req)

	testutil.StatusCode(t, http.StatusServiceUnavailable, w.Code)
	testutil.Contains(t, w.Body.String(), "edge functions are not enabled")
}

func TestSetTriggerServices_TypedNilStorageServiceTreatedAsDisabled(t *testing.T) {
	t.Parallel()

	s := &Server{}
	var storageSvc *edgefunc.StorageTriggerService

	s.SetTriggerServices(nil, nil, storageSvc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/functions/fn/triggers/storage", nil)
	w := httptest.NewRecorder()
	s.handleStorageTriggerList(w, req)

	testutil.StatusCode(t, http.StatusServiceUnavailable, w.Code)
	testutil.Contains(t, w.Body.String(), "edge functions are not enabled")
}

func TestSetTriggerServices_TypedNilInvokerTreatedAsDisabledForManualRun(t *testing.T) {
	t.Parallel()

	s := &Server{}
	cronSvc := newFakeCronTriggerAdmin()
	var invoker *fakeManualRunInvoker

	s.SetTriggerServices(nil, cronSvc, nil, invoker)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/functions/fn/triggers/cron/ct-001/run", nil)
	w := httptest.NewRecorder()
	s.handleCronTriggerManualRun(w, req)

	testutil.StatusCode(t, http.StatusServiceUnavailable, w.Code)
	testutil.Contains(t, w.Body.String(), "edge functions are not enabled")
}
