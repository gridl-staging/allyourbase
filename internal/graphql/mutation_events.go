package graphql

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
)

type mutationEventCollector struct {
	mu     sync.Mutex
	events []*realtime.Event
}

type mutationEventCollectorKey struct{}

func ctxWithMutationEventCollector(ctx context.Context) context.Context {
	if mutationEventCollectorFromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, mutationEventCollectorKey{}, &mutationEventCollector{})
}

func mutationEventCollectorFromContext(ctx context.Context) *mutationEventCollector {
	collector, _ := ctx.Value(mutationEventCollectorKey{}).(*mutationEventCollector)
	return collector
}

func addMutationEvent(ctx context.Context, event *realtime.Event) {
	if event == nil {
		return
	}
	collector := mutationEventCollectorFromContext(ctx)
	if collector == nil {
		return
	}
	collector.mu.Lock()
	collector.events = append(collector.events, event)
	collector.mu.Unlock()
}

func mutationEventsFromContext(ctx context.Context) []*realtime.Event {
	collector := mutationEventCollectorFromContext(ctx)
	if collector == nil {
		return nil
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	out := make([]*realtime.Event, len(collector.events))
	copy(out, collector.events)
	return out
}

// collectMutationEvents creates mutation events from affected rows for insert, update, or delete operations, converting operation types to action names and structuring records with old record data as needed.
func collectMutationEvents(ctx context.Context, tbl *schema.Table, op string, rows []map[string]any) {
	if tbl == nil || len(rows) == 0 {
		return
	}
	action := mutationAction(op)
	if action == "" {
		return
	}
	if action == "update" {
		collectUpdateMutationEvents(ctx, tbl, rows, nil)
		return
	}

	for _, row := range rows {
		event := &realtime.Event{
			Action: action,
			Table:  tbl.Name,
			Record: row,
		}
		if action == "delete" {
			event.OldRecord = row
			event.Record = mutationPKMap(tbl, row)
		}
		addMutationEvent(ctx, event)
	}
}

// collectUpdateMutationEvents creates update mutation events from new rows and optional old rows, matching them by primary key to track record changes for real-time subscriptions.
func collectUpdateMutationEvents(ctx context.Context, tbl *schema.Table, rows []map[string]any, oldRows []map[string]any) {
	if tbl == nil || len(rows) == 0 {
		return
	}
	oldRowsByPK := mutationRowsByPK(tbl, oldRows)
	for _, row := range rows {
		event := &realtime.Event{
			Action: "update",
			Table:  tbl.Name,
			Record: row,
		}
		if key, ok := mutationRowKey(tbl, row); ok {
			event.OldRecord = oldRowsByPK[key]
		}
		addMutationEvent(ctx, event)
	}
}

func mutationAction(op string) string {
	switch op {
	case "insert":
		return "create"
	case "update":
		return "update"
	case "delete":
		return "delete"
	default:
		return ""
	}
}

func mutationPKMap(tbl *schema.Table, row map[string]any) map[string]any {
	out := make(map[string]any, len(tbl.PrimaryKey))
	for _, key := range tbl.PrimaryKey {
		out[key] = row[key]
	}
	return out
}

func mutationRowsByPK(tbl *schema.Table, rows []map[string]any) map[string]map[string]any {
	index := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		key, ok := mutationRowKey(tbl, row)
		if !ok {
			continue
		}
		index[key] = row
	}
	return index
}

func mutationRowKey(tbl *schema.Table, row map[string]any) (string, bool) {
	if tbl == nil || len(tbl.PrimaryKey) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(tbl.PrimaryKey))
	for _, key := range tbl.PrimaryKey {
		value, ok := row[key]
		if !ok {
			return "", false
		}
		parts = append(parts, fmt.Sprintf("%T:%v", value, value))
	}
	return strings.Join(parts, "|"), true
}
