// Package edgefunc provides functions for validating and normalizing log list options for edge function execution logs.
package edgefunc

import (
	"fmt"
	"strings"
)

// NormalizeLogListOptions validates and normalizes a LogListOptions struct. It sets Page to 1 if not positive, defaults PerPage to 50 and caps it at 1000, normalizes and validates the Status field (must be empty, success, or error), normalizes and validates the TriggerType field, and verifies that Since is before or equal to Until. It returns ErrInvalidLogFilter if any validation fails.
func NormalizeLogListOptions(opts LogListOptions) (LogListOptions, error) {
	if opts.Page < 0 {
		return LogListOptions{}, fmt.Errorf("%w: page must be >= 0", ErrInvalidLogFilter)
	}
	if opts.PerPage < 0 {
		return LogListOptions{}, fmt.Errorf("%w: perPage must be >= 0", ErrInvalidLogFilter)
	}

	if opts.Page <= 0 {
		opts.Page = 1
	}
	switch {
	case opts.PerPage <= 0:
		opts.PerPage = 50
	case opts.PerPage > 1000:
		opts.PerPage = 1000
	}

	opts.Status = strings.ToLower(strings.TrimSpace(opts.Status))
	if opts.Status != "" && opts.Status != "success" && opts.Status != "error" {
		return LogListOptions{}, fmt.Errorf("%w: status must be one of success,error", ErrInvalidLogFilter)
	}

	opts.TriggerType = strings.ToLower(strings.TrimSpace(opts.TriggerType))
	if opts.TriggerType != "" && !isValidTriggerType(opts.TriggerType) {
		return LogListOptions{}, fmt.Errorf("%w: trigger_type must be one of http,db,cron,storage,function", ErrInvalidLogFilter)
	}

	if opts.Since != nil && opts.Until != nil && opts.Since.After(*opts.Until) {
		return LogListOptions{}, fmt.Errorf("%w: since must be before or equal to until", ErrInvalidLogFilter)
	}

	return opts, nil
}

func normalizeLogListOptions(opts LogListOptions) (LogListOptions, error) {
	return NormalizeLogListOptions(opts)
}

func isValidTriggerType(triggerType string) bool {
	switch TriggerType(triggerType) {
	case TriggerHTTP, TriggerDB, TriggerCron, TriggerStorage, TriggerFunction:
		return true
	default:
		return false
	}
}
