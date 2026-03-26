package edgefunc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/adhocore/gronx"
	"github.com/google/uuid"
)

// JobType used for edge function cron triggers in the jobs scheduler.
const CronJobType = "edge_func_invoke"

// FunctionInvoker invokes edge functions by ID. Implemented by Service.
type FunctionInvoker interface {
	InvokeByID(ctx context.Context, functionID string, req Request) (Response, error)
}

// JobsScheduler is the subset of jobs.Service needed by CronTriggerService.
// This avoids a direct dependency on the jobs package in the trigger layer.
type JobsScheduler interface {
	CreateSchedule(ctx context.Context, name, jobType string, payload json.RawMessage, cronExpr, timezone string, maxAttempts int, nextRunAt *time.Time) (string, error)
	DeleteSchedule(ctx context.Context, id string) error
	SetScheduleEnabled(ctx context.Context, id string, enabled bool) error
}

// CreateCronTriggerInput holds the parameters for creating a cron trigger.
type CreateCronTriggerInput struct {
	FunctionID string
	CronExpr   string
	Timezone   string
	Payload    json.RawMessage
}

// CronTriggerService manages the lifecycle of cron triggers, coordinating
// between the CronTriggerStore (trigger metadata) and JobsScheduler (schedule records).
type CronTriggerService struct {
	store     CronTriggerStore
	scheduler JobsScheduler
}

// NewCronTriggerService creates a new CronTriggerService.
func NewCronTriggerService(store CronTriggerStore, scheduler JobsScheduler) *CronTriggerService {
	return &CronTriggerService{store: store, scheduler: scheduler}
}

func (s *CronTriggerService) ensureStoreConfigured() error {
	if s == nil || isNilable(s.store) {
		return ErrCronTriggerServiceNotConfigured
	}
	return nil
}

func (s *CronTriggerService) ensureSchedulerConfigured() error {
	if err := s.ensureStoreConfigured(); err != nil {
		return err
	}
	if isNilable(s.scheduler) {
		return ErrCronTriggerServiceNotConfigured
	}
	return nil
}

func isNilable(value any) bool {
	if value == nil {
		return true
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// Create validates input, creates a jobs.Schedule, and persists the cron trigger linking record.
func (s *CronTriggerService) Create(ctx context.Context, input CreateCronTriggerInput) (*CronTrigger, error) {
	if err := s.ensureSchedulerConfigured(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.FunctionID) == "" {
		return nil, ErrFunctionIDRequired
	}
	if strings.TrimSpace(input.CronExpr) == "" {
		return nil, ErrCronExprRequired
	}

	// Default timezone
	if input.Timezone == "" {
		input.Timezone = "UTC"
	}

	// Validate cron expression
	gron := gronx.New()
	if !gron.IsValid(input.CronExpr) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidCronExpr, input.CronExpr)
	}

	// Validate timezone
	loc, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTimezone, input.Timezone)
	}

	// Compute next run time
	nextTick, err := gronx.NextTickAfter(input.CronExpr, time.Now().In(loc), false)
	if err != nil {
		return nil, fmt.Errorf("computing next run time: %w", err)
	}
	nextRunUTC := nextTick.UTC()

	// Default payload
	if input.Payload == nil {
		input.Payload = json.RawMessage(`{}`)
	}

	// Pre-generate the trigger ID so it can be included in the schedule payload.
	// This lets the job handler attribute invocations back to the correct trigger.
	triggerID := uuid.New().String()

	// Build job payload containing function reference and trigger ID
	jobPayload := CronJobPayload{
		FunctionID:    input.FunctionID,
		CronTriggerID: triggerID,
		Payload:       input.Payload,
	}
	jobPayloadBytes, err := json.Marshal(jobPayload)
	if err != nil {
		return nil, fmt.Errorf("marshaling job payload: %w", err)
	}

	// Create the schedule in the jobs service
	scheduleName := fmt.Sprintf("edge_cron_%s_%s", input.FunctionID, input.CronExpr)
	scheduleID, err := s.scheduler.CreateSchedule(ctx, scheduleName, CronJobType, jobPayloadBytes, input.CronExpr, input.Timezone, 3, &nextRunUTC)
	if err != nil {
		return nil, fmt.Errorf("creating schedule: %w", err)
	}

	// Create the cron trigger record with the pre-generated ID
	trigger := &CronTrigger{
		ID:         triggerID,
		FunctionID: input.FunctionID,
		ScheduleID: scheduleID,
		CronExpr:   input.CronExpr,
		Timezone:   input.Timezone,
		Payload:    input.Payload,
		Enabled:    true,
	}

	created, err := s.store.CreateCronTrigger(ctx, trigger)
	if err != nil {
		// Clean up the schedule if trigger creation fails
		if delErr := s.scheduler.DeleteSchedule(ctx, scheduleID); delErr != nil {
			slog.Error("cron trigger: cleanup failed after create error",
				"schedule_id", scheduleID, "create_error", err, "delete_error", delErr)
		}
		return nil, fmt.Errorf("creating cron trigger: %w", err)
	}

	return created, nil
}

