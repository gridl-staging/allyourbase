package templates

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestChatSchemaContainsAllTables(t *testing.T) {
	t.Parallel()
	dt := mustChatTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS rooms")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS participants")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS messages")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS read_receipts")
}

func TestChatSchemaContainsRLS(t *testing.T) {
	t.Parallel()
	dt := mustChatTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "rooms_select")
	testutil.Contains(t, schema, "rooms_insert")
	testutil.Contains(t, schema, "messages_select")
	testutil.Contains(t, schema, "messages_insert")
	testutil.Contains(t, schema, "messages_update")
	testutil.Contains(t, schema, "participants_select")
	testutil.Contains(t, schema, "participants_insert")
	testutil.Contains(t, schema, "read_receipts_select")
}

func TestChatSchemaConstrainsReadReceiptMessageToRoom(t *testing.T) {
	t.Parallel()
	dt := mustChatTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "m.room_id = read_receipts.room_id")
	testutil.Contains(t, schema, "last_read_message_id IS NULL")
}

func TestChatSchemaContainsRoomTypeConstraint(t *testing.T) {
	t.Parallel()
	dt := mustChatTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "'direct'")
	testutil.Contains(t, schema, "'group'")
	testutil.Contains(t, schema, "'channel'")
}

func TestChatSchemaContainsParticipantRoleConstraint(t *testing.T) {
	t.Parallel()
	dt := mustChatTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "'owner'")
	testutil.Contains(t, schema, "'admin'")
	testutil.Contains(t, schema, "'member'")
}

func TestChatSchemaUsesUUIDSessionCast(t *testing.T) {
	t.Parallel()
	dt := mustChatTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "current_setting('ayb.user_id', true)::uuid")
	testutil.False(t, strings.Contains(schema, "::text"))
}

func TestChatSeedDataContainsExpectedInserts(t *testing.T) {
	t.Parallel()
	dt := mustChatTemplate(t)

	seed := dt.SeedData()
	testutil.Contains(t, seed, "INSERT INTO rooms")
	testutil.Contains(t, seed, "INSERT INTO participants")
	testutil.Contains(t, seed, "INSERT INTO messages")
	testutil.Contains(t, seed, "INSERT INTO read_receipts")
}

func TestChatClientCodeContainsHelpers(t *testing.T) {
	t.Parallel()
	dt := mustChatTemplate(t)

	files := dt.ClientCode()
	code, ok := files["src/lib/chat.ts"]
	testutil.True(t, ok)
	testutil.Contains(t, code, "listRooms")
	testutil.Contains(t, code, "sendMessage")
	testutil.Contains(t, code, "markRead")
}

func TestChatClientCodeUsesCompositeKeysForMembershipAndReadReceipts(t *testing.T) {
	t.Parallel()
	dt := mustChatTemplate(t)

	files := dt.ClientCode()
	code, ok := files["src/lib/chat.ts"]
	testutil.True(t, ok)
	testutil.Contains(t, code, "roomId + \",\" + userId")
	testutil.Contains(t, code, "read_receipts\", roomId + \",\" + userId")
}

func TestChatClientCodeUsesSharedAuthenticatedUserHelper(t *testing.T) {
	t.Parallel()
	dt := mustChatTemplate(t)

	files := dt.ClientCode()
	code, ok := files["src/lib/chat.ts"]
	testutil.True(t, ok)
	testutil.Contains(t, code, "async function requireCurrentUserID()")
	testutil.Contains(t, code, "const userId = await requireCurrentUserID()")
}

func mustChatTemplate(t *testing.T) DomainTemplate {
	t.Helper()
	return mustTemplate(t, "chat")
}
