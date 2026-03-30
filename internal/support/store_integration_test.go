//go:build integration

package support_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/support"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

// testTenantID is a fixed UUID for the test tenant created in resetAndMigrate.
const testTenantID = "00000000-0000-0000-0000-000000000001"

// testUserID and testUserID2 are fixed UUIDs for test users.
const testUserID = "00000000-0000-0000-0000-000000000010"
const testUserID2 = "00000000-0000-0000-0000-000000000020"

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// resetAndMigrate drops and recreates the public schema, runs all migrations,
// and inserts a test tenant so that FK constraints on _ayb_support_tickets are satisfied.
func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	r := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, r.Bootstrap(ctx))
	_, err = r.Run(ctx)
	testutil.NoError(t, err)

	// Insert a test tenant for FK references.
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_tenants (id, name, slug, state) VALUES ($1, 'Test Tenant', 'test-tenant', 'active')`,
		testTenantID)
	testutil.NoError(t, err)
}

func TestCreateAndGetTicket(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := support.NewStore(sharedPG.Pool)

	// CreateTicket should insert a ticket with the initial customer message.
	ticket, err := store.CreateTicket(ctx, testTenantID, testUserID, "Login broken", "I can't log in", support.TicketPriorityHigh)
	testutil.NoError(t, err)
	testutil.True(t, ticket.ID != "", "ticket ID should be populated")
	testutil.Equal(t, testTenantID, ticket.TenantID)
	testutil.Equal(t, testUserID, ticket.UserID)
	testutil.Equal(t, "Login broken", ticket.Subject)
	testutil.Equal(t, support.TicketStatusOpen, ticket.Status)
	testutil.Equal(t, support.TicketPriorityHigh, ticket.Priority)
	testutil.True(t, !ticket.CreatedAt.IsZero(), "CreatedAt should be set")

	// GetTicket should return the same ticket.
	got, err := store.GetTicket(ctx, ticket.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, ticket.ID, got.ID)
	testutil.Equal(t, "Login broken", got.Subject)
}

func TestCreateTicketDefaultPriority(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := support.NewStore(sharedPG.Pool)

	// Empty priority should default to "normal".
	ticket, err := store.CreateTicket(ctx, testTenantID, testUserID, "Question", "How do I?", "")
	testutil.NoError(t, err)
	testutil.Equal(t, support.TicketPriorityNormal, ticket.Priority)
}

func TestListTicketsWithFilters(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := support.NewStore(sharedPG.Pool)

	// Create tickets with different priorities.
	_, err := store.CreateTicket(ctx, testTenantID, testUserID, "Urgent bug", "body", support.TicketPriorityUrgent)
	testutil.NoError(t, err)
	_, err = store.CreateTicket(ctx, testTenantID, testUserID2, "Low priority", "body", support.TicketPriorityLow)
	testutil.NoError(t, err)

	// List all tickets for the tenant (no filters).
	tickets, err := store.ListTickets(ctx, testTenantID, support.TicketFilters{})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(tickets))

	// Filter by priority.
	tickets, err = store.ListTickets(ctx, testTenantID, support.TicketFilters{Priority: support.TicketPriorityUrgent})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(tickets))
	testutil.Equal(t, "Urgent bug", tickets[0].Subject)

	// Filter by status (all should be "open" initially).
	tickets, err = store.ListTickets(ctx, testTenantID, support.TicketFilters{Status: support.TicketStatusOpen})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(tickets))
}

func TestUpdateTicket(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := support.NewStore(sharedPG.Pool)

	ticket, err := store.CreateTicket(ctx, testTenantID, testUserID, "Bug report", "details", support.TicketPriorityNormal)
	testutil.NoError(t, err)

	// Update status to in_progress.
	newStatus := support.TicketStatusInProgress
	updated, err := store.UpdateTicket(ctx, ticket.ID, support.TicketUpdate{Status: &newStatus})
	testutil.NoError(t, err)
	testutil.Equal(t, support.TicketStatusInProgress, updated.Status)
	testutil.Equal(t, support.TicketPriorityNormal, updated.Priority) // unchanged

	// Update priority only.
	newPriority := support.TicketPriorityUrgent
	updated, err = store.UpdateTicket(ctx, ticket.ID, support.TicketUpdate{Priority: &newPriority})
	testutil.NoError(t, err)
	testutil.Equal(t, support.TicketPriorityUrgent, updated.Priority)
	testutil.Equal(t, support.TicketStatusInProgress, updated.Status) // unchanged
}

func TestGetTicketNotFound(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := support.NewStore(sharedPG.Pool)

	_, err := store.GetTicket(ctx, "00000000-0000-0000-0000-000000000000")
	testutil.Error(t, err)
}

func TestAddAndListMessages(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := support.NewStore(sharedPG.Pool)

	// Create a ticket (also inserts the initial customer message).
	ticket, err := store.CreateTicket(ctx, testTenantID, testUserID, "Help needed", "Initial message", support.TicketPriorityNormal)
	testutil.NoError(t, err)

	// Add a support reply.
	msg, err := store.AddMessage(ctx, ticket.ID, support.SenderSupport, "We're looking into it")
	testutil.NoError(t, err)
	testutil.True(t, msg.ID != "", "message ID should be populated")
	testutil.Equal(t, ticket.ID, msg.TicketID)
	testutil.Equal(t, support.SenderSupport, msg.SenderType)
	testutil.Equal(t, "We're looking into it", msg.Body)

	// List messages should return both the initial message and the reply.
	messages, err := store.ListMessages(ctx, ticket.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(messages))
	// First message is the customer's initial message (ordered by created_at).
	testutil.Equal(t, support.SenderCustomer, messages[0].SenderType)
	testutil.Equal(t, "Initial message", messages[0].Body)
	testutil.Equal(t, support.SenderSupport, messages[1].SenderType)
}

func TestTicketLifecycle(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := support.NewStore(sharedPG.Pool)

	// Full lifecycle: create -> in_progress -> waiting -> resolved -> closed.
	ticket, err := store.CreateTicket(ctx, testTenantID, testUserID, "Full lifecycle", "start", support.TicketPriorityNormal)
	testutil.NoError(t, err)
	testutil.Equal(t, support.TicketStatusOpen, ticket.Status)

	transitions := []string{
		support.TicketStatusInProgress,
		support.TicketStatusWaitingOnCustomer,
		support.TicketStatusResolved,
		support.TicketStatusClosed,
	}
	for _, nextStatus := range transitions {
		s := nextStatus
		ticket, err = store.UpdateTicket(ctx, ticket.ID, support.TicketUpdate{Status: &s})
		testutil.NoError(t, err)
		testutil.Equal(t, nextStatus, ticket.Status)
	}

	// Closed tickets should not appear in "open" filter.
	tickets, err := store.ListTickets(ctx, testTenantID, support.TicketFilters{Status: support.TicketStatusOpen})
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(tickets))
}
