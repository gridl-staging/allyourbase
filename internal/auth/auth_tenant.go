// Package auth Manages default tenant lookup and automatic creation for authenticated users, ensuring every user has at least one tenant available for sessions.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func cloneTokenOptions(opts *tokenOptions) *tokenOptions {
	if opts == nil {
		return &tokenOptions{}
	}
	cloned := *opts
	if len(opts.AMR) > 0 {
		cloned.AMR = append([]string(nil), opts.AMR...)
	}
	return &cloned
}

func (s *Service) sessionTokenOptions(ctx context.Context, user *User, opts *tokenOptions) (*tokenOptions, error) {
	resolved := cloneTokenOptions(opts)
	if user == nil || user.IsAnonymous || strings.TrimSpace(user.ID) == "" {
		return resolved, nil
	}

	tenantID, err := s.ensureDefaultTenantID(ctx, user)
	if err != nil {
		return nil, err
	}
	resolved.TenantID = tenantID
	return resolved, nil
}

// ensureDefaultTenantID returns the user's default tenant ID, automatically creating a personal tenant if one does not exist and adding the user as its owner.
func (s *Service) ensureDefaultTenantID(ctx context.Context, user *User) (string, error) {
	if user == nil || strings.TrimSpace(user.ID) == "" {
		return "", nil
	}

	tenantID, err := s.lookupDefaultTenantID(ctx, user.ID)
	if err != nil {
		return "", err
	}
	if tenantID != "" {
		return tenantID, nil
	}

	tenantSvc := tenant.NewService(s.pool, s.logger)
	personalTenant, err := tenantSvc.CreateTenant(
		ctx,
		personalTenantName(user),
		"user-"+strings.ToLower(user.ID),
		"schema",
		"free",
		"default",
		nil,
		"auth-personal-tenant:"+user.ID,
	)
	if err != nil {
		return "", fmt.Errorf("creating default tenant for user %s: %w", user.ID, err)
	}
	if _, err := tenantSvc.AddMembership(ctx, personalTenant.ID, user.ID, tenant.MemberRoleOwner); err != nil && !errors.Is(err, tenant.ErrMembershipExists) {
		return "", fmt.Errorf("adding default tenant membership for user %s: %w", user.ID, err)
	}
	return personalTenant.ID, nil
}

// lookupDefaultTenantID queries the database to find the user's highest-priority tenant membership, ordered by role precedence (owner, admin, member) and creation time, returning the tenant ID or an empty string if no membership exists.
func (s *Service) lookupDefaultTenantID(ctx context.Context, userID string) (string, error) {
	var tenantID string
	err := s.pool.QueryRow(ctx, `
		SELECT m.tenant_id
		  FROM _ayb_tenant_memberships m
		  JOIN _ayb_tenants t ON t.id = m.tenant_id
		 WHERE m.user_id = $1
		   AND t.state <> 'deleted'
		 ORDER BY CASE m.role
		     WHEN 'owner' THEN 0
		     WHEN 'admin' THEN 1
		     WHEN 'member' THEN 2
		     ELSE 3
		 END,
		 m.created_at ASC
		 LIMIT 1
	`, userID).Scan(&tenantID)
	if err == nil {
		return tenantID, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return "", fmt.Errorf("resolving default tenant for user %s: %w", userID, err)
}

func personalTenantName(user *User) string {
	if user == nil {
		return "Personal Workspace"
	}
	if email := strings.TrimSpace(user.Email); email != "" {
		return email
	}
	if phone := strings.TrimSpace(user.Phone); phone != "" {
		return phone
	}
	return "Personal Workspace"
}
