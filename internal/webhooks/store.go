package webhooks

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Webhook is a row from _ayb_webhooks.
type Webhook struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Secret    string    `json:"-"`
	Events    []string  `json:"events"`
	Tables    []string  `json:"tables"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// WebhookStore defines the data access interface for webhook CRUD.
type WebhookStore interface {
	List(ctx context.Context) ([]Webhook, error)
	Get(ctx context.Context, id string) (*Webhook, error)
	Create(ctx context.Context, w *Webhook) error
	Update(ctx context.Context, id string, w *Webhook) error
	Delete(ctx context.Context, id string) error
}

// WebhookLister loads enabled webhook definitions for the dispatcher.
type WebhookLister interface {
	ListEnabled(ctx context.Context) ([]Webhook, error)
}

// Delivery is a row from _ayb_webhook_deliveries.
type Delivery struct {
	ID           string    `json:"id"`
	WebhookID    string    `json:"webhookId"`
	EventAction  string    `json:"eventAction"`
	EventTable   string    `json:"eventTable"`
	Success      bool      `json:"success"`
	StatusCode   int       `json:"statusCode,omitempty"`
	Attempt      int       `json:"attempt"`
	DurationMs   int       `json:"durationMs"`
	Error        string    `json:"error,omitempty"`
	RequestBody  string    `json:"requestBody,omitempty"`
	ResponseBody string    `json:"responseBody,omitempty"`
	DeliveredAt  time.Time `json:"deliveredAt"`
}

// DeliveryStore persists and queries webhook delivery logs.
type DeliveryStore interface {
	RecordDelivery(ctx context.Context, d *Delivery) error
	ListDeliveries(ctx context.Context, webhookID string, page, perPage int) ([]Delivery, int, error)
	GetDelivery(ctx context.Context, webhookID, deliveryID string) (*Delivery, error)
	PruneDeliveries(ctx context.Context, olderThan time.Duration) (int64, error)
}

// Store handles CRUD operations on _ayb_webhooks.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new webhook Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const columns = "id, url, secret, events, tables, enabled, created_at, updated_at"

func scanWebhook(row pgx.Row) (*Webhook, error) {
	var w Webhook
	err := row.Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Tables, &w.Enabled, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *Store) List(ctx context.Context) ([]Webhook, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" FROM _ayb_webhooks ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Webhook
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Tables, &w.Enabled, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	if result == nil {
		result = []Webhook{}
	}
	return result, rows.Err()
}

func (s *Store) Get(ctx context.Context, id string) (*Webhook, error) {
	row := s.pool.QueryRow(ctx, "SELECT "+columns+" FROM _ayb_webhooks WHERE id = $1", id)
	return scanWebhook(row)
}

func (s *Store) Create(ctx context.Context, w *Webhook) error {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_webhooks (url, secret, events, tables, enabled)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at, updated_at`,
		w.URL, w.Secret, w.Events, w.Tables, w.Enabled,
	)
	return row.Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt)
}

func (s *Store) Update(ctx context.Context, id string, w *Webhook) error {
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_webhooks
		 SET url = $1, secret = $2, events = $3, tables = $4, enabled = $5, updated_at = NOW()
		 WHERE id = $6
		 RETURNING id, created_at, updated_at`,
		w.URL, w.Secret, w.Events, w.Tables, w.Enabled, id,
	)
	return row.Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt)
}

func (s *Store) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM _ayb_webhooks WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ListEnabled(ctx context.Context) ([]Webhook, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" FROM _ayb_webhooks WHERE enabled = true")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Webhook
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Tables, &w.Enabled, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

// --- Delivery log methods ---

const deliveryColumns = "id, webhook_id, event_action, event_table, success, status_code, attempt, duration_ms, error, request_body, response_body, delivered_at"

func scanDelivery(row pgx.Row) (*Delivery, error) {
	var d Delivery
	err := row.Scan(&d.ID, &d.WebhookID, &d.EventAction, &d.EventTable,
		&d.Success, &d.StatusCode, &d.Attempt, &d.DurationMs,
		&d.Error, &d.RequestBody, &d.ResponseBody, &d.DeliveredAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) RecordDelivery(ctx context.Context, d *Delivery) error {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_webhook_deliveries
		 (webhook_id, event_action, event_table, success, status_code, attempt, duration_ms, error, request_body, response_body)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, delivered_at`,
		d.WebhookID, d.EventAction, d.EventTable, d.Success, d.StatusCode,
		d.Attempt, d.DurationMs, d.Error, d.RequestBody, d.ResponseBody,
	)
	return row.Scan(&d.ID, &d.DeliveredAt)
}

func (s *Store) ListDeliveries(ctx context.Context, webhookID string, page, perPage int) ([]Delivery, int, error) {
	// Count total.
	var total int
	err := s.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_webhook_deliveries WHERE webhook_id = $1",
		webhookID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	rows, err := s.pool.Query(ctx,
		"SELECT "+deliveryColumns+" FROM _ayb_webhook_deliveries WHERE webhook_id = $1 ORDER BY delivered_at DESC LIMIT $2 OFFSET $3",
		webhookID, perPage, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.EventAction, &d.EventTable,
			&d.Success, &d.StatusCode, &d.Attempt, &d.DurationMs,
			&d.Error, &d.RequestBody, &d.ResponseBody, &d.DeliveredAt); err != nil {
			return nil, 0, err
		}
		result = append(result, d)
	}
	if result == nil {
		result = []Delivery{}
	}
	return result, total, rows.Err()
}

func (s *Store) GetDelivery(ctx context.Context, webhookID, deliveryID string) (*Delivery, error) {
	row := s.pool.QueryRow(ctx,
		"SELECT "+deliveryColumns+" FROM _ayb_webhook_deliveries WHERE id = $1 AND webhook_id = $2",
		deliveryID, webhookID,
	)
	return scanDelivery(row)
}

func (s *Store) PruneDeliveries(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		"DELETE FROM _ayb_webhook_deliveries WHERE delivered_at < NOW() - $1::interval",
		olderThan.String(),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
