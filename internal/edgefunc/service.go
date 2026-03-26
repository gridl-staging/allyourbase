package edgefunc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// DeployOptions configures a function deployment.
type DeployOptions struct {
	EntryPoint   string
	TimeoutMs    int
	EnvVars      map[string]string
	Public       bool
	IsTypeScript bool
}

// VaultSecretProvider loads all vault secrets as decrypted key-value pairs.
// Used to inject vault secrets into edge function env vars at invocation time.
type VaultSecretProvider interface {
	GetAllSecretsDecrypted(ctx context.Context) (map[string]string, error)
}

// Service orchestrates edge function deployment, invocation, and lifecycle management.
// It wires together Store (persistence), Pool (execution), and LogStore (logging).
type Service struct {
	store                  Store
	pool                   *Pool
	logStore               LogStore
	queryExecutor          QueryExecutor
	vaultProvider          VaultSecretProvider
	invocationLogWriter    InvocationLogWriter
	defaultTimeout         time.Duration
	functionInvokerEnabled bool
}

// InvocationLogWriter receives a compact invocation summary after each execution.
type InvocationLogWriter interface {
	WriteLog(ctx context.Context, functionName, invocationID, status string, durationMs int, stdout, errMsg string)
}

// ServiceOption customizes Service behavior.
type ServiceOption func(*Service)

// WithServiceQueryExecutor enables ayb.db execution for function invocations.
func WithServiceQueryExecutor(qe QueryExecutor) ServiceOption {
	return func(s *Service) {
		s.queryExecutor = qe
	}
}

// WithDefaultTimeout sets the default timeout used for new deployments when
// DeployOptions.TimeoutMs is not provided.
func WithDefaultTimeout(timeout time.Duration) ServiceOption {
	return func(s *Service) {
		if timeout > 0 {
			s.defaultTimeout = timeout
		}
	}
}

// WithVaultProvider sets the vault secret provider for injecting vault secrets
// into edge function env vars at invocation time.
func WithVaultProvider(vp VaultSecretProvider) ServiceOption {
	return func(s *Service) {
		s.vaultProvider = vp
	}
}

// WithServiceFunctionInvoker toggles the ayb.functions.invoke bridge during execution.
// When disabled, ayb.functions is omitted from the VM (used by nil-invoker tests and constrained runtimes).
func WithServiceFunctionInvoker(enabled bool) ServiceOption {
	return func(s *Service) {
		s.functionInvokerEnabled = enabled
	}
}

// WithInvocationLogWriter configures a callback that receives edge function
// invocation summaries for external fan-out (for example log drains).
func WithInvocationLogWriter(writer InvocationLogWriter) ServiceOption {
	return func(s *Service) {
		s.invocationLogWriter = writer
	}
}

