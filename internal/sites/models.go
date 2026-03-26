package sites

import "time"

// DeployStatus represents the lifecycle state of a deploy.
type DeployStatus string

const (
	StatusUploading  DeployStatus = "uploading"
	StatusLive       DeployStatus = "live"
	StatusSuperseded DeployStatus = "superseded"
	StatusFailed     DeployStatus = "failed"
)

// Site represents a static site hosted on the platform.
type Site struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	SPAMode        bool      `json:"spaMode"`
	CustomDomainID *string   `json:"customDomainId,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	LiveDeployID   *string   `json:"liveDeployId,omitempty"`
}

// Deploy represents a single deployment of a site.
type Deploy struct {
	ID           string       `json:"id"`
	SiteID       string       `json:"siteId"`
	Status       DeployStatus `json:"status"`
	FileCount    int          `json:"fileCount"`
	TotalBytes   int64        `json:"totalBytes"`
	ErrorMessage *string      `json:"errorMessage,omitempty"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

// SiteListResult holds a paginated list of sites.
type SiteListResult struct {
	Sites      []Site `json:"sites"`
	TotalCount int    `json:"totalCount"`
	Page       int    `json:"page"`
	PerPage    int    `json:"perPage"`
}

// DeployListResult holds a paginated list of deploys for a site.
type DeployListResult struct {
	Deploys    []Deploy `json:"deploys"`
	TotalCount int      `json:"totalCount"`
	Page       int      `json:"page"`
	PerPage    int      `json:"perPage"`
}

// RuntimeSite contains the site and live deploy fields needed for host-based runtime serving.
type RuntimeSite struct {
	SiteID       string
	Slug         string
	SPAMode      bool
	LiveDeployID string
}
