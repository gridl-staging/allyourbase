// Package support Store persists support tickets and messages in Postgres.
package support

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrTicketNotFound indicates no support ticket exists for the requested ID.
	ErrTicketNotFound = errors.New("support ticket not found")
)

type supportDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Store persists support tickets and messages in Postgres.
type Store struct {
	db supportDB
}

const (
	ticketColumns  = `id, tenant_id, user_id, subject, status, priority, created_at, updated_at`
	messageColumns = `id, ticket_id, sender_type, body, created_at`
)

// NewStore creates a support store backed by a pgx pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{db: pool}
}

// NewStoreWithDB allows tests to inject a lightweight DB facade.
func NewStoreWithDB(db supportDB) *Store {
	return &Store{db: db}
}

// CreateTicket creates a ticket and first customer message atomically.
func (s *Store) CreateTicket(ctx context.Context, tenantID, userID, subject, body, priority string) (*Ticket, error) {
	if priority == "" {
		priority = TicketPriorityNormal
	}

	row := s.db.QueryRow(ctx,
		`WITH new_ticket AS (
			INSERT INTO _ayb_support_tickets (tenant_id, user_id, subject, priority)
			VALUES ($1, $2, $3, $4)
			RETURNING `+ticketColumns+`
		), initial_message AS (
			INSERT INTO _ayb_support_messages (ticket_id, sender_type, body)
			SELECT id, 'customer', $5 FROM new_ticket
		)
		SELECT `+ticketColumns+` FROM new_ticket`,
		tenantID, userID, subject, priority, body,
	)

	ticket, err := scanTicketRow(row)
	if err != nil {
		return nil, fmt.Errorf("create support ticket: %w", err)
	}
	return ticket, nil
}

// ListTickets returns tickets ordered by newest first, filtered by optional criteria.
func (s *Store) ListTickets(ctx context.Context, tenantID string, filters TicketFilters) ([]Ticket, error) {
	var (
		where []string
		args  []any
	)

	argPos := 1
	effectiveTenantID := tenantID
	if effectiveTenantID == "" {
		effectiveTenantID = filters.TenantID
	}
	if effectiveTenantID != "" {
		where = append(where, fmt.Sprintf("tenant_id = $%d", argPos))
		args = append(args, effectiveTenantID)
		argPos++
	}
	if filters.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argPos))
		args = append(args, filters.Status)
		argPos++
	}
	if filters.Priority != "" {
		where = append(where, fmt.Sprintf("priority = $%d", argPos))
		args = append(args, filters.Priority)
		argPos++
	}

	query := `SELECT ` + ticketColumns + ` FROM _ayb_support_tickets`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list support tickets: %w", err)
	}
	defer rows.Close()

	items := make([]Ticket, 0)
	for rows.Next() {
		ticket, scanErr := scanTicketRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, ticket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate support tickets: %w", err)
	}

	return items, nil
}

// GetTicket returns one ticket by ID.
func (s *Store) GetTicket(ctx context.Context, ticketID string) (*Ticket, error) {
	row := s.db.QueryRow(ctx,
		`SELECT `+ticketColumns+` FROM _ayb_support_tickets WHERE id = $1`,
		ticketID,
	)

	ticket, err := scanTicketRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("get support ticket: %w", err)
	}
	return ticket, nil
}

// UpdateTicket updates status/priority and bumps updated_at.
func (s *Store) UpdateTicket(ctx context.Context, ticketID string, updates TicketUpdate) (*Ticket, error) {
	if updates.Status == nil && updates.Priority == nil {
		return nil, ErrNoTicketUpdates
	}

	setParts := make([]string, 0, 3)
	args := []any{ticketID}
	argPos := 2

	if updates.Status != nil {
		setParts = append(setParts, fmt.Sprintf("status = $%d", argPos))
		args = append(args, *updates.Status)
		argPos++
	}
	if updates.Priority != nil {
		setParts = append(setParts, fmt.Sprintf("priority = $%d", argPos))
		args = append(args, *updates.Priority)
		argPos++
	}
	setParts = append(setParts, "updated_at = NOW()")

	row := s.db.QueryRow(ctx,
		fmt.Sprintf(
			`UPDATE _ayb_support_tickets
			 SET %s
			 WHERE id = $1
			 RETURNING %s`,
			strings.Join(setParts, ", "),
			ticketColumns,
		),
		args...,
	)

	ticket, err := scanTicketRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("update support ticket: %w", err)
	}
	return ticket, nil
}

// AddMessage creates a message and bumps parent ticket updated_at atomically.
func (s *Store) AddMessage(ctx context.Context, ticketID, senderType, body string) (*Message, error) {
	row := s.db.QueryRow(ctx,
		`WITH updated_ticket AS (
			UPDATE _ayb_support_tickets
			SET updated_at = NOW()
			WHERE id = $1
			RETURNING id
		), inserted_message AS (
			INSERT INTO _ayb_support_messages (ticket_id, sender_type, body)
			SELECT id, $2, $3 FROM updated_ticket
			RETURNING `+messageColumns+`
		)
		SELECT `+messageColumns+` FROM inserted_message`,
		ticketID, senderType, body,
	)

	message, err := scanMessageRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("add support message: %w", err)
	}
	return message, nil
}

// ListMessages returns ticket messages oldest-first.
func (s *Store) ListMessages(ctx context.Context, ticketID string) ([]Message, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+messageColumns+`
		 FROM _ayb_support_messages
		 WHERE ticket_id = $1
		 ORDER BY created_at ASC`,
		ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("list support messages: %w", err)
	}
	defer rows.Close()

	items := make([]Message, 0)
	for rows.Next() {
		msg, scanErr := scanMessageRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate support messages: %w", err)
	}

	return items, nil
}

// scanTicketRow scans a database row into a Ticket pointer.
func scanTicketRow(row pgx.Row) (*Ticket, error) {
	var ticket Ticket
	if err := row.Scan(
		&ticket.ID,
		&ticket.TenantID,
		&ticket.UserID,
		&ticket.Subject,
		&ticket.Status,
		&ticket.Priority,
		&ticket.CreatedAt,
		&ticket.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &ticket, nil
}

// scanTicketRows scans a row from a Rows iterator into a Ticket.
func scanTicketRows(rows pgx.Rows) (Ticket, error) {
	var ticket Ticket
	if err := rows.Scan(
		&ticket.ID,
		&ticket.TenantID,
		&ticket.UserID,
		&ticket.Subject,
		&ticket.Status,
		&ticket.Priority,
		&ticket.CreatedAt,
		&ticket.UpdatedAt,
	); err != nil {
		return Ticket{}, fmt.Errorf("scan support ticket row: %w", err)
	}
	return ticket, nil
}

func scanMessageRow(row pgx.Row) (*Message, error) {
	var message Message
	if err := row.Scan(
		&message.ID,
		&message.TicketID,
		&message.SenderType,
		&message.Body,
		&message.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &message, nil
}

func scanMessageRows(rows pgx.Rows) (Message, error) {
	var message Message
	if err := rows.Scan(
		&message.ID,
		&message.TicketID,
		&message.SenderType,
		&message.Body,
		&message.CreatedAt,
	); err != nil {
		return Message{}, fmt.Errorf("scan support message row: %w", err)
	}
	return message, nil
}
