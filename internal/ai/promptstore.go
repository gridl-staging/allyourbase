// Package ai promptstore.go implements a PostgreSQL-backed prompt store with CRUD operations and version history for managing AI prompt templates.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Prompt represents a managed AI prompt template.
type Prompt struct {
	ID          uuid.UUID        `json:"id"`
	Name        string           `json:"name"`
	Version     int              `json:"version"`
	Template    string           `json:"template"`
	Variables   []PromptVariable `json:"variables"`
	Model       string           `json:"model,omitempty"`
	Provider    string           `json:"provider,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// PromptVariable defines a variable placeholder in a prompt template.
type PromptVariable struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
}

// PromptVersion records a historical version of a prompt.
type PromptVersion struct {
	ID        uuid.UUID        `json:"id"`
	PromptID  uuid.UUID        `json:"prompt_id"`
	Version   int              `json:"version"`
	Template  string           `json:"template"`
	Variables []PromptVariable `json:"variables"`
	CreatedAt time.Time        `json:"created_at"`
}

// CreatePromptRequest holds fields for creating a new prompt.
type CreatePromptRequest struct {
	Name        string           `json:"name"`
	Template    string           `json:"template"`
	Variables   []PromptVariable `json:"variables"`
	Model       string           `json:"model,omitempty"`
	Provider    string           `json:"provider,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
}

// UpdatePromptRequest holds fields for updating an existing prompt.
type UpdatePromptRequest struct {
	Template    *string           `json:"template,omitempty"`
	Variables   *[]PromptVariable `json:"variables,omitempty"`
	Model       *string           `json:"model,omitempty"`
	Provider    *string           `json:"provider,omitempty"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
}

// PromptStore defines operations for prompt CRUD and version history.
type PromptStore interface {
	Create(ctx context.Context, req CreatePromptRequest) (Prompt, error)
	Get(ctx context.Context, id uuid.UUID) (Prompt, error)
	GetByName(ctx context.Context, name string) (Prompt, error)
	List(ctx context.Context, page, perPage int) ([]Prompt, int, error)
	Update(ctx context.Context, id uuid.UUID, req UpdatePromptRequest) (Prompt, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListVersions(ctx context.Context, promptID uuid.UUID) ([]PromptVersion, error)
}

// PgPromptStore implements PromptStore backed by PostgreSQL.
type PgPromptStore struct {
	pool *pgxpool.Pool
}

// NewPgPromptStore creates a PostgreSQL-backed prompt store.
func NewPgPromptStore(pool *pgxpool.Pool) *PgPromptStore {
	return &PgPromptStore{pool: pool}
}

// Create inserts a new prompt with the given request parameters and returns the created Prompt with assigned ID, version, and timestamps.
func (s *PgPromptStore) Create(ctx context.Context, req CreatePromptRequest) (Prompt, error) {
	varsJSON, err := json.Marshal(req.Variables)
	if err != nil {
		return Prompt{}, fmt.Errorf("marshal variables: %w", err)
	}

	var p Prompt
	var varsBytes []byte
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_ai_prompts (name, template, variables, model, provider, max_tokens, temperature)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, name, version, template, variables, model, provider, max_tokens, temperature, created_at, updated_at`,
		req.Name, req.Template, varsJSON, req.Model, req.Provider, req.MaxTokens, req.Temperature,
	).Scan(&p.ID, &p.Name, &p.Version, &p.Template, &varsBytes, &p.Model, &p.Provider, &p.MaxTokens, &p.Temperature, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Prompt{}, fmt.Errorf("creating prompt: %w", err)
	}
	_ = json.Unmarshal(varsBytes, &p.Variables)
	return p, nil
}

func (s *PgPromptStore) Get(ctx context.Context, id uuid.UUID) (Prompt, error) {
	return s.scanPrompt(ctx,
		`SELECT id, name, version, template, variables, model, provider, max_tokens, temperature, created_at, updated_at
		 FROM _ayb_ai_prompts WHERE id = $1`, id)
}

func (s *PgPromptStore) GetByName(ctx context.Context, name string) (Prompt, error) {
	return s.scanPrompt(ctx,
		`SELECT id, name, version, template, variables, model, provider, max_tokens, temperature, created_at, updated_at
		 FROM _ayb_ai_prompts WHERE name = $1`, name)
}

