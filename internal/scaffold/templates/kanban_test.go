package templates

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestKanbanSchemaContainsAllTables(t *testing.T) {
	t.Parallel()
	dt := mustKanbanTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS boards")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS columns")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS cards")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS labels")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS card_labels")
}

func TestKanbanSchemaContainsRLS(t *testing.T) {
	t.Parallel()
	dt := mustKanbanTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "boards_select")
	testutil.Contains(t, schema, "boards_insert")
	testutil.Contains(t, schema, "boards_update")
	testutil.Contains(t, schema, "boards_delete")
	testutil.Contains(t, schema, "columns_select")
	testutil.Contains(t, schema, "cards_update")
	testutil.Contains(t, schema, "labels_insert")
	testutil.Contains(t, schema, "card_labels_insert")
}

func TestKanbanSchemaUsesUUIDSessionCast(t *testing.T) {
	t.Parallel()
	dt := mustKanbanTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "current_setting('ayb.user_id', true)::uuid")
	testutil.False(t, strings.Contains(schema, "::text"))
}

func TestKanbanSeedDataContainsExpectedInserts(t *testing.T) {
	t.Parallel()
	dt := mustKanbanTemplate(t)

	seed := dt.SeedData()
	testutil.Contains(t, seed, "INSERT INTO boards")
	testutil.Contains(t, seed, "INSERT INTO columns")
	testutil.Contains(t, seed, "INSERT INTO cards")
	testutil.Contains(t, seed, "INSERT INTO labels")
	testutil.Contains(t, seed, "INSERT INTO card_labels")
}

func TestKanbanClientCodeContainsHelpers(t *testing.T) {
	t.Parallel()
	dt := mustKanbanTemplate(t)

	files := dt.ClientCode()
	code, ok := files["src/lib/kanban.ts"]
	testutil.True(t, ok)
	testutil.Contains(t, code, "listBoards")
	testutil.Contains(t, code, "createCard")
	testutil.Contains(t, code, "moveCard")
}

func TestNamesIncludesAllRegistered(t *testing.T) {
	t.Parallel()
	names := Names()
	testutil.Equal(t, 5, len(names))
	testutil.Contains(t, strings.Join(names, ","), "blog")
	testutil.Contains(t, strings.Join(names, ","), "kanban")
	testutil.Contains(t, strings.Join(names, ","), "ecommerce")
	testutil.Contains(t, strings.Join(names, ","), "polls")
	testutil.Contains(t, strings.Join(names, ","), "chat")
}

func mustKanbanTemplate(t *testing.T) DomainTemplate {
	t.Helper()
	return mustTemplate(t, "kanban")
}
