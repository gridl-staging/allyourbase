package jobs

import (
	"context"
	"encoding/json"
	"time"
)

// JobState represents the lifecycle state of a job.
type JobState string

const (
	StateQueued    JobState = "queued"
	StateRunning   JobState = "running"
	StateCompleted JobState = "completed"
	StateFailed    JobState = "failed"
	StateCanceled  JobState = "canceled"
)

// Job represents a row in _ayb_jobs.
type Job struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	State          JobState        `json:"state"`
	RunAt          time.Time       `json:"runAt"`
	LeaseUntil     *time.Time      `json:"leaseUntil,omitempty"`
	WorkerID       *string         `json:"workerId,omitempty"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"maxAttempts"`
	LastError      *string         `json:"lastError,omitempty"`
	LastRunAt      *time.Time      `json:"lastRunAt,omitempty"`
	IdempotencyKey *string         `json:"idempotencyKey,omitempty"`
	ScheduleID     *string         `json:"scheduleId,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	CompletedAt    *time.Time      `json:"completedAt,omitempty"`
	CanceledAt     *time.Time      `json:"canceledAt,omitempty"`
}

// Schedule represents a row in _ayb_job_schedules.
type Schedule struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	JobType     string          `json:"jobType"`
	Payload     json.RawMessage `json:"payload"`
	CronExpr    string          `json:"cronExpr"`
	Timezone    string          `json:"timezone"`
	Enabled     bool            `json:"enabled"`
	MaxAttempts int             `json:"maxAttempts"`
	NextRunAt   *time.Time      `json:"nextRunAt,omitempty"`
	LastRunAt   *time.Time      `json:"lastRunAt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// EnqueueOpts are optional parameters for Enqueue.
type EnqueueOpts struct {
	RunAt          *time.Time
	IdempotencyKey string
	MaxAttempts    int // 0 = use service default
	ScheduleID     string
}

// JobHandler processes a job payload. Implementations must be idempotent.
type JobHandler func(ctx context.Context, payload json.RawMessage) error

// QueueStats holds aggregate counts by job state.
type QueueStats struct {
	Queued    int       `json:"queued"`
	Running   int       `json:"running"`
	Completed int       `json:"completed"`
	Failed    int       `json:"failed"`
	Canceled  int       `json:"canceled"`
	OldestAge *float64  `json:"oldestQueuedAgeSec,omitempty"` // seconds since oldest queued job's run_at
}
