package matview

import "time"

// RefreshMode controls how REFRESH MATERIALIZED VIEW is executed.
type RefreshMode string

const (
	RefreshModeStandard   RefreshMode = "standard"
	RefreshModeConcurrent RefreshMode = "concurrent"
)

// RefreshStatus tracks the latest refresh outcome.
type RefreshStatus string

const (
	RefreshStatusSuccess RefreshStatus = "success"
	RefreshStatusError   RefreshStatus = "error"
)

// Registration is a row from _ayb_matview_refreshes.
type Registration struct {
	ID                    string         `json:"id"`
	SchemaName            string         `json:"schemaName"`
	ViewName              string         `json:"viewName"`
	RefreshMode           RefreshMode    `json:"refreshMode"`
	LastRefreshAt         *time.Time     `json:"lastRefreshAt,omitempty"`
	LastRefreshDurationMs *int           `json:"lastRefreshDurationMs,omitempty"`
	LastRefreshStatus     *RefreshStatus `json:"lastRefreshStatus,omitempty"`
	LastRefreshError      *string        `json:"lastRefreshError,omitempty"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

// RefreshResult contains metadata from a manual refresh operation.
type RefreshResult struct {
	Registration Registration `json:"registration"`
	DurationMs   int          `json:"durationMs"`
}
