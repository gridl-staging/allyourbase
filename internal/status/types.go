package status

import (
	"strings"
	"time"
)

// ServiceName identifies a probed service in status checks.
type ServiceName string

const (
	Database  ServiceName = "database"
	Storage   ServiceName = "storage"
	Auth      ServiceName = "auth"
	Realtime  ServiceName = "realtime"
	Functions ServiceName = "functions"
)

var validIncidentStatuses = map[IncidentStatus]struct{}{
	IncidentInvestigating: {},
	IncidentIdentified:    {},
	IncidentMonitoring:    {},
	IncidentResolved:      {},
}

// IsValid reports whether the incident status is one of the allowed lifecycle states.
func (s IncidentStatus) IsValid() bool {
	_, ok := validIncidentStatuses[s]
	return ok
}

// ParseIncidentStatus normalizes and validates a status string.
func ParseIncidentStatus(raw string) (IncidentStatus, bool) {
	s := IncidentStatus(strings.ToLower(strings.TrimSpace(raw)))
	return s, s.IsValid()
}

// ProbeResult is one health-check result for a service.
type ProbeResult struct {
	Service   ServiceName   `json:"service"`
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency"`
	Error     string        `json:"error,omitempty"`
	CheckedAt time.Time     `json:"checkedAt"`
}

// ServiceStatus is the rolled-up service health state.
type ServiceStatus string

const (
	Operational   ServiceStatus = "operational"
	Degraded      ServiceStatus = "degraded"
	PartialOutage ServiceStatus = "partial_outage"
	MajorOutage   ServiceStatus = "major_outage"
)

// IncidentStatus is the lifecycle state of an incident record.
type IncidentStatus string

const (
	IncidentInvestigating IncidentStatus = "investigating"
	IncidentIdentified    IncidentStatus = "identified"
	IncidentMonitoring    IncidentStatus = "monitoring"
	IncidentResolved      IncidentStatus = "resolved"
)

// IncidentUpdateEntry is a timeline entry attached to an incident.
type IncidentUpdateEntry struct {
	ID         string         `json:"id"`
	IncidentID string         `json:"incidentId"`
	Message    string         `json:"message"`
	Status     IncidentStatus `json:"status"`
	CreatedAt  time.Time      `json:"createdAt"`
}

// Incident is the persisted incident model exposed by admin/public status APIs.
type Incident struct {
	ID               string                `json:"id"`
	Title            string                `json:"title"`
	Status           IncidentStatus        `json:"status"`
	AffectedServices []string              `json:"affectedServices"`
	CreatedAt        time.Time             `json:"createdAt"`
	UpdatedAt        time.Time             `json:"updatedAt"`
	ResolvedAt       *time.Time            `json:"resolvedAt,omitempty"`
	Updates          []IncidentUpdateEntry `json:"updates,omitempty"`
}

// IncidentUpdate updates mutable incident fields on the parent incident record.
type IncidentUpdate struct {
	Title      *string         `json:"title,omitempty"`
	Status     *IncidentStatus `json:"status,omitempty"`
	ResolvedAt *time.Time      `json:"resolvedAt,omitempty"`
}

// StatusSnapshot is the public/read-model status view at a point in time.
type StatusSnapshot struct {
	Status    ServiceStatus `json:"status"`
	Services  []ProbeResult `json:"services"`
	CheckedAt time.Time     `json:"checkedAt"`
}
