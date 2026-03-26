package tenant

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dry-run action constants.
const (
	dryRunActionCreate = "create"
	dryRunActionReuse  = "reuse"
	dryRunActionSkip   = "skip"
)

// Dry-run conflict reason constants.
const (
	dryRunConflictMissingOwner  = "missing-owner"
	dryRunConflictNoApps        = "no-apps"
	dryRunConflictInvalidSlug   = "invalid-slug"
	dryRunConflictSlugCollision = "slug-collision"
)

// MigrationOpts controls batch size and item limits for progressive rollouts.
type MigrationOpts struct {
	// BatchSize is the number of owner groups processed per transaction.
	// Zero or negative defaults to 50.
	BatchSize int
	// MaxItems caps the total number of groups processed in one run. Zero means unlimited.
	MaxItems int
}

// MigrationResult holds structured counters for a migration run.
type MigrationResult struct {
	ExaminedGroups     int
	CreatedTenants     int
	ReusedTenants      int
	AssignedApps       int
	CreatedMemberships int
	SkippedGroups      int
	ErroredGroups      int
	Errors             []string
}

// DryRunGroupReport describes the proposed action for one owner group.
type DryRunGroupReport struct {
	OwnerUserID      string
	OwnerEmail       string
	AppIDs           []string
	ProposedSlug     string
	ExistingTenantID string
	Action           string
	Conflict         string
}

// DryRunReport is the full preview produced before applying migration.
type DryRunReport struct {
	Groups  []DryRunGroupReport
	Summary MigrationResult
}

// ConsistencyReport holds post-migration health check results.
type ConsistencyReport struct {
	NullTenantIDApps     int
	DanglingTenantIDApps int
	OrphanTenants        int
	TotalApps            int
	MigratedApps         int
	Clean                bool
	Issues               []string
}

// computeClean derives the Clean flag from the violation counts.
func (r *ConsistencyReport) computeClean() {
	r.Clean = r.NullTenantIDApps == 0 && r.DanglingTenantIDApps == 0 && r.OrphanTenants == 0
}

// legacyAppRow is a raw DB row joining _ayb_apps with _ayb_users.
type legacyAppRow struct {
	AppID       string
	AppTenantID *string // nil = not yet migrated
	OwnerUserID string
	OwnerEmail  string // empty when owner is missing from _ayb_users
}

// ownerGroup is one canonical tenant candidate: all apps belonging to one owner.
type ownerGroup struct {
	OwnerUserID      string
	OwnerEmail       string
	AppIDs           []string // unmigrated app IDs
	AlreadyMigrated  []string // app IDs already having tenant_id set
	ExistingTenantID string   // non-empty when a migration tenant already exists for this owner
}

// MigrationService handles legacy app-to-tenant backfill operations.
type MigrationService struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewMigrationService creates a MigrationService backed by the given pool.
func NewMigrationService(pool *pgxpool.Pool, logger *slog.Logger) *MigrationService {
	return &MigrationService{pool: pool, logger: logger}
}

