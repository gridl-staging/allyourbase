package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/allyourbase/ayb/internal/jobs"
)

// jobAdmin is the interface for job queue admin operations.
// jobs.Service satisfies this interface.
type jobAdmin interface {
	List(ctx context.Context, state, jobType string, limit, offset int) ([]jobs.Job, error)
	Get(ctx context.Context, jobID string) (*jobs.Job, error)
	RetryNow(ctx context.Context, jobID string) (*jobs.Job, error)
	Cancel(ctx context.Context, jobID string) (*jobs.Job, error)
	Stats(ctx context.Context) (*jobs.QueueStats, error)

	ListSchedules(ctx context.Context) ([]jobs.Schedule, error)
	GetSchedule(ctx context.Context, id string) (*jobs.Schedule, error)
	CreateSchedule(ctx context.Context, sched *jobs.Schedule) (*jobs.Schedule, error)
	UpdateSchedule(ctx context.Context, id, cronExpr, timezone string, payload json.RawMessage, enabled bool, nextRunAt *time.Time) (*jobs.Schedule, error)
	DeleteSchedule(ctx context.Context, id string) error
	SetScheduleEnabled(ctx context.Context, id string, enabled bool) (*jobs.Schedule, error)
}

type jobListResponse struct {
	Items []jobs.Job `json:"items"`
	Count int        `json:"count"` // number of items returned (page size, not total)
}

type scheduleListResponse struct {
	Items []jobs.Schedule `json:"items"`
	Count int             `json:"count"` // number of items returned
}

type createScheduleRequest struct {
	Name        string          `json:"name"`
	JobType     string          `json:"jobType"`
	CronExpr    string          `json:"cronExpr"`
	Timezone    string          `json:"timezone"`
	Payload     json.RawMessage `json:"payload"`
	Enabled     *bool           `json:"enabled"`
	MaxAttempts int             `json:"maxAttempts"`
}

type updateScheduleRequest struct {
	CronExpr string          `json:"cronExpr"`
	Timezone string          `json:"timezone"`
	Payload  json.RawMessage `json:"payload"`
	Enabled  *bool           `json:"enabled"`
}
