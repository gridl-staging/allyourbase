package sites

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// EnsureDeployUploading verifies that the requested deploy exists for the site
// and is currently in the "uploading" state.
func (s *Service) EnsureDeployUploading(ctx context.Context, siteID, deployID string) error {
	deploy, err := s.GetDeploy(ctx, siteID, deployID)
	if err != nil {
		return err
	}
	if deploy.Status != StatusUploading {
		return fmt.Errorf("%w: deploy status %q does not accept file uploads", ErrInvalidTransition, deploy.Status)
	}
	return nil
}

// RecordDeployFileUpload increments deploy file counters after a successful upload.
func (s *Service) RecordDeployFileUpload(ctx context.Context, siteID, deployID string, fileSize int64) (*Deploy, error) {
	if fileSize < 0 {
		return nil, fmt.Errorf("file size must be non-negative")
	}

	var deploy Deploy
	err := s.pool.QueryRow(ctx,
		`UPDATE _ayb_deploys
		    SET file_count = file_count + 1,
		        total_bytes = total_bytes + $3,
		        updated_at = now()
		  WHERE id = $1 AND site_id = $2 AND status = 'uploading'
		  RETURNING id, site_id, status, file_count, total_bytes, error_message, created_at, updated_at`,
		deployID, siteID, fileSize,
	).Scan(
		&deploy.ID, &deploy.SiteID, &deploy.Status, &deploy.FileCount, &deploy.TotalBytes,
		&deploy.ErrorMessage, &deploy.CreatedAt, &deploy.UpdatedAt,
	)
	if err == nil {
		return &deploy, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("record deploy upload: %w", err)
	}

	return nil, s.deployStatusError(
		ctx,
		siteID,
		deployID,
		"deploy status %q does not accept file uploads",
	)
}