// MigrationDryRun previews what MigrateLegacyApps would do without writing anything.
func (ms *MigrationService) MigrationDryRun(ctx context.Context, opts MigrationOpts) (*DryRunReport, error) {
	rows, err := ms.fetchLegacyAppRows(ctx)
	if err != nil {
		return nil, err
	}
	groups := groupAppsByOwner(rows)
	existingSlugs, err := ms.fetchExistingSlugs(ctx)
	if err != nil {
		return nil, err
	}
	existingMigrationTenants, err := ms.fetchMigrationTenantsByOwner(ctx)
	if err != nil {
		return nil, err
	}

	report := &DryRunReport{}
	slugsSeen := make(map[string]bool)
	for k, v := range existingSlugs {
		slugsSeen[k] = v
	}

	limit := opts.MaxItems
	for i := range groups {
		if limit > 0 && i >= limit {
			break
		}
		g := &groups[i]
		if tid, ok := existingMigrationTenants[g.OwnerUserID]; ok {
			g.ExistingTenantID = tid
		}

		action, conflict := classifyGroup(*g, slugsSeen)
		slug := proposeDryRunSlug(*g, action, slugsSeen)
		grp := DryRunGroupReport{
			OwnerUserID:      g.OwnerUserID,
			OwnerEmail:       g.OwnerEmail,
			AppIDs:           g.AppIDs,
			ProposedSlug:     slug,
			ExistingTenantID: g.ExistingTenantID,
			Action:           action,
			Conflict:         conflict,
		}
		report.Groups = append(report.Groups, grp)

		switch action {
		case dryRunActionCreate:
			report.Summary.CreatedTenants++
			report.Summary.AssignedApps += len(g.AppIDs)
			report.Summary.CreatedMemberships++
		case dryRunActionReuse:
			report.Summary.ReusedTenants++
			report.Summary.AssignedApps += len(g.AppIDs)
		case dryRunActionSkip:
			report.Summary.SkippedGroups++
		}
		report.Summary.ExaminedGroups++
	}

	return report, nil
}

// MigrateLegacyApps applies tenant assignment for all unmigrated apps.
// Each batch of owner groups is wrapped in its own transaction for safe rollback.
func (ms *MigrationService) MigrateLegacyApps(ctx context.Context, opts MigrationOpts) (*MigrationResult, error) {
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	runID := fmt.Sprintf("migration-%d", time.Now().UnixNano())
	ms.logger.Info("migration run started", "run_id", runID, "batch_size", batchSize)
	start := time.Now()

	rows, err := ms.fetchLegacyAppRows(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching legacy apps: %w", err)
	}
	groups := groupAppsByOwner(rows)

	existingSlugs, err := ms.fetchExistingSlugs(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching existing slugs: %w", err)
	}
	existingMigrationTenants, err := ms.fetchMigrationTenantsByOwner(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching migration tenants: %w", err)
	}

	result := &MigrationResult{}
	slugsSeen := make(map[string]bool)
	for k, v := range existingSlugs {
		slugsSeen[k] = v
	}

	limit := opts.MaxItems
	for batchStart := 0; batchStart < len(groups); batchStart += batchSize {
		if limit > 0 && result.ExaminedGroups >= limit {
			break
		}
		batchEnd := batchStart + batchSize
		if batchEnd > len(groups) {
			batchEnd = len(groups)
		}
		batch := groups[batchStart:batchEnd]
		if limit > 0 && result.ExaminedGroups+len(batch) > limit {
			batch = batch[:limit-result.ExaminedGroups]
		}

		batchResult, batchSlugs, err := ms.applyBatch(ctx, batch, existingMigrationTenants, slugsSeen)
		if err != nil {
			result.ExaminedGroups += len(batch)
			result.Errors = append(result.Errors, fmt.Sprintf("batch %d: %v", batchStart/batchSize, err))
			result.ErroredGroups += len(batch)
			continue
		}
		for k, v := range batchSlugs {
			slugsSeen[k] = v
		}
		result.ExaminedGroups += batchResult.ExaminedGroups
		result.CreatedTenants += batchResult.CreatedTenants
		result.ReusedTenants += batchResult.ReusedTenants
		result.AssignedApps += batchResult.AssignedApps
		result.CreatedMemberships += batchResult.CreatedMemberships
		result.SkippedGroups += batchResult.SkippedGroups
		result.ErroredGroups += batchResult.ErroredGroups
		result.Errors = append(result.Errors, batchResult.Errors...)
	}

	ms.logger.Info("migration run complete",
		"run_id", runID,
		"duration_ms", time.Since(start).Milliseconds(),
		"examined_groups", result.ExaminedGroups,
		"created_tenants", result.CreatedTenants,
		"reused_tenants", result.ReusedTenants,
		"assigned_apps", result.AssignedApps,
		"created_memberships", result.CreatedMemberships,
		"skipped_groups", result.SkippedGroups,
		"errored_groups", result.ErroredGroups,
	)
	return result, nil
}