// NewService creates a Service with the given dependencies.
func NewService(store Store, pool *Pool, logStore LogStore, opts ...ServiceOption) *Service {
	svc := &Service{
		store:                  store,
		pool:                   pool,
		logStore:               logStore,
		defaultTimeout:         DefaultTimeout,
		functionInvokerEnabled: true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

// SetInvocationLogWriter configures or updates the callback receiving invocation
// summaries. Safe to call any time before first use.
func (s *Service) SetInvocationLogWriter(writer InvocationLogWriter) {
	s.invocationLogWriter = writer
}

// Deploy validates, transpiles (if TS), compiles, and persists a new edge function.
// Pipeline: validate → transpile (if TS) → compile goja.Program → persist to Store.
func (s *Service) Deploy(ctx context.Context, name, source string, opts DeployOptions) (*EdgeFunction, error) {
	entryPoint := opts.EntryPoint
	if entryPoint == "" {
		entryPoint = "handler"
	}

	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = s.defaultTimeout
	}

	fn := &EdgeFunction{
		Name:       name,
		EntryPoint: entryPoint,
		Source:     source,
		Timeout:    timeout,
		EnvVars:    opts.EnvVars,
		Public:     opts.Public,
	}

	if err := fn.Validate(); err != nil {
		return nil, fmt.Errorf("validating edge function: %w", err)
	}

	compiledJS, err := s.prepareCompiledJS(name, source, entryPoint, opts.IsTypeScript)
	if err != nil {
		return nil, err
	}
	fn.CompiledJS = compiledJS

	created, err := s.store.Create(ctx, fn)
	if err != nil {
		return nil, err
	}

	return created, nil
}

// Invoke loads and executes a function by name, logging the result.
func (s *Service) Invoke(ctx context.Context, name string, req Request) (Response, error) {
	fn, err := s.store.GetByName(ctx, name)
	if err != nil {
		return Response{}, err
	}
	return s.invokeFunction(ctx, fn, req)
}

// InvokeByID loads and executes a function by ID, logging the result.
// Implements FunctionInvoker for use by cron triggers and function-to-function calls.
func (s *Service) InvokeByID(ctx context.Context, functionID string, req Request) (Response, error) {
	id, err := uuid.Parse(functionID)
	if err != nil {
		return Response{}, fmt.Errorf("invalid function ID: %w", err)
	}
	fn, err := s.store.Get(ctx, id)
	if err != nil {
		return Response{}, err
	}
	return s.invokeFunction(ctx, fn, req)
}

// invokeFunction executes a loaded function, logging the result.
func (s *Service) invokeFunction(ctx context.Context, fn *EdgeFunction, req Request) (Response, error) {
	invocationID := uuid.New()
	start := time.Now()

	timeout := fn.EffectiveTimeout()
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var namedInvoker NamedInvoker
	if s.functionInvokerEnabled {
		// Build a NamedInvoker that increments depth and checks budget for nested calls.
		currentDepth := InvocationDepth(ctx)
		namedInvoker = NamedInvoker(func(nestedCtx context.Context, name string, nestedReq Request) (Response, error) {
			if err := checkInvocationBudget(nestedCtx, MaxInvocationDepth); err != nil {
				return Response{}, err
			}
			childCtx := WithInvocationDepth(nestedCtx, currentDepth+1)
			childCtx = WithParentInvocationID(childCtx, invocationID.String())
			return s.Invoke(childCtx, name, nestedReq)
		})
	}

	// Merge vault secrets with function env vars (function env vars take precedence).
	envVars := fn.EnvVars
	if s.vaultProvider != nil {
		vaultSecrets, vaultErr := s.vaultProvider.GetAllSecretsDecrypted(ctx)
		if vaultErr != nil {
			slog.Warn("failed to load vault secrets for edge function", "function", fn.Name, "error", vaultErr)
		} else if len(vaultSecrets) > 0 {
			merged := make(map[string]string, len(vaultSecrets)+len(envVars))
			for k, v := range vaultSecrets {
				merged[k] = v
			}
			for k, v := range envVars {
				merged[k] = v
			}
			envVars = merged
		}
	}

	resp, execErr := s.pool.Execute(execCtx, fn.CompiledJS, fn.EntryPoint, req, envVars, s.queryExecutor, namedInvoker)

	durationMs := int(time.Since(start).Milliseconds())

	entry := &LogEntry{
		FunctionID:         fn.ID,
		InvocationID:       invocationID,
		DurationMs:         durationMs,
		RequestMethod:      req.Method,
		RequestPath:        req.Path,
		ParentInvocationID: ParentInvocationID(ctx),
	}

	if meta, ok := GetTriggerMeta(ctx); ok {
		entry.TriggerType = string(meta.Type)
		entry.TriggerID = meta.ID
	}

	if execErr != nil {
		entry.Status = "error"
		entry.Error = execErr.Error()
		entry.Stdout = resp.Stdout
	} else {
		entry.Status = "success"
		entry.Stdout = resp.Stdout
	}
	entry.StdoutBytes = len(entry.Stdout)
	entry.ResponseStatusCode = resp.StatusCode

	if logErr := s.logStore.WriteLog(ctx, entry); logErr != nil {
		slog.Error("failed to write edge function log", "function", fn.Name, "invocation_id", invocationID, "error", logErr)
	}

	if s.invocationLogWriter != nil {
		s.invocationLogWriter.WriteLog(ctx, fn.Name, invocationID.String(), entry.Status, durationMs, entry.Stdout, entry.Error)
	}

	if execErr != nil {
		return Response{}, execErr
	}

	return resp, nil
}

// Get returns a function by ID.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*EdgeFunction, error) {
	return s.store.Get(ctx, id)
}

