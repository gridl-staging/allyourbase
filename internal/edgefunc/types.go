package edgefunc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultTimeout is used when an edge function does not define a custom timeout.
	DefaultTimeout = 5 * time.Second
)

var (
	ErrNameRequired       = errors.New("edge function name is required")
	ErrEntryPointRequired = errors.New("edge function entry point is required")
	ErrNegativeTimeout    = errors.New("edge function timeout must be non-negative")
	ErrSourceRequired     = errors.New("edge function source or source path is required")
)

// Runtime executes edge function source code against a request payload.
type Runtime interface {
	Execute(ctx context.Context, code string, entryPoint string, request Request) (Response, error)
}

// Request is the HTTP-like payload provided to an edge function execution.
type Request struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Query   string              `json:"query,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    []byte              `json:"body,omitempty"`
}

// Response is the execution result produced by an edge function runtime.
type Response struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       []byte              `json:"body,omitempty"`
	Stdout     string              `json:"stdout,omitempty"`
}

// EdgeFunction stores metadata and configuration for a deployed function.
type EdgeFunction struct {
	ID            uuid.UUID         `json:"id"`
	Name          string            `json:"name"`
	EntryPoint    string            `json:"entryPoint"`
	Timeout       time.Duration     `json:"timeout"`
	LastInvokedAt *time.Time        `json:"lastInvokedAt,omitempty"`
	EnvVars       map[string]string `json:"envVars,omitempty"`
	Public        bool              `json:"public"`
	Source        string            `json:"source"`
	CompiledJS    string            `json:"compiledJs"`
	SourcePath    string            `json:"sourcePath"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// EffectiveTimeout returns the configured timeout or the runtime default.
func (f EdgeFunction) EffectiveTimeout() time.Duration {
	if f.Timeout <= 0 {
		return DefaultTimeout
	}
	return f.Timeout
}

// Validate checks that required edge function metadata is present.
func (f EdgeFunction) Validate() error {
	if strings.TrimSpace(f.Name) == "" {
		return ErrNameRequired
	}
	if strings.TrimSpace(f.EntryPoint) == "" {
		return ErrEntryPointRequired
	}
	if f.Timeout < 0 {
		return ErrNegativeTimeout
	}
	if strings.TrimSpace(f.Source) == "" && strings.TrimSpace(f.SourcePath) == "" {
		return ErrSourceRequired
	}
	return nil
}
