package matview

import "errors"

var (
	ErrInvalidIdentifier                  = errors.New("invalid identifier")
	ErrInvalidRefreshMode                 = errors.New("invalid refresh mode")
	ErrRegistrationNotFound               = errors.New("materialized view registration not found")
	ErrNotMaterializedView                = errors.New("materialized view not found")
	ErrRefreshInProgress                  = errors.New("refresh already in progress")
	ErrConcurrentRefreshRequiresIndex     = errors.New("concurrent refresh requires a unique index")
	ErrConcurrentRefreshRequiresPopulated = errors.New("concurrent refresh requires a populated materialized view")
	ErrDuplicateRegistration              = errors.New("materialized view already registered")
)