// applyBatch processes one batch of owner groups inside a single transaction.
// Returns the per-batch result and the slug names claimed during this batch
// so the caller can merge them into the global seen set.
func (ms *MigrationService) applyBatch(ctx context.Context, batch []ownerGroup, existingMigrationTenants map[string]string, slugsSeen map[string]bool) (*MigrationResult, map[string]bool, error) {
	tx, err := ms.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	result := &MigrationResult{}
	batchSlugClaims := make(map[string]bool)
	batchSlugsSeen := make(map[string]bool, len(slugsSeen))
	for k, v := range slugsSeen {
		batchSlugsSeen[k] = v
	}

	for i := range batch {
		g := &batch[i]
		if tid, ok := existingMigrationTenants[g.OwnerUserID]; ok {
			g.ExistingTenantID = tid
		}

		result.ExaminedGroups++

		action, conflict := classifyGroup(*g, batchSlugsSeen)
		if action == dryRunActionSkip {
			ms.logger.Warn("migration: skipping owner group", "owner", g.OwnerUserID, "reason", conflict)
			result.SkippedGroups++
			continue
		}

		var tenantID string
		idemKey := migrationIdempotencyKey(g.OwnerUserID)

		if action == dryRunActionReuse {
			tenantID = g.ExistingTenantID
			result.ReusedTenants++
		} else {
			base := deriveSlug(g.OwnerEmail)
			slug := resolveSlugWithCollisions(base, batchSlugsSeen)

			err := tx.QueryRow(ctx,
				`INSERT INTO _ayb_tenants (name, slug, isolation_mode, plan_tier, region, idempotency_key)
				 VALUES ($1, $2, 'schema', 'free', 'default', $3)
				 ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL
				 DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
				 RETURNING id`,
				slug, slug, idemKey,
			).Scan(&tenantID)
			if err != nil {
				return nil, nil, fmt.Errorf("creating tenant for owner %s (slug %s): %w", g.OwnerUserID, slug, err)
			}
			batchSlugsSeen[slug] = true
			batchSlugClaims[slug] = true
			result.CreatedTenants++
		}

		// Add owner membership (idempotent via ON CONFLICT DO NOTHING).
		if g.OwnerEmail != "" {
			tag, err := tx.Exec(ctx,
				`INSERT INTO _ayb_tenant_memberships (tenant_id, user_id, role)
				 VALUES ($1::uuid, $2::uuid, 'owner')
				 ON CONFLICT (tenant_id, user_id) DO NOTHING`,
				tenantID, g.OwnerUserID,
			)
			if err != nil {
				return nil, nil, fmt.Errorf("adding owner membership for owner %s in tenant %s: %w", g.OwnerUserID, tenantID, err)
			}
			if tag.RowsAffected() > 0 {
				result.CreatedMemberships++
			}
		}

		// Assign unmigrated apps (idempotent: only rows where tenant_id IS NULL).
		for _, appID := range g.AppIDs {
			tag, err := tx.Exec(ctx,
				`UPDATE _ayb_apps SET tenant_id = $1::uuid WHERE id = $2::uuid AND tenant_id IS NULL`,
				tenantID, appID,
			)
			if err != nil {
				return nil, nil, fmt.Errorf("assigning app %s to tenant %s: %w", appID, tenantID, err)
			}
			if tag.RowsAffected() > 0 {
				result.AssignedApps++
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("committing batch: %w", err)
	}
	return result, batchSlugClaims, nil
}

// CheckMigrationConsistency queries the DB for migration health indicators.
// It can be run independently of apply mode and returns machine-readable output.
func (ms *MigrationService) CheckMigrationConsistency(ctx context.Context) (*ConsistencyReport, error) {
	r := &ConsistencyReport{}

	if err := ms.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_apps`).Scan(&r.TotalApps); err != nil {
		return nil, fmt.Errorf("counting total apps: %w", err)
	}

	if err := ms.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_apps WHERE tenant_id IS NULL`).Scan(&r.NullTenantIDApps); err != nil {
		return nil, fmt.Errorf("counting null tenant_id apps: %w", err)
	}
	r.MigratedApps = r.TotalApps - r.NullTenantIDApps

	if err := ms.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_apps a
		 WHERE a.tenant_id IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM _ayb_tenants t WHERE t.id = a.tenant_id)`,
	).Scan(&r.DanglingTenantIDApps); err != nil {
		return nil, fmt.Errorf("counting dangling tenant refs: %w", err)
	}

	// Only flag migration-created tenants as orphans; admin-created tenants
	// legitimately have no apps and should not trigger false positives.
	if err := ms.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_tenants t
		 WHERE t.idempotency_key LIKE 'miglegacy:%'
		   AND NOT EXISTS (SELECT 1 FROM _ayb_apps a WHERE a.tenant_id = t.id)`,
	).Scan(&r.OrphanTenants); err != nil {
		return nil, fmt.Errorf("counting orphan tenants: %w", err)
	}

	if r.NullTenantIDApps > 0 {
		r.Issues = append(r.Issues, fmt.Sprintf("%d app(s) still have NULL tenant_id", r.NullTenantIDApps))
	}
	if r.DanglingTenantIDApps > 0 {
		r.Issues = append(r.Issues, fmt.Sprintf("%d app(s) reference non-existent tenants", r.DanglingTenantIDApps))
	}
	if r.OrphanTenants > 0 {
		r.Issues = append(r.Issues, fmt.Sprintf("%d migration tenant(s) have no apps", r.OrphanTenants))
	}
	r.computeClean()
	return r, nil
}

