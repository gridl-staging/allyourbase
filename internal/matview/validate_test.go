package matview

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestValidateIdentifier(t *testing.T) {
	t.Parallel()

	valid := []string{"public", "leaderboard", "_internal1", "A1"}
	for _, v := range valid {
		err := ValidateIdentifier(v)
		testutil.NoError(t, err)
	}

	invalid := []string{"", "1abc", "bad-name", "bad.name", "bad name", `bad"quote`}
	for _, v := range invalid {
		err := ValidateIdentifier(v)
		testutil.True(t, err != nil, "expected invalid identifier %q", v)
	}
}

func TestBuildRefreshSQL(t *testing.T) {
	t.Parallel()

	sql, err := BuildRefreshSQL("public", "leaderboard", RefreshModeStandard)
	testutil.NoError(t, err)
	testutil.Equal(t, `REFRESH MATERIALIZED VIEW "public"."leaderboard"`, sql)

	sql, err = BuildRefreshSQL("public", "leaderboard", RefreshModeConcurrent)
	testutil.NoError(t, err)
	testutil.Equal(t, `REFRESH MATERIALIZED VIEW CONCURRENTLY "public"."leaderboard"`, sql)
}
