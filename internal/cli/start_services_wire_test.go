package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestGraphQLAdminCheckerForIntrospectionMode(t *testing.T) {
	baseChecker := func(*http.Request) bool { return true }
	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)

	testutil.Nil(t, graphQLAdminCheckerForIntrospectionMode("open", baseChecker))

	disabledChecker := graphQLAdminCheckerForIntrospectionMode("disabled", baseChecker)
	testutil.NotNil(t, disabledChecker)
	testutil.False(t, disabledChecker(req))

	defaultChecker := graphQLAdminCheckerForIntrospectionMode("locked", baseChecker)
	testutil.NotNil(t, defaultChecker)
	testutil.True(t, defaultChecker(req))
}
