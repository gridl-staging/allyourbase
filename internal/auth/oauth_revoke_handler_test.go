package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeOAuthRevokeProvider struct {
	revokeCalls int
	lastToken   string
	revokeErr   error
}

func (f *fakeOAuthRevokeProvider) RevokeOAuthToken(_ context.Context, token string) error {
	f.revokeCalls++
	f.lastToken = token
	return f.revokeErr
}

func newRevokeHandler(prov *fakeOAuthRevokeProvider) *Handler {
	h := &Handler{
		logger:      testutil.DiscardLogger(),
		oauthRevoke: prov,
	}
	return h
}

func postRevokeForm(h *Handler, vals url.Values) *httptest.ResponseRecorder {
	body := vals.Encode()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/revoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleOAuthRevoke(w, req)
	return w
}

func TestOAuthRevokeAccessToken(t *testing.T) {
	t.Parallel()
	prov := &fakeOAuthRevokeProvider{}
	h := newRevokeHandler(prov)

	w := postRevokeForm(h, url.Values{
		"token":           {"ayb_at_somefaketoken"},
		"token_type_hint": {"access_token"},
	})

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, prov.revokeCalls)
	testutil.Equal(t, "ayb_at_somefaketoken", prov.lastToken)
}

func TestOAuthRevokeRefreshToken(t *testing.T) {
	t.Parallel()
	prov := &fakeOAuthRevokeProvider{}
	h := newRevokeHandler(prov)

	w := postRevokeForm(h, url.Values{
		"token":           {"ayb_rt_somefaketoken"},
		"token_type_hint": {"refresh_token"},
	})

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, prov.revokeCalls)
	testutil.Equal(t, "ayb_rt_somefaketoken", prov.lastToken)
}

func TestOAuthRevokeNoHint(t *testing.T) {
	t.Parallel()
	prov := &fakeOAuthRevokeProvider{}
	h := newRevokeHandler(prov)

	w := postRevokeForm(h, url.Values{
		"token": {"ayb_at_nohint"},
	})

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, prov.revokeCalls)
	testutil.Equal(t, "ayb_at_nohint", prov.lastToken)
}

func TestOAuthRevokeUnknownToken(t *testing.T) {
	t.Parallel()
	// Per RFC 7009: return 200 OK even for unknown tokens.
	prov := &fakeOAuthRevokeProvider{}
	h := newRevokeHandler(prov)

	w := postRevokeForm(h, url.Values{
		"token": {"totally_unknown_token"},
	})

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, prov.revokeCalls)
}

func TestOAuthRevokeMissingToken(t *testing.T) {
	t.Parallel()
	prov := &fakeOAuthRevokeProvider{}
	h := newRevokeHandler(prov)

	w := postRevokeForm(h, url.Values{})

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Equal(t, 0, prov.revokeCalls)

	var resp OAuthError
	err := json.NewDecoder(w.Body).Decode(&resp)
	testutil.NoError(t, err)
	testutil.Equal(t, OAuthErrInvalidRequest, resp.Code)
}

func TestOAuthRevokeRequiresFormContentType(t *testing.T) {
	t.Parallel()
	prov := &fakeOAuthRevokeProvider{}
	h := newRevokeHandler(prov)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/revoke", strings.NewReader(`{"token":"abc"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleOAuthRevoke(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Equal(t, 0, prov.revokeCalls)
}

func TestOAuthRevokeAlways200OnServiceError(t *testing.T) {
	t.Parallel()
	// Per RFC 7009: always return 200 regardless.
	prov := &fakeOAuthRevokeProvider{
		revokeErr: errors.New("db timeout"),
	}
	h := newRevokeHandler(prov)

	w := postRevokeForm(h, url.Values{
		"token": {"anything"},
	})

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, prov.revokeCalls)
	testutil.Equal(t, "anything", prov.lastToken)
}
