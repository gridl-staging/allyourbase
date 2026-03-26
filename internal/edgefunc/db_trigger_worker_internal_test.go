package edgefunc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

type workerTestEventStore struct {
	events     []*DBTriggerQueueEvent
	claimCount int
	failedIDs  []string
}

func (s *workerTestEventStore) ClaimPendingEvents(_ context.Context, _ int) ([]*DBTriggerQueueEvent, error) {
	s.claimCount++
	if s.claimCount > 1 {
		return []*DBTriggerQueueEvent{}, nil
	}
	return s.events, nil
}

func (s *workerTestEventStore) MarkCompleted(_ context.Context, _ string) error {
	return nil
}

func (s *workerTestEventStore) MarkFailed(_ context.Context, eventID string) error {
	s.failedIDs = append(s.failedIDs, eventID)
	return nil
}

type workerTestTriggerStore struct {
	trigger *DBTrigger
}

func (s *workerTestTriggerStore) CreateDBTrigger(_ context.Context, _ *DBTrigger) (*DBTrigger, error) {
	return nil, errors.New("unexpected call")
}

func (s *workerTestTriggerStore) GetDBTrigger(_ context.Context, id string) (*DBTrigger, error) {
	if s.trigger != nil && s.trigger.ID == id {
		return s.trigger, nil
	}
	return nil, ErrDBTriggerNotFound
}

func (s *workerTestTriggerStore) ListDBTriggers(_ context.Context, _ string) ([]*DBTrigger, error) {
	return nil, errors.New("unexpected call")
}

func (s *workerTestTriggerStore) ListDBTriggersByTable(_ context.Context, _, _ string) ([]*DBTrigger, error) {
	return nil, errors.New("unexpected call")
}

func (s *workerTestTriggerStore) UpdateDBTrigger(_ context.Context, _ *DBTrigger) (*DBTrigger, error) {
	return nil, errors.New("unexpected call")
}

func (s *workerTestTriggerStore) DeleteDBTrigger(_ context.Context, _ string) error {
	return errors.New("unexpected call")
}

type workerTestInvoker struct{}

func (workerTestInvoker) InvokeByID(_ context.Context, _ string, _ Request) (Response, error) {
	return Response{}, errors.New("invoke failed")
}

func TestDBTriggerWorkerProcessAvailableEvents_LogsAccurateRetryMetadata(t *testing.T) {
	trigger := &DBTrigger{
		ID:         "trig-1",
		FunctionID: "func-1",
		TableName:  "users",
		Schema:     "public",
		Events:     []DBTriggerEvent{DBEventInsert},
		Enabled:    true,
	}
	event := &DBTriggerQueueEvent{
		ID:         "event-1",
		TriggerID:  trigger.ID,
		TableName:  "users",
		SchemaName: "public",
		Operation:  "INSERT",
		Payload:    json.RawMessage(`{"id":"1"}`),
		Attempts:   MaxDBTriggerRetries - 1,
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	worker := &DBTriggerWorker{
		eventStore: &workerTestEventStore{events: []*DBTriggerQueueEvent{event}},
		dispatcher: NewDBTriggerDispatcher(&workerTestTriggerStore{trigger: trigger}, workerTestInvoker{}),
		logger:     logger,
	}

	worker.processAvailableEvents(context.Background())

	if !strings.Contains(logs.String(), `"event_id":"event-1"`) {
		t.Fatalf("expected dispatch failure log for event, got: %s", logs.String())
	}

	var failureLog map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("unmarshal log line: %v", err)
		}
		if record["msg"] == "db trigger worker: dispatch failed" {
			failureLog = record
			break
		}
	}
	if failureLog == nil {
		t.Fatalf("expected worker dispatch failure log, got: %s", logs.String())
	}

	attempt, ok := failureLog["attempt"].(float64)
	if !ok {
		t.Fatalf("expected numeric attempt in failure log, got: %#v", failureLog["attempt"])
	}
	if int(attempt) != MaxDBTriggerRetries-1 {
		t.Fatalf("expected attempt %d, got %.0f", MaxDBTriggerRetries-1, attempt)
	}

	willRetry, ok := failureLog["will_retry"].(bool)
	if !ok {
		t.Fatalf("expected boolean will_retry in failure log, got: %#v", failureLog["will_retry"])
	}
	if !willRetry {
		t.Fatalf("expected will_retry=true for attempt %d of %d", MaxDBTriggerRetries-1, MaxDBTriggerRetries)
	}
}
