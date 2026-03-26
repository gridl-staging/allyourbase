// Package backup This file provides a PostgreSQL-backed repository for storing and querying database integrity verification reports.
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

type VerificationReport struct {
	ProjectID  string        `json:"project_id"`
	DatabaseID string        `json:"database_id"`
	Status     string        `json:"status"`
	Checks     []CheckResult `json:"checks"`
	VerifiedAt time.Time     `json:"verified_at"`
}

type IntegrityReportRepo interface {
	Save(ctx context.Context, report VerificationReport) error
	LatestByProject(ctx context.Context, projectID, databaseID string) (*VerificationReport, error)
}

type PgIntegrityReportRepo struct {
	pool *pgxpool.Pool
}

func NewPgIntegrityReportRepo(pool *pgxpool.Pool) IntegrityReportRepo {
	return &PgIntegrityReportRepo{pool: pool}
}

// Save persists a verification report to the database by marshaling its checks to JSON, with created_at automatically set to the current time.
func (r *PgIntegrityReportRepo) Save(ctx context.Context, report VerificationReport) error {
	checksJSON, err := json.Marshal(report.Checks)
	if err != nil {
		return fmt.Errorf("marshaling checks to JSON: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO _ayb_pitr_integrity_reports
			(project_id, database_id, status, checks, verified_at, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())`,
		report.ProjectID, report.DatabaseID, report.Status, checksJSON, report.VerifiedAt,
	)
	if err != nil {
		return fmt.Errorf("saving integrity report: %w", err)
	}
	return nil
}

// LatestByProject retrieves the most recent integrity report for a given project and database combination.
func (r *PgIntegrityReportRepo) LatestByProject(ctx context.Context, projectID, databaseID string) (*VerificationReport, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT project_id, database_id, status, checks, verified_at
		FROM _ayb_pitr_integrity_reports
		WHERE project_id = $1 AND database_id = $2
		ORDER BY verified_at DESC
		LIMIT 1`,
		projectID, databaseID,
	)
	var report VerificationReport
	var checksJSON []byte
	err := row.Scan(
		&report.ProjectID, &report.DatabaseID, &report.Status,
		&checksJSON, &report.VerifiedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting latest integrity report: %w", err)
	}
	if err := json.Unmarshal(checksJSON, &report.Checks); err != nil {
		return nil, fmt.Errorf("unmarshaling checks from JSON: %w", err)
	}
	return &report, nil
}
