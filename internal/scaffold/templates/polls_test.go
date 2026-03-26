package templates

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestPollsSchemaContainsAllTables(t *testing.T) {
	t.Parallel()
	dt := mustPollsTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS polls")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS poll_options")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS votes")
}

func TestPollsSchemaContainsRLS(t *testing.T) {
	t.Parallel()
	dt := mustPollsTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "polls_select")
	testutil.Contains(t, schema, "polls_insert")
	testutil.Contains(t, schema, "polls_update")
	testutil.Contains(t, schema, "polls_delete")
	testutil.Contains(t, schema, "poll_options_select")
	testutil.Contains(t, schema, "votes_select")
	testutil.Contains(t, schema, "votes_insert")
}

func TestPollsSchemaUsesUUIDSessionCast(t *testing.T) {
	t.Parallel()
	dt := mustPollsTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "current_setting('ayb.user_id', true)::uuid")
	testutil.False(t, strings.Contains(schema, "::text"))
}

func TestPollsSchemaContainsSingleChoiceUniqueConstraint(t *testing.T) {
	t.Parallel()
	dt := mustPollsTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "UNIQUE")
	testutil.Contains(t, schema, "poll_id")
	testutil.Contains(t, schema, "user_id")
}

func TestPollsSchemaConstrainsVoteOptionToSamePoll(t *testing.T) {
	t.Parallel()
	dt := mustPollsTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "FOREIGN KEY (poll_id, option_id)")
	testutil.Contains(t, schema, "REFERENCES poll_options(poll_id, id)")
}

func TestPollsSeedDataContainsExpectedInserts(t *testing.T) {
	t.Parallel()
	dt := mustPollsTemplate(t)

	seed := dt.SeedData()
	testutil.Contains(t, seed, "INSERT INTO polls")
	testutil.Contains(t, seed, "INSERT INTO poll_options")
	testutil.Contains(t, seed, "INSERT INTO votes")
	testutil.Contains(t, seed, "ON CONFLICT DO NOTHING")
}

func TestPollsClientCodeContainsHelpers(t *testing.T) {
	t.Parallel()
	dt := mustPollsTemplate(t)

	files := dt.ClientCode()
	code, ok := files["src/lib/polls.ts"]
	testutil.True(t, ok)
	testutil.Contains(t, code, "listPolls")
	testutil.Contains(t, code, "castVote")
	testutil.Contains(t, code, "createPoll")
}

func mustPollsTemplate(t *testing.T) DomainTemplate {
	t.Helper()
	return mustTemplate(t, "polls")
}
