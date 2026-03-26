package server

import (
	"context"
	"fmt"
	"time"

	"github.com/allyourbase/ayb/internal/logging"
)

// edgeFuncDrainWriter forwards edge function invocation results to log drains
// so that edge function output appears alongside app and request logs in external
// observability systems.
type edgeFuncDrainWriter struct {
	manager         *logging.DrainManager
	managerProvider func() *logging.DrainManager
}

// WriteLog enqueues an edge function invocation log into the drain manager.
func (w *edgeFuncDrainWriter) WriteLog(_ context.Context, functionName, invocationID, status string, durationMs int, stdout, errMsg string) {
	manager := w.manager
	if manager == nil && w.managerProvider != nil {
		manager = w.managerProvider()
	}
	if manager == nil {
		return
	}

	level := "info"
	if status == "error" {
		level = "error"
	}

	fields := map[string]any{
		"function":      functionName,
		"invocation_id": invocationID,
		"status":        status,
		"duration_ms":   durationMs,
	}
	if stdout != "" {
		fields["stdout"] = stdout
	}
	if errMsg != "" {
		fields["error"] = errMsg
	}

	manager.Enqueue(logging.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   fmt.Sprintf("edge_function:%s", functionName),
		Source:    "edge_function",
		Fields:    fields,
	})
}
