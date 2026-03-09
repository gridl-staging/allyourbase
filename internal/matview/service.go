package matview

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type refresherStore interface {
	Get(ctx context.Context, id string) (*Registration, error)
	MatviewState(ctx context.Context, schemaName, viewName string) (exists bool, populated bool, err error)
	HasConcurrentUniqueIndex(ctx context.Context, schemaName, viewName string) (bool, error)
	LockedRefresh(ctx context.Context, lockKey, sql string) error
	UpdateRefreshStatus(ctx context.Context, id string, status RefreshStatus, durationMs int, errText *string) error
}

// Service runs manual refresh operations with safety checks and status updates.
type Service struct {
	store refresherStore
	now   func() time.Time
}

// NewService creates a materialized view refresh service.
func NewService(store refresherStore) *Service {
	return &Service{store: store, now: time.Now}
}

// RefreshNow refreshes a registered materialized view synchronously.
func (s *Service) RefreshNow(ctx context.Context, id string) (*RefreshResult, error) {
	reg, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	exists, populated, err := s.store.MatviewState(ctx, reg.SchemaName, reg.ViewName)
	if err != nil {
		return nil, err
	}
	if !exists {
		runErr := fmt.Errorf("%w: %s.%s", ErrNotMaterializedView, reg.SchemaName, reg.ViewName)
		s.recordError(ctx, reg.ID, 0, runErr)
		return nil, runErr
	}

	if reg.RefreshMode == RefreshModeConcurrent {
		if !populated {
			runErr := ErrConcurrentRefreshRequiresPopulated
			s.recordError(ctx, reg.ID, 0, runErr)
			return nil, runErr
		}
		hasIndex, idxErr := s.store.HasConcurrentUniqueIndex(ctx, reg.SchemaName, reg.ViewName)
		if idxErr != nil {
			s.recordError(ctx, reg.ID, 0, idxErr)
			return nil, idxErr
		}
		if !hasIndex {
			runErr := ErrConcurrentRefreshRequiresIndex
			s.recordError(ctx, reg.ID, 0, runErr)
			return nil, runErr
		}
	}

	sql, err := BuildRefreshSQL(reg.SchemaName, reg.ViewName, reg.RefreshMode)
	if err != nil {
		s.recordError(ctx, reg.ID, 0, err)
		return nil, err
	}

	lockKey := reg.SchemaName + "." + reg.ViewName
	started := s.now()

	if err := s.store.LockedRefresh(ctx, lockKey, sql); err != nil {
		dur := int(time.Since(started).Milliseconds())
		if errors.Is(err, ErrRefreshInProgress) {
			s.recordError(ctx, reg.ID, 0, err)
		} else {
			s.recordError(ctx, reg.ID, dur, err)
		}
		return nil, err
	}

	dur := int(time.Since(started).Milliseconds())
	if err := s.store.UpdateRefreshStatus(ctx, reg.ID, RefreshStatusSuccess, dur, nil); err != nil {
		return nil, err
	}

	return &RefreshResult{Registration: *reg, DurationMs: dur}, nil
}

func (s *Service) recordError(ctx context.Context, id string, durationMs int, runErr error) {
	msg := runErr.Error()
	_ = s.store.UpdateRefreshStatus(ctx, id, RefreshStatusError, durationMs, &msg)
}
