package matview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// matviewRefreshPayload is the expected payload for materialized_view_refresh jobs.
type matviewRefreshPayload struct {
	Schema   string `json:"schema"`
	ViewName string `json:"view_name"`
}

// registrationLookup is the subset of Store used by the handler for name-to-ID resolution.
type registrationLookup interface {
	GetByName(ctx context.Context, schemaName, viewName string) (*Registration, error)
	Register(ctx context.Context, schemaName, viewName string, mode RefreshMode) (*Registration, error)
}

// MatviewRefreshHandler returns a job handler that refreshes a registered materialized view.
// The payload must contain {"view_name": "..."} and optionally {"schema": "..."} (defaults to "public").
// If no registration exists for the view, one is auto-created with standard mode.
func MatviewRefreshHandler(svc *Service, lookup registrationLookup) func(ctx context.Context, payload json.RawMessage) error {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p matviewRefreshPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("materialized_view_refresh: invalid payload: %w", err)
		}
		if p.ViewName == "" {
			return fmt.Errorf("materialized_view_refresh: view_name is required in payload")
		}
		if p.Schema == "" {
			p.Schema = "public"
		}

		reg, err := lookup.GetByName(ctx, p.Schema, p.ViewName)
		if err != nil {
			if !errors.Is(err, ErrRegistrationNotFound) {
				return fmt.Errorf("materialized_view_refresh: lookup %s.%s: %w", p.Schema, p.ViewName, err)
			}
			// Auto-register if not found
			reg, err = lookup.Register(ctx, p.Schema, p.ViewName, RefreshModeStandard)
			if err != nil {
				// Another worker may have registered the view concurrently.
				// Retry lookup on duplicate registration to avoid failing the job.
				if errors.Is(err, ErrDuplicateRegistration) {
					reg, err = lookup.GetByName(ctx, p.Schema, p.ViewName)
					if err != nil {
						return fmt.Errorf("materialized_view_refresh: lookup after duplicate %s.%s: %w", p.Schema, p.ViewName, err)
					}
				} else {
					return fmt.Errorf("materialized_view_refresh: auto-register %s.%s: %w", p.Schema, p.ViewName, err)
				}
			}
		}

		_, err = svc.RefreshNow(ctx, reg.ID)
		return err
	}
}