// Get returns a cron trigger by ID.
func (s *CronTriggerService) Get(ctx context.Context, id string) (*CronTrigger, error) {
	if err := s.ensureStoreConfigured(); err != nil {
		return nil, err
	}
	return s.store.GetCronTrigger(ctx, id)
}

// List returns all cron triggers for a function.
func (s *CronTriggerService) List(ctx context.Context, functionID string) ([]*CronTrigger, error) {
	if err := s.ensureStoreConfigured(); err != nil {
		return nil, err
	}
	return s.store.ListCronTriggers(ctx, functionID)
}

// Delete removes a cron trigger and its underlying schedule.
func (s *CronTriggerService) Delete(ctx context.Context, id string) error {
	if err := s.ensureSchedulerConfigured(); err != nil {
		return err
	}
	trigger, err := s.store.GetCronTrigger(ctx, id)
	if err != nil {
		return err
	}

	// Delete schedule first so trigger metadata isn't removed if scheduler delete fails.
	// The FK schedule_id uses ON DELETE CASCADE, so the trigger row may already be gone.
	if err := s.scheduler.DeleteSchedule(ctx, trigger.ScheduleID); err != nil {
		return fmt.Errorf("deleting schedule: %w", err)
	}

	if err := s.store.DeleteCronTrigger(ctx, id); err != nil && !errors.Is(err, ErrCronTriggerNotFound) {
		return err
	}

	return nil
}

// SetEnabled toggles a cron trigger and its underlying schedule.
func (s *CronTriggerService) SetEnabled(ctx context.Context, id string, enabled bool) (*CronTrigger, error) {
	if err := s.ensureSchedulerConfigured(); err != nil {
		return nil, err
	}
	trigger, err := s.store.GetCronTrigger(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.scheduler.SetScheduleEnabled(ctx, trigger.ScheduleID, enabled); err != nil {
		return nil, fmt.Errorf("toggling schedule: %w", err)
	}

	trigger.Enabled = enabled
	return s.store.UpdateCronTrigger(ctx, trigger)
}

// NewCronJobHandler returns a JobHandler function for job type "edge_func_invoke".
// It deserializes the CronJobPayload and invokes the function via the FunctionInvoker.
func NewCronJobHandler(invoker FunctionInvoker) func(ctx context.Context, payload json.RawMessage) error {
	return func(ctx context.Context, payload json.RawMessage) error {
		var cronPayload CronJobPayload
		if err := json.Unmarshal(payload, &cronPayload); err != nil {
			return fmt.Errorf("unmarshaling cron job payload: %w", err)
		}

		req := Request{
			Method: "POST",
			Path:   "/cron",
			Body:   cronPayload.Payload,
		}

		ctx = WithTriggerMeta(ctx, TriggerCron, cronPayload.CronTriggerID)
		_, err := invoker.InvokeByID(ctx, cronPayload.FunctionID, req)
		return err
	}
}
