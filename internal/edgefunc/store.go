package edgefunc

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Sentinel errors for store operations.
var (
	ErrFunctionNotFound     = errors.New("edge function not found")
	ErrFunctionNameConflict = errors.New("edge function name already exists")
)

// Store defines CRUD operations for edge function persistence.
type Store interface {
	Create(ctx context.Context, fn *EdgeFunction) (*EdgeFunction, error)
	Get(ctx context.Context, id uuid.UUID) (*EdgeFunction, error)
	GetByName(ctx context.Context, name string) (*EdgeFunction, error)
	List(ctx context.Context, page, perPage int) ([]*EdgeFunction, error)
	Update(ctx context.Context, fn *EdgeFunction) (*EdgeFunction, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
