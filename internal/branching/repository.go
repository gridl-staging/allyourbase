package branching

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Branch status constants.
const (
	StatusCreating = "creating"
	StatusReady    = "ready"
	StatusFailed   = "failed"
	StatusDeleting = "deleting"
)

// BranchRecord represents a row in _ayb_branches.
type BranchRecord struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	SourceDatabase string    `json:"source_database"`
	BranchDatabase string    `json:"branch_database"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ErrorMessage   string    `json:"error_message,omitempty"`
}

// Repo is the branch metadata repository interface.
type Repo interface {
	Create(ctx context.Context, name, sourceDB, branchDB string) (*BranchRecord, error)
	Get(ctx context.Context, name string) (*BranchRecord, error)
	GetByID(ctx context.Context, id string) (*BranchRecord, error)
	List(ctx context.Context) ([]BranchRecord, error)
	UpdateStatus(ctx context.Context, id, status, errMsg string) error
	Delete(ctx context.Context, id string) error
}

// PgRepo implements Repo with PostgreSQL.
type PgRepo struct {
	pool *pgxpool.Pool
}

// NewPgRepo creates a PgRepo.
func NewPgRepo(pool *pgxpool.Pool) Repo {
	return &PgRepo{pool: pool}
}

const branchColumns = "id, name, source_database, branch_database, status, created_at, updated_at, error_message"

func scanBranch(row pgx.Row) (*BranchRecord, error) {
	var rec BranchRecord
	err := row.Scan(
		&rec.ID, &rec.Name, &rec.SourceDatabase, &rec.BranchDatabase,
		&rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &rec.ErrorMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning branch record: %w", err)
	}
	return &rec, nil
}

func scanBranchRows(rows pgx.Rows) (*BranchRecord, error) {
	var rec BranchRecord
	err := rows.Scan(
		&rec.ID, &rec.Name, &rec.SourceDatabase, &rec.BranchDatabase,
		&rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &rec.ErrorMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning branch row: %w", err)
	}
	return &rec, nil
}

// Create inserts a new branch record with status "creating".
func (r *PgRepo) Create(ctx context.Context, name, sourceDB, branchDB string) (*BranchRecord, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO _ayb_branches (name, source_database, branch_database, status)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+branchColumns,
		name, sourceDB, branchDB, StatusCreating,
	)
	return scanBranch(row)
}

// Get retrieves a branch by name.
func (r *PgRepo) Get(ctx context.Context, name string) (*BranchRecord, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+branchColumns+" FROM _ayb_branches WHERE name = $1", name,
	)
	rec, err := scanBranch(row)
	if err != nil {
		if err.Error() == "scanning branch record: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return rec, nil
}

// GetByID retrieves a branch by ID.
func (r *PgRepo) GetByID(ctx context.Context, id string) (*BranchRecord, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+branchColumns+" FROM _ayb_branches WHERE id = $1", id,
	)
	rec, err := scanBranch(row)
	if err != nil {
		if err.Error() == "scanning branch record: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return rec, nil
}

// List returns all branches sorted by created_at.
func (r *PgRepo) List(ctx context.Context) ([]BranchRecord, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+branchColumns+" FROM _ayb_branches ORDER BY created_at ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("listing branches: %w", err)
	}
	defer rows.Close()

	var records []BranchRecord
	for rows.Next() {
		rec, err := scanBranchRows(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, rows.Err()
}

// UpdateStatus sets the status and error_message for a branch.
func (r *PgRepo) UpdateStatus(ctx context.Context, id, status, errMsg string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE _ayb_branches SET status = $2, error_message = $3, updated_at = NOW()
		 WHERE id = $1`,
		id, status, errMsg,
	)
	if err != nil {
		return fmt.Errorf("updating branch status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("branch record %q not found", id)
	}
	return nil
}

// Delete removes a branch record by ID.
func (r *PgRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx,
		"DELETE FROM _ayb_branches WHERE id = $1", id,
	)
	if err != nil {
		return fmt.Errorf("deleting branch record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("branch record %q not found", id)
	}
	return nil
}