// CreateDeploy inserts a new deploy for the given site with status "uploading".
func (s *Service) CreateDeploy(ctx context.Context, siteID string) (*Deploy, error) {
	// Verify site exists first.
	if _, err := s.GetSite(ctx, siteID); err != nil {
		return nil, err
	}

	var d Deploy
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_deploys (site_id, status)
		 VALUES ($1, 'uploading')
		 RETURNING id, site_id, status, file_count, total_bytes, error_message, created_at, updated_at`,
		siteID,
	).Scan(&d.ID, &d.SiteID, &d.Status, &d.FileCount, &d.TotalBytes,
		&d.ErrorMessage, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create deploy: %w", err)
	}
	return &d, nil
}

// GetDeploy retrieves a deploy by ID.
func (s *Service) GetDeploy(ctx context.Context, siteID, deployID string) (*Deploy, error) {
	var d Deploy
	err := s.pool.QueryRow(ctx,
		`SELECT id, site_id, status, file_count, total_bytes, error_message, created_at, updated_at
		 FROM _ayb_deploys
		 WHERE id = $1 AND site_id = $2`,
		deployID, siteID,
	).Scan(&d.ID, &d.SiteID, &d.Status, &d.FileCount, &d.TotalBytes,
		&d.ErrorMessage, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDeployNotFound
		}
		return nil, fmt.Errorf("get deploy: %w", err)
	}
	return &d, nil
}

// ListDeploys returns a paginated list of deploys for a site.
func (s *Service) ListDeploys(ctx context.Context, siteID string, page, perPage int) (*DeployListResult, error) {
	page, perPage = clampPagination(page, perPage)
	offset := (page - 1) * perPage

	var totalCount int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM _ayb_deploys WHERE site_id = $1`, siteID,
	).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("count deploys: %w", err)
	}
	if totalCount == 0 {
		if _, err := s.GetSite(ctx, siteID); err != nil {
			return nil, err
		}
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, site_id, status, file_count, total_bytes, error_message, created_at, updated_at
		 FROM _ayb_deploys
		 WHERE site_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`, siteID, perPage, offset)
	if err != nil {
		return nil, fmt.Errorf("list deploys: %w", err)
	}
	defer rows.Close()

	var deploys []Deploy
	for rows.Next() {
		var d Deploy
		if err := rows.Scan(&d.ID, &d.SiteID, &d.Status, &d.FileCount, &d.TotalBytes,
			&d.ErrorMessage, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan deploy: %w", err)
		}
		deploys = append(deploys, d)
	}
	if deploys == nil {
		deploys = []Deploy{}
	}

	return &DeployListResult{
		Deploys:    deploys,
		TotalCount: totalCount,
		Page:       page,
		PerPage:    perPage,
	}, nil
}

// PromoteDeploy atomically transitions the target deploy to "live" and any
// existing live deploy to "superseded", all in a single transaction.
// Only deploys with status "uploading" or "superseded" can be promoted.
func (s *Service) PromoteDeploy(ctx context.Context, siteID, deployID string) (*Deploy, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin promote tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Verify the target deploy exists and is promotable.
	var currentStatus DeployStatus
	err = tx.QueryRow(ctx,
		`SELECT status FROM _ayb_deploys WHERE id = $1 AND site_id = $2 FOR UPDATE`,
		deployID, siteID,
	).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDeployNotFound
		}
		return nil, fmt.Errorf("lock deploy: %w", err)
	}
	if currentStatus != StatusUploading && currentStatus != StatusSuperseded {
		return nil, fmt.Errorf("%w: cannot promote deploy with status %q", ErrInvalidTransition, currentStatus)
	}

	// Supersede current live deploy if one exists.
	_, err = tx.Exec(ctx,
		`UPDATE _ayb_deploys SET status = 'superseded', updated_at = now()
		 WHERE site_id = $1 AND status = 'live'`, siteID)
	if err != nil {
		return nil, fmt.Errorf("supersede live deploy: %w", err)
	}

	// Promote the target deploy.
	var d Deploy
	err = tx.QueryRow(ctx,
		`UPDATE _ayb_deploys SET status = 'live', updated_at = now()
		 WHERE id = $1
		 RETURNING id, site_id, status, file_count, total_bytes, error_message, created_at, updated_at`,
		deployID,
	).Scan(&d.ID, &d.SiteID, &d.Status, &d.FileCount, &d.TotalBytes,
		&d.ErrorMessage, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("promote deploy: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit promote tx: %w", err)
	}
	return &d, nil
}

// FailDeploy marks a deploy as failed with the given error message.
func (s *Service) FailDeploy(ctx context.Context, siteID, deployID, errorMsg string) (*Deploy, error) {
	var d Deploy
	err := s.pool.QueryRow(ctx,
		`UPDATE _ayb_deploys SET status = 'failed', error_message = $3, updated_at = now()
		 WHERE id = $1 AND site_id = $2 AND status = 'uploading'
		 RETURNING id, site_id, status, file_count, total_bytes, error_message, created_at, updated_at`,
		deployID, siteID, errorMsg,
	).Scan(&d.ID, &d.SiteID, &d.Status, &d.FileCount, &d.TotalBytes,
		&d.ErrorMessage, &d.CreatedAt, &d.UpdatedAt)
	if err == nil {
		return &d, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("fail deploy: %w", err)
	}

	return nil, s.deployStatusError(
		ctx,
		siteID,
		deployID,
		"cannot fail deploy with status %q",
	)
}

// RollbackDeploy finds the most recent superseded deploy for the site and
// promotes it back to live, reusing the same transactional promotion logic.
func (s *Service) RollbackDeploy(ctx context.Context, siteID string) (*Deploy, error) {
	var lastSupersededID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM _ayb_deploys
		 WHERE site_id = $1 AND status = 'superseded'
		 ORDER BY updated_at DESC
		 LIMIT 1`, siteID,
	).Scan(&lastSupersededID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoLiveDeploy
		}
		return nil, fmt.Errorf("find superseded deploy: %w", err)
	}

	return s.PromoteDeploy(ctx, siteID, lastSupersededID)
}

func (s *Service) deployStatusError(ctx context.Context, siteID, deployID, message string) error {
	currentDeploy, err := s.GetDeploy(ctx, siteID, deployID)
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: "+message, ErrInvalidTransition, currentDeploy.Status)
}
