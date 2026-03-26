package sites

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ResolveRuntimeSiteByCustomDomainID returns runtime-serving metadata for a site
// with a live deploy bound to the provided custom domain id.
func (s *Service) ResolveRuntimeSiteByCustomDomainID(ctx context.Context, customDomainID string) (*RuntimeSite, error) {
	customDomainID = strings.TrimSpace(customDomainID)
	if customDomainID == "" {
		return nil, ErrSiteNotFound
	}

	var runtimeSite RuntimeSite
	err := s.pool.QueryRow(ctx,
		`SELECT s.id, s.slug, s.spa_mode, d.id
		 FROM _ayb_sites s
		 INNER JOIN _ayb_deploys d ON d.site_id = s.id AND d.status = 'live'
		 WHERE s.custom_domain_id = $1`,
		customDomainID,
	).Scan(
		&runtimeSite.SiteID,
		&runtimeSite.Slug,
		&runtimeSite.SPAMode,
		&runtimeSite.LiveDeployID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSiteNotFound
		}
		return nil, fmt.Errorf("resolve runtime site by custom domain id: %w", err)
	}
	return &runtimeSite, nil
}

// ResolveRuntimeSiteBySlug returns runtime-serving metadata for the slug host.
func (s *Service) ResolveRuntimeSiteBySlug(ctx context.Context, slug string) (*RuntimeSite, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return nil, ErrSiteNotFound
	}

	var runtimeSite RuntimeSite
	err := s.pool.QueryRow(ctx,
		`SELECT s.id, s.slug, s.spa_mode, d.id
		 FROM _ayb_sites s
		 INNER JOIN _ayb_deploys d ON d.site_id = s.id AND d.status = 'live'
		 WHERE s.slug = $1`,
		slug,
	).Scan(
		&runtimeSite.SiteID,
		&runtimeSite.Slug,
		&runtimeSite.SPAMode,
		&runtimeSite.LiveDeployID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSiteNotFound
		}
		return nil, fmt.Errorf("resolve runtime site by slug: %w", err)
	}
	return &runtimeSite, nil
}
