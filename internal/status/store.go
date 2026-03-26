// Package status Store implements PostgreSQL-backed incident storage with methods for creating, retrieving, updating incidents and managing their timeline updates.
package status

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
	// ErrIncidentNotFound indicates the incident does not exist.
	ErrIncidentNotFound = errors.New("incident not found")
)

const incidentColumns = `id, title, status, affected_services, created_at, updated_at, resolved_at`

// IncidentStore defines incident persistence required by status handlers.
type IncidentStore interface {
	CreateIncident(ctx context.Context, incident *Incident) error
	GetIncident(ctx context.Context, id string) (*Incident, error)
	ListIncidents(ctx context.Context, activeOnly bool) ([]Incident, error)
	UpdateIncident(ctx context.Context, id string, update *IncidentUpdate) error
	AddIncidentUpdate(ctx context.Context, incidentID string, update *IncidentUpdateEntry) error
}

// PgIncidentStore is a PostgreSQL-backed incident store.
type PgIncidentStore struct {
	pool *pgxpool.Pool
}

func NewPgIncidentStore(pool *pgxpool.Pool) *PgIncidentStore {
	return &PgIncidentStore{pool: pool}
}

// CreateIncident inserts a new incident into the database with the given title, status, and affected services. It validates required fields, sets the default status to investigating if unspecified, and returns the incident with generated ID and timestamps.
func (s *PgIncidentStore) CreateIncident(ctx context.Context, incident *Incident) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("incident store not configured")
	}
	if incident == nil {
		return fmt.Errorf("incident is required")
	}
	incident.Title = strings.TrimSpace(incident.Title)
	if incident.Title == "" {
		return fmt.Errorf("incident title is required")
	}
	if incident.Status == "" {
		incident.Status = IncidentInvestigating
	}
	if !incident.Status.IsValid() {
		return fmt.Errorf("invalid incident status %q", incident.Status)
	}

	affected := incident.AffectedServices
	if affected == nil {
		affected = []string{}
	}

	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_incidents (title, status, affected_services)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at, updated_at, resolved_at`,
		incident.Title,
		string(incident.Status),
		affected,
	).Scan(&incident.ID, &incident.CreatedAt, &incident.UpdatedAt, &incident.ResolvedAt)
	if err != nil {
		return fmt.Errorf("create incident: %w", err)
	}

	incident.AffectedServices = affected
	incident.Updates = []IncidentUpdateEntry{}
	return nil
}

// GetIncident retrieves an incident by ID along with all its timeline updates. It returns ErrIncidentNotFound if the incident does not exist.
func (s *PgIncidentStore) GetIncident(ctx context.Context, id string) (*Incident, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("incident store not configured")
	}

	incident, err := scanIncident(s.pool.QueryRow(ctx,
		`SELECT `+incidentColumns+` FROM _ayb_incidents WHERE id = $1`,
		id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrIncidentNotFound
		}
		return nil, fmt.Errorf("get incident: %w", err)
	}

	incident.Updates, err = s.listIncidentUpdates(ctx, incident.ID)
	if err != nil {
		return nil, err
	}
	return incident, nil
}

// ListIncidents retrieves all incidents from the database ordered by creation time descending. If activeOnly is true, it returns only incidents with status other than resolved.
func (s *PgIncidentStore) ListIncidents(ctx context.Context, activeOnly bool) ([]Incident, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("incident store not configured")
	}

	query := `SELECT ` + incidentColumns + ` FROM _ayb_incidents`
	args := []any{}
	if activeOnly {
		query += ` WHERE status <> 'resolved'`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()

	incidents := make([]Incident, 0)
	for rows.Next() {
		incident, err := scanIncident(rows)
		if err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}
		incident.Updates, err = s.listIncidentUpdates(ctx, incident.ID)
		if err != nil {
			return nil, err
		}
		incidents = append(incidents, *incident)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incidents: %w", err)
	}
	if incidents == nil {
		return []Incident{}, nil
	}
	return incidents, nil
}

// UpdateIncident modifies an incident's title and/or status. When status changes to resolved, resolved_at is automatically set to the provided time or current time if not specified.
func (s *PgIncidentStore) UpdateIncident(ctx context.Context, id string, update *IncidentUpdate) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("incident store not configured")
	}
	if update == nil {
		return fmt.Errorf("incident update is required")
	}

	var title *string
	if update.Title != nil {
		trimmed := strings.TrimSpace(*update.Title)
		title = &trimmed
	}

	var status *string
	if update.Status != nil {
		if !update.Status.IsValid() {
			return fmt.Errorf("invalid incident status %q", *update.Status)
		}
		sv := string(*update.Status)
		status = &sv
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_incidents
		 SET title = COALESCE($2, title),
		     status = COALESCE($3, status),
		     resolved_at = CASE
		         WHEN $3::text IS NULL THEN resolved_at
		         WHEN $3::text = 'resolved' THEN COALESCE(resolved_at, COALESCE($4, NOW()))
		         ELSE NULL
		     END,
		     updated_at = NOW()
		 WHERE id = $1`,
		id,
		title,
		status,
		update.ResolvedAt,
	)
	if err != nil {
		return fmt.Errorf("update incident: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrIncidentNotFound
	}
	return nil
}

