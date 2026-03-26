package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestServiceUnavailable(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	serviceUnavailable(w, "edge functions are not enabled")

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp httputil.ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	testutil.NoError(t, err)
	testutil.Equal(t, http.StatusServiceUnavailable, resp.Code)
	testutil.Equal(t, "edge functions are not enabled", resp.Message)
}

func requestWithChiParam(param, value string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(param, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestParseUUIDParamValidUUID(t *testing.T) {
	t.Parallel()

	want := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	r := requestWithChiParam("id", want.String())
	w := httptest.NewRecorder()

	got, ok := parseUUIDParam(w, r, "id")
	testutil.True(t, ok)
	testutil.Equal(t, want, got)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestParseUUIDParamEmptyString(t *testing.T) {
	t.Parallel()

	r := requestWithChiParam("id", "")
	w := httptest.NewRecorder()

	_, ok := parseUUIDParam(w, r, "id")
	testutil.True(t, !ok)
	testutil.Equal(t, http.StatusBadRequest, w.Code)

	var resp httputil.ErrorResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Contains(t, resp.Message, "id")
}

func TestParseUUIDParamMalformed(t *testing.T) {
	t.Parallel()

	r := requestWithChiParam("appID", "not-a-uuid")
	w := httptest.NewRecorder()

	_, ok := parseUUIDParam(w, r, "appID")
	testutil.True(t, !ok)
	testutil.Equal(t, http.StatusBadRequest, w.Code)

	var resp httputil.ErrorResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Contains(t, resp.Message, "appID")
}

func TestParseUUIDParamWithLabelMalformed(t *testing.T) {
	t.Parallel()

	r := requestWithChiParam("id", "not-a-uuid")
	w := httptest.NewRecorder()

	_, ok := parseUUIDParamWithLabel(w, r, "id", "prompt id")
	testutil.True(t, !ok)
	testutil.Equal(t, http.StatusBadRequest, w.Code)

	var resp httputil.ErrorResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, "invalid prompt id format", resp.Message)
}