// --- pure helper functions (no DB dependency) ---

// fetchLegacyAppRows loads all apps joined with their owner's email.
func (ms *MigrationService) fetchLegacyAppRows(ctx context.Context) ([]legacyAppRow, error) {
	rows, err := ms.pool.Query(ctx,
		`SELECT a.id, a.tenant_id, a.owner_user_id, COALESCE(u.email, '')
		 FROM _ayb_apps a
		 LEFT JOIN _ayb_users u ON u.id = a.owner_user_id
		 ORDER BY a.owner_user_id, a.created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying legacy apps: %w", err)
	}
	defer rows.Close()

	var out []legacyAppRow
	for rows.Next() {
		var r legacyAppRow
		if err := rows.Scan(&r.AppID, &r.AppTenantID, &r.OwnerUserID, &r.OwnerEmail); err != nil {
			return nil, fmt.Errorf("scanning legacy app row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// fetchExistingSlugs returns a set of all slug values currently in _ayb_tenants.
func (ms *MigrationService) fetchExistingSlugs(ctx context.Context) (map[string]bool, error) {
	rows, err := ms.pool.Query(ctx, `SELECT slug FROM _ayb_tenants`)
	if err != nil {
		return nil, fmt.Errorf("querying existing slugs: %w", err)
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("scanning slug: %w", err)
		}
		out[slug] = true
	}
	return out, rows.Err()
}

// fetchMigrationTenantsByOwner returns a map of owner_user_id → tenant_id for
// tenants already created by a previous migration run.
func (ms *MigrationService) fetchMigrationTenantsByOwner(ctx context.Context) (map[string]string, error) {
	rows, err := ms.pool.Query(ctx,
		`SELECT idempotency_key, id FROM _ayb_tenants WHERE idempotency_key LIKE 'miglegacy:%'`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying migration tenants: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var idemKey, tenantID string
		if err := rows.Scan(&idemKey, &tenantID); err != nil {
			return nil, fmt.Errorf("scanning migration tenant: %w", err)
		}
		ownerID := strings.TrimPrefix(idemKey, "miglegacy:")
		out[ownerID] = tenantID
	}
	return out, rows.Err()
}

// migrationIdempotencyKey returns the stable key used to identify migration-created tenants.
func migrationIdempotencyKey(ownerUserID string) string {
	return "miglegacy:" + ownerUserID
}

// groupAppsByOwner groups legacy app rows by owner_user_id, returning groups
// sorted deterministically by owner_user_id. This is the canonical grouping
// algorithm used by both dry-run and apply modes.
func groupAppsByOwner(rows []legacyAppRow) []ownerGroup {
	if len(rows) == 0 {
		return nil
	}

	ownerOrder := make([]string, 0)
	byOwner := make(map[string]*ownerGroup)

	for _, r := range rows {
		if _, seen := byOwner[r.OwnerUserID]; !seen {
			ownerOrder = append(ownerOrder, r.OwnerUserID)
			byOwner[r.OwnerUserID] = &ownerGroup{
				OwnerUserID: r.OwnerUserID,
				OwnerEmail:  r.OwnerEmail,
			}
		}
		g := byOwner[r.OwnerUserID]
		if r.AppTenantID != nil {
			g.AlreadyMigrated = append(g.AlreadyMigrated, r.AppID)
		} else {
			g.AppIDs = append(g.AppIDs, r.AppID)
		}
	}

	sort.Strings(ownerOrder)

	groups := make([]ownerGroup, len(ownerOrder))
	for i, ownerID := range ownerOrder {
		groups[i] = *byOwner[ownerID]
	}
	return groups
}

// slugRe matches sequences of non-slug characters (anything that's not
// lowercase alphanumeric).
var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// deriveSlug computes a URL-safe slug from an owner's email address.
func deriveSlug(email string) string {
	local := email
	if idx := strings.Index(email, "@"); idx >= 0 {
		local = email[:idx]
	}
	local = strings.ToLower(local)
	slug := slugRe.ReplaceAllString(local, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "tenant"
	}
	return slug
}

// resolveSlugWithCollisions appends numeric suffixes (-1, -2, …) until it
// finds a slug not present in existing. The caller must add the returned slug
// to existing to prevent the same value from being reused.
func resolveSlugWithCollisions(base string, existing map[string]bool) string {
	if !existing[base] {
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !existing[candidate] {
			return candidate
		}
	}
}

// classifyGroup determines the dry-run/apply action for an owner group.
func classifyGroup(g ownerGroup, existingSlugs map[string]bool) (action, conflict string) {
	if g.ExistingTenantID != "" {
		return dryRunActionReuse, ""
	}
	if g.OwnerEmail == "" {
		return dryRunActionSkip, dryRunConflictMissingOwner
	}
	if len(g.AppIDs) == 0 {
		return dryRunActionSkip, dryRunConflictNoApps
	}
	base := deriveSlug(g.OwnerEmail)
	if base == "" {
		return dryRunActionSkip, dryRunConflictInvalidSlug
	}
	resolved := resolveSlugWithCollisions(base, existingSlugs)
	if resolved != base {
		return dryRunActionCreate, dryRunConflictSlugCollision
	}
	return dryRunActionCreate, ""
}

// proposeDryRunSlug derives a preview slug and claims it only for create actions.
// Reuse actions can display a derived slug for visibility but must not reserve it.
func proposeDryRunSlug(g ownerGroup, action string, slugsSeen map[string]bool) string {
	if action == dryRunActionSkip {
		return ""
	}
	base := deriveSlug(g.OwnerEmail)
	slug := resolveSlugWithCollisions(base, slugsSeen)
	if action == dryRunActionCreate {
		slugsSeen[slug] = true
	}
	return slug
}

// Ensure pgx is used (Row scanning in fetchLegacyAppRows).
var _ pgx.Row