// AddIncidentUpdate appends a timeline entry to an incident and atomically updates the parent incident status within a transaction. This ensures the incident status always reflects the latest timeline progression.
func (s *PgIncidentStore) AddIncidentUpdate(ctx context.Context, incidentID string, update *IncidentUpdateEntry) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("incident store not configured")
	}
	if update == nil {
		return fmt.Errorf("incident update entry is required")
	}
	update.Message = strings.TrimSpace(update.Message)
	if update.Message == "" {
		return fmt.Errorf("incident update message is required")
	}
	if !update.Status.IsValid() {
		return fmt.Errorf("invalid incident update status %q", update.Status)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin add incident update tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	err = tx.QueryRow(ctx,
		`INSERT INTO _ayb_incident_updates (incident_id, message, status)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		incidentID,
		update.Message,
		string(update.Status),
	).Scan(&update.ID, &update.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrIncidentNotFound
		}
		return fmt.Errorf("add incident update: %w", err)
	}

	// Keep parent status synchronized with timeline progression atomically.
	tag, err := tx.Exec(ctx,
		`UPDATE _ayb_incidents
		 SET status = $2,
		     resolved_at = CASE WHEN $2::text = 'resolved' THEN COALESCE(resolved_at, NOW()) ELSE NULL END,
		     updated_at = NOW()
		 WHERE id = $1`,
		incidentID,
		string(update.Status),
	)
	if err != nil {
		return fmt.Errorf("update parent incident status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrIncidentNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit add incident update tx: %w", err)
	}
	update.IncidentID = incidentID
	return nil
}

// listIncidentUpdates retrieves all timeline updates for an incident, ordered chronologically by creation time.
func (s *PgIncidentStore) listIncidentUpdates(ctx context.Context, incidentID string) ([]IncidentUpdateEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, incident_id, message, status, created_at
		 FROM _ayb_incident_updates
		 WHERE incident_id = $1
		 ORDER BY created_at ASC`,
		incidentID,
	)
	if err != nil {
		return nil, fmt.Errorf("list incident updates: %w", err)
	}
	defer rows.Close()

	updates := make([]IncidentUpdateEntry, 0)
	for rows.Next() {
		var entry IncidentUpdateEntry
		var statusValue string
		if err := rows.Scan(&entry.ID, &entry.IncidentID, &entry.Message, &statusValue, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan incident update: %w", err)
		}
		entry.Status = IncidentStatus(statusValue)
		updates = append(updates, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incident updates: %w", err)
	}
	if updates == nil {
		return []IncidentUpdateEntry{}, nil
	}
	return updates, nil
}

// scanIncident unmarshals a database row into an Incident struct. It converts the status string value and ensures affected services is an empty slice rather than nil.
func scanIncident(row pgx.Row) (*Incident, error) {
	var incident Incident
	var statusValue string
	if err := row.Scan(
		&incident.ID,
		&incident.Title,
		&statusValue,
		&incident.AffectedServices,
		&incident.CreatedAt,
		&incident.UpdatedAt,
		&incident.ResolvedAt,
	); err != nil {
		return nil, err
	}
	incident.Status = IncidentStatus(statusValue)
	if incident.AffectedServices == nil {
		incident.AffectedServices = []string{}
	}
	return &incident, nil
}