// scanPrompt executes the given query and scans a single row into a Prompt, unmarshaling the JSON variables. Returns an error if the prompt is not found or if scanning fails.
func (s *PgPromptStore) scanPrompt(ctx context.Context, query string, args ...any) (Prompt, error) {
	var p Prompt
	var varsBytes []byte
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&p.ID, &p.Name, &p.Version, &p.Template, &varsBytes,
		&p.Model, &p.Provider, &p.MaxTokens, &p.Temperature, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return Prompt{}, fmt.Errorf("prompt not found")
	}
	if err != nil {
		return Prompt{}, fmt.Errorf("querying prompt: %w", err)
	}
	_ = json.Unmarshal(varsBytes, &p.Variables)
	return p, nil
}

// List returns a paginated list of prompts ordered by name along with the total count. It defaults to page 1 and 20 items per page if invalid page or perPage values are provided.
func (s *PgPromptStore) List(ctx context.Context, page, perPage int) ([]Prompt, int, error) {
	if perPage <= 0 {
		perPage = 20
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	var total int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM _ayb_ai_prompts").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting prompts: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, name, version, template, variables, model, provider, max_tokens, temperature, created_at, updated_at
		 FROM _ayb_ai_prompts ORDER BY name LIMIT $1 OFFSET $2`, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing prompts: %w", err)
	}
	defer rows.Close()

	var prompts []Prompt
	for rows.Next() {
		var p Prompt
		var varsBytes []byte
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Template, &varsBytes,
			&p.Model, &p.Provider, &p.MaxTokens, &p.Temperature, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning prompt: %w", err)
		}
		_ = json.Unmarshal(varsBytes, &p.Variables)
		prompts = append(prompts, p)
	}
	return prompts, total, rows.Err()
}

// Update modifies a prompt by ID, merging the provided partial update fields with the current values and incrementing the version number.
func (s *PgPromptStore) Update(ctx context.Context, id uuid.UUID, req UpdatePromptRequest) (Prompt, error) {
	// Fetch current to merge partial updates.
	current, err := s.Get(ctx, id)
	if err != nil {
		return Prompt{}, err
	}

	template := current.Template
	if req.Template != nil {
		template = *req.Template
	}
	vars := current.Variables
	if req.Variables != nil {
		vars = *req.Variables
	}
	model := current.Model
	if req.Model != nil {
		model = *req.Model
	}
	provider := current.Provider
	if req.Provider != nil {
		provider = *req.Provider
	}
	maxTokens := current.MaxTokens
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}
	temperature := current.Temperature
	if req.Temperature != nil {
		temperature = req.Temperature
	}

	varsJSON, _ := json.Marshal(vars)

	var p Prompt
	var varsBytes []byte
	err = s.pool.QueryRow(ctx,
		`UPDATE _ayb_ai_prompts
		 SET template = $2, variables = $3, model = $4, provider = $5, max_tokens = $6, temperature = $7,
		     version = version + 1, updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, name, version, template, variables, model, provider, max_tokens, temperature, created_at, updated_at`,
		id, template, varsJSON, model, provider, maxTokens, temperature,
	).Scan(&p.ID, &p.Name, &p.Version, &p.Template, &varsBytes,
		&p.Model, &p.Provider, &p.MaxTokens, &p.Temperature, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Prompt{}, fmt.Errorf("updating prompt: %w", err)
	}
	_ = json.Unmarshal(varsBytes, &p.Variables)
	return p, nil
}

func (s *PgPromptStore) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM _ayb_ai_prompts WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("deleting prompt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("prompt not found")
	}
	return nil
}

// ListVersions retrieves the historical versions of a prompt ordered by version descending.
func (s *PgPromptStore) ListVersions(ctx context.Context, promptID uuid.UUID) ([]PromptVersion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, prompt_id, version, template, variables, created_at
		 FROM _ayb_ai_prompt_versions WHERE prompt_id = $1 ORDER BY version DESC`, promptID)
	if err != nil {
		return nil, fmt.Errorf("listing prompt versions: %w", err)
	}
	defer rows.Close()

	var versions []PromptVersion
	for rows.Next() {
		var v PromptVersion
		var varsBytes []byte
		if err := rows.Scan(&v.ID, &v.PromptID, &v.Version, &v.Template, &varsBytes, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning prompt version: %w", err)
		}
		_ = json.Unmarshal(varsBytes, &v.Variables)
		versions = append(versions, v)
	}
	return versions, rows.Err()
}