// GetByName returns a function by name.
func (s *Service) GetByName(ctx context.Context, name string) (*EdgeFunction, error) {
	return s.store.GetByName(ctx, name)
}

// List returns functions with pagination.
func (s *Service) List(ctx context.Context, page, perPage int) ([]*EdgeFunction, error) {
	functions, err := s.store.List(ctx, page, perPage)
	if err != nil {
		return nil, err
	}
	if s.logStore == nil {
		return functions, nil
	}

	for _, fn := range functions {
		logs, logErr := s.logStore.ListByFunction(ctx, fn.ID, LogListOptions{Page: 1, PerPage: 1})
		if logErr != nil || len(logs) == 0 {
			fn.LastInvokedAt = nil
			continue
		}
		lastInvokedAt := logs[0].CreatedAt
		fn.LastInvokedAt = &lastInvokedAt
	}

	return functions, nil
}

// Update re-deploys a function with new source and options.
// Re-transpiles (if TS), re-compiles the goja.Program, and updates the store.
func (s *Service) Update(ctx context.Context, id uuid.UUID, source string, opts DeployOptions) (*EdgeFunction, error) {
	existing, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	entryPoint := opts.EntryPoint
	if entryPoint == "" {
		entryPoint = existing.EntryPoint
	}

	compiledJS, err := s.prepareCompiledJS(existing.Name, source, entryPoint, opts.IsTypeScript)
	if err != nil {
		return nil, err
	}

	existing.Source = source
	existing.CompiledJS = compiledJS
	existing.EntryPoint = entryPoint
	existing.Public = opts.Public
	if opts.TimeoutMs > 0 {
		existing.Timeout = time.Duration(opts.TimeoutMs) * time.Millisecond
	}
	if opts.EnvVars != nil {
		existing.EnvVars = opts.EnvVars
	}

	return s.store.Update(ctx, existing)
}

// prepareCompiledJS returns executable JS and validates it through goja compilation.
// If IsTypeScript is false, it first tries raw JS compilation and then falls back to
// TS transpilation when raw parsing fails. This keeps JS passthrough behavior while
// allowing TS source from clients that omit the explicit TS flag.
func (s *Service) prepareCompiledJS(name, source, entryPoint string, isTypeScript bool) (string, error) {
	if isTypeScript {
		compiledJS, err := Transpile(source, true, entryPoint)
		if err != nil {
			return "", err
		}
		if _, err := s.pool.Compile(name, compiledJS); err != nil {
			return "", err
		}
		return compiledJS, nil
	}

	if _, err := s.pool.Compile(name, source); err == nil {
		return source, nil
	} else {
		compiledJS, transpileErr := Transpile(source, true, entryPoint)
		if transpileErr != nil {
			return "", transpileErr
		}
		if _, compileErr := s.pool.Compile(name, compiledJS); compileErr != nil {
			return "", compileErr
		}
		return compiledJS, nil
	}
}

// Delete removes a function by ID.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.store.Delete(ctx, id)
}

// ListLogs returns execution logs for a function (paginated, newest first).
func (s *Service) ListLogs(ctx context.Context, functionID uuid.UUID, opts LogListOptions) ([]*LogEntry, error) {
	if _, err := s.store.Get(ctx, functionID); err != nil {
		return nil, err
	}

	normalized, err := normalizeLogListOptions(opts)
	if err != nil {
		return nil, err
	}

	logs, err := s.logStore.ListByFunction(ctx, functionID, normalized)
	if err != nil {
		return nil, err
	}
	if logs == nil {
		return []*LogEntry{}, nil
	}
	return logs, nil
}
