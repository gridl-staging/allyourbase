package sites

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors for site operations.
var (
	ErrSiteNotFound          = errors.New("site not found")
	ErrSiteSlugTaken         = errors.New("site slug already taken")
	ErrSiteCustomDomainTaken = errors.New("site custom domain already taken")
	ErrDeployNotFound        = errors.New("deploy not found")
	ErrNoLiveDeploy          = errors.New("no live deploy exists")
	ErrInvalidTransition     = errors.New("invalid deploy status transition")
)

// Service manages site and deploy persistence and lifecycle.
type Service struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewService creates a new sites service backed by the given pool.
func NewService(pool *pgxpool.Pool, logger *slog.Logger) *Service {
	return &Service{pool: pool, logger: logger}
}

const defaultPerPage = 20

func clampPagination(page, perPage int) (int, int) {
	if perPage <= 0 || perPage > 100 {
		perPage = defaultPerPage
	}
	if page < 1 {
		page = 1
	}
	return page, perPage
}

func mapSiteWriteError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	switch pgErr.ConstraintName {
	case "_ayb_sites_slug_unique":
		return ErrSiteSlugTaken
	case "_ayb_sites_custom_domain_unique":
		return ErrSiteCustomDomainTaken
	default:
		return err
	}
}

// CreateSite inserts a new site record.
func (s *Service) CreateSite(ctx context.Context, name, slug string, spaMode bool, customDomainID *string) (*Site, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("site name is required")
	}
	if slug == "" {
		return nil, fmt.Errorf("site slug is required")
	}

	var site Site
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_sites (name, slug, spa_mode, custom_domain_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, slug, spa_mode, custom_domain_id, created_at, updated_at`,
		name, slug, spaMode, customDomainID,
	).Scan(&site.ID, &site.Name, &site.Slug, &site.SPAMode,
		&site.CustomDomainID, &site.CreatedAt, &site.UpdatedAt)
	if err != nil {
		if mappedErr := mapSiteWriteError(err); mappedErr != err {
			return nil, mappedErr
		}
		return nil, fmt.Errorf("create site: %w", err)
	}
	return &site, nil
}

// GetSite retrieves a site by ID, including its current live deploy ID if any.
func (s *Service) GetSite(ctx context.Context, id string) (*Site, error) {
	var site Site
	err := s.pool.QueryRow(ctx,
		`SELECT s.id, s.name, s.slug, s.spa_mode, s.custom_domain_id,
		        s.created_at, s.updated_at,
		        d.id AS live_deploy_id
		 FROM _ayb_sites s
		 LEFT JOIN _ayb_deploys d ON d.site_id = s.id AND d.status = 'live'
		 WHERE s.id = $1`, id,
	).Scan(&site.ID, &site.Name, &site.Slug, &site.SPAMode,
		&site.CustomDomainID, &site.CreatedAt, &site.UpdatedAt,
		&site.LiveDeployID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSiteNotFound
		}
		return nil, fmt.Errorf("get site: %w", err)
	}
	return &site, nil
}

// ListSites returns a paginated list of sites.
func (s *Service) ListSites(ctx context.Context, page, perPage int) (*SiteListResult, error) {
	page, perPage = clampPagination(page, perPage)
	offset := (page - 1) * perPage

	var totalCount int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM _ayb_sites`).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("count sites: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT s.id, s.name, s.slug, s.spa_mode, s.custom_domain_id,
		        s.created_at, s.updated_at,
		        d.id AS live_deploy_id
		 FROM _ayb_sites s
		 LEFT JOIN _ayb_deploys d ON d.site_id = s.id AND d.status = 'live'
		 ORDER BY s.created_at DESC
		 LIMIT $1 OFFSET $2`, perPage, offset)
	if err != nil {
		return nil, fmt.Errorf("list sites: %w", err)
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		var site Site
		if err := rows.Scan(&site.ID, &site.Name, &site.Slug, &site.SPAMode,
			&site.CustomDomainID, &site.CreatedAt, &site.UpdatedAt,
			&site.LiveDeployID); err != nil {
			return nil, fmt.Errorf("scan site: %w", err)
		}
		sites = append(sites, site)
	}
	if sites == nil {
		sites = []Site{}
	}

	return &SiteListResult{
		Sites:      sites,
		TotalCount: totalCount,
		Page:       page,
		PerPage:    perPage,
	}, nil
}

// UpdateSite updates mutable site fields atomically.
// Only provided (non-nil) fields are changed; clearCustomDomain explicitly
// nulls the FK without needing a zero-value pointer.
func (s *Service) UpdateSite(ctx context.Context, id string, name *string, spaMode *bool, customDomainID *string, clearCustomDomain bool) (*Site, error) {
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return nil, fmt.Errorf("site name is required")
		}
		name = &n
	}

	// Resolve custom_domain_id: explicit clear → NULL, pointer provided → use it,
	// otherwise keep the current value (COALESCE path).
	var domainArg interface{}
	if clearCustomDomain {
		domainArg = nil
	} else {
		domainArg = customDomainID // nil means "keep current"
	}

	var site Site
	err := s.pool.QueryRow(ctx,
		`WITH updated AS (
			UPDATE _ayb_sites
			   SET name = COALESCE($2, name),
			       spa_mode = COALESCE($3, spa_mode),
			       custom_domain_id = CASE
			           WHEN $5 THEN NULL
			           WHEN $4::uuid IS NOT NULL THEN $4::uuid
			           ELSE custom_domain_id
			       END,
			       updated_at = now()
			 WHERE id = $1
			 RETURNING id, name, slug, spa_mode, custom_domain_id, created_at, updated_at
		)
		SELECT updated.id, updated.name, updated.slug, updated.spa_mode,
		       updated.custom_domain_id, updated.created_at, updated.updated_at,
		       (
		           SELECT d.id
		             FROM _ayb_deploys d
		            WHERE d.site_id = updated.id AND d.status = 'live'
		            ORDER BY d.updated_at DESC
		            LIMIT 1
		       ) AS live_deploy_id
		  FROM updated`,
		id, name, spaMode, domainArg, clearCustomDomain,
	).Scan(&site.ID, &site.Name, &site.Slug, &site.SPAMode,
		&site.CustomDomainID, &site.CreatedAt, &site.UpdatedAt, &site.LiveDeployID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSiteNotFound
		}
		if mappedErr := mapSiteWriteError(err); mappedErr != err {
			return nil, mappedErr
		}
		return nil, fmt.Errorf("update site: %w", err)
	}
	return &site, nil
}

// DeleteSite deletes a site and all its deploys (via CASCADE).
func (s *Service) DeleteSite(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM _ayb_sites WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete site: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSiteNotFound
	}
	return nil
}
