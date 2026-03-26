package tenant

import (
	"errors"
	"regexp"
	"time"
)

// sentinel errors for organization and team stores.
var (
	ErrOrgNotFound            = errors.New("org not found")
	ErrParentOrgNotFound      = errors.New("parent org not found")
	ErrOrgSlugTaken           = errors.New("org slug is already taken")
	ErrTeamNotFound           = errors.New("team not found")
	ErrTeamSlugTaken          = errors.New("team slug is already taken")
	ErrInvalidSlug            = errors.New("invalid slug")
	ErrCircularParentOrg      = errors.New("circular parent org relationship")
	ErrOrgMembershipNotFound  = errors.New("org membership not found")
	ErrOrgMembershipExists    = errors.New("org membership already exists")
	ErrTeamMembershipNotFound = errors.New("team membership not found")
	ErrTeamMembershipExists   = errors.New("team membership already exists")
	ErrLastOwner              = errors.New("cannot remove or demote the last owner")
)

// Organization role constants.
const (
	OrgRoleOwner  = MemberRoleOwner
	OrgRoleAdmin  = MemberRoleAdmin
	OrgRoleMember = MemberRoleMember
	OrgRoleViewer = MemberRoleViewer

	TeamRoleLead   = "lead"
	TeamRoleMember = "member"
)

// Organization stores org tenant metadata and hierarchy links.
type Organization struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	ParentOrgID *string   `json:"parentOrgId,omitempty"`
	PlanTier    string    `json:"planTier"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Team stores team metadata and tenancy boundaries.
type Team struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// OrgMembership associates a user with an organization.
type OrgMembership struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	UserID    string    `json:"userId"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// TeamMembership associates a user with a team.
type TeamMembership struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"teamId"`
	UserID    string    `json:"userId"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// OrgUpdate defines patch fields for Organization update.
type OrgUpdate struct {
	Name        *string
	Slug        *string
	ParentOrgID *string
}

// TeamUpdate defines patch fields for Team update.
type TeamUpdate struct {
	Name *string
	Slug *string
}

var slugRegexp = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])$`)

// IsValidSlug validates tenant/org/team slugs across packages.
func IsValidSlug(slug string) bool {
	return slugRegexp.MatchString(slug)
}

// IsValidTeamRole validates team membership roles.
func IsValidTeamRole(role string) bool {
	switch role {
	case TeamRoleLead, TeamRoleMember:
		return true
	default:
		return false
	}
}
