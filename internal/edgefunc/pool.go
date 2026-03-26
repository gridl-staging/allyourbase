// Package edgefunc implements a bounded-concurrency pool for executing JavaScript edge functions in isolated goja VMs with program caching and bridges to database, HTTP, authentication, and AI services.
package edgefunc

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dop251/goja"

	"github.com/allyourbase/ayb/internal/schema"
)

// ProviderTokenGetter fetches a provider access token by user and provider.
// Implemented by the auth service for ayb.auth.getProviderToken() bridge.
type ProviderTokenGetter func(ctx context.Context, userID, provider string) (string, error)

// Pool manages bounded-concurrency execution of edge functions with compiled
// program caching. A channel-based semaphore limits concurrent VM count.
// Each invocation gets a fresh VM (with frozen built-ins) for full isolation,
// while *goja.Program caching avoids repeated parse/compile overhead.
// Pool manages bounded-concurrency execution of edge functions with compiled program caching. It uses a channel-based semaphore to limit concurrent VM instances, creates a fresh goja.Runtime for each invocation with frozen built-ins for isolation, and maintains an LRU cache of compiled goja.Programs to avoid repeated parse/compile overhead.
type Pool struct {
	sem  chan struct{}
	size int

	maxConcurrent int
	inFlight      atomic.Int64

	memoryLimitMB    int
	stdoutLimitBytes int

	// Program cache: content hash → compiled program
	cacheMu        sync.RWMutex
	programs       map[string]*goja.Program
	cacheSizeLimit int
	cacheAccessLRU *list.List
	cacheLRUByHash map[string]*list.Element

	// HTTP client for fetch() bridge (shared, stateless).
	// Defaults to SSRF-safe client that blocks private/reserved IPs.
	httpClient          *http.Client
	providerTokenGetter ProviderTokenGetter

	// AI bridge functions — nil when AI subsystem not wired.
	aiGenerate      AIGenerateFunc
	aiRenderPrompt  AIRenderPromptFunc
	aiParseDocument AIParseDocumentFunc

	// Email bridge function — nil when email subsystem not wired.
	emailSend EmailSendFunc

	// Schema cache getter used by ayb.spatial bridge setup.
	schemaCacheGetter func() *schema.SchemaCache

	closed atomic.Bool
}

const (
	defaultMaxConcurrentInvocations = 50
	defaultCodeCacheSize            = 256
	defaultMemoryLimitMB            = 128
	minStdoutCaptureBytes           = 64 * 1024
	maxStdoutCaptureBytes           = 1 << 20
	stdoutTruncationMarker          = "[stdout truncated]\n"
)

var ErrConcurrencyLimitExceeded = errors.New("edge function concurrency limit exceeded")

// PoolOption customizes pool behavior.
type PoolOption func(*Pool)

// WithPoolHTTPClient overrides the HTTP client used by fetch().
func WithPoolHTTPClient(client *http.Client) PoolOption {
	return func(p *Pool) {
		if client != nil {
			p.httpClient = client
		}
	}
}

// WithPoolProviderTokenGetter wires a provider token fetch function for
// ayb.auth.getProviderToken() in the edgefunction runtime.
func WithPoolProviderTokenGetter(getter ProviderTokenGetter) PoolOption {
	return func(p *Pool) {
		p.providerTokenGetter = getter
	}
}

// WithPoolAIGenerate wires the AI text generation function for ayb.ai.generateText().
func WithPoolAIGenerate(fn AIGenerateFunc) PoolOption {
	return func(p *Pool) { p.aiGenerate = fn }
}

// WithPoolAIRenderPrompt wires the prompt rendering function for ayb.ai.renderPrompt().
func WithPoolAIRenderPrompt(fn AIRenderPromptFunc) PoolOption {
	return func(p *Pool) { p.aiRenderPrompt = fn }
}

// WithPoolAIParseDocument wires the document parsing function for ayb.ai.parseDocument().
func WithPoolAIParseDocument(fn AIParseDocumentFunc) PoolOption {
	return func(p *Pool) { p.aiParseDocument = fn }
}

// SetAIGenerate sets the AI text generation function after pool construction.
func (p *Pool) SetAIGenerate(fn AIGenerateFunc) { p.aiGenerate = fn }

// SetAIRenderPrompt sets the prompt rendering function after pool construction.
func (p *Pool) SetAIRenderPrompt(fn AIRenderPromptFunc) { p.aiRenderPrompt = fn }

// SetAIParseDocument sets the document parsing function after pool construction.
func (p *Pool) SetAIParseDocument(fn AIParseDocumentFunc) { p.aiParseDocument = fn }

// WithPoolEmailSend wires the email send function for ayb.email.send().
func WithPoolEmailSend(fn EmailSendFunc) PoolOption {
	return func(p *Pool) { p.emailSend = fn }
}

// WithPoolSchemaCache wires a schema cache getter for the ayb.spatial bridge.
func WithPoolSchemaCache(getter func() *schema.SchemaCache) PoolOption {
	return func(p *Pool) {
		if getter != nil {
			p.schemaCacheGetter = getter
		}
	}
}

// WithPoolMaxConcurrentInvocations configures the non-blocking admission cap.
func WithPoolMaxConcurrentInvocations(limit int) PoolOption {
	return func(p *Pool) {
		if limit >= 1 {
			p.maxConcurrent = limit
		}
	}
}

// WithPoolCodeCacheSize configures the max number of compiled programs kept in cache.
func WithPoolCodeCacheSize(limit int) PoolOption {
	return func(p *Pool) {
		if limit >= 1 {
			p.cacheSizeLimit = limit
		}
	}
}

// WithPoolMemoryLimitMB configures the best-effort memory guard settings.
func WithPoolMemoryLimitMB(limit int) PoolOption {
	return func(p *Pool) {
		if limit >= 1 {
			p.memoryLimitMB = limit
			p.stdoutLimitBytes = stdoutLimitBytesForMemoryLimit(limit)
		}
	}
}

// SetEmailSend sets the email send function after pool construction.
func (p *Pool) SetEmailSend(fn EmailSendFunc) { p.emailSend = fn }

// NewPool creates a pool with the given max concurrency.
// The pool uses an SSRF-safe HTTP client by default.
func NewPool(size int, opts ...PoolOption) *Pool {
	if size < 1 {
		size = 1
	}
	p := &Pool{
		sem:              make(chan struct{}, size),
		size:             size,
		maxConcurrent:    defaultMaxConcurrentInvocations,
		memoryLimitMB:    defaultMemoryLimitMB,
		stdoutLimitBytes: stdoutLimitBytesForMemoryLimit(defaultMemoryLimitMB),
		programs:         make(map[string]*goja.Program),
		cacheSizeLimit:   defaultCodeCacheSize,
		cacheAccessLRU:   list.New(),
		cacheLRUByHash:   make(map[string]*list.Element),
		httpClient:       NewSSRFSafeClient(nil),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	if p.maxConcurrent < p.size {
		p.maxConcurrent = p.size
	}
	if p.cacheSizeLimit < 1 {
		p.cacheSizeLimit = 1
	}
	if p.memoryLimitMB < 1 {
		p.memoryLimitMB = 1
	}
	if p.stdoutLimitBytes < 1 {
		p.stdoutLimitBytes = stdoutLimitBytesForMemoryLimit(p.memoryLimitMB)
	}
	// Pre-fill the semaphore
	for i := 0; i < size; i++ {
		p.sem <- struct{}{}
	}
	return p
}

// Size returns the pool's max concurrency.
func (p *Pool) Size() int {
	return p.size
}

// Close marks the pool as closed. Idempotent.
// After Close, new Execute calls will fail and in-flight invocations
// will safely release their semaphore slots.
func (p *Pool) Close() {
	p.closed.Store(true)
}

// Compile compiles JS source into a reusable *goja.Program, cached by content hash.
// Programs are safe to share across VMs; they contain parsed bytecode only.
func (p *Pool) Compile(name, source string) (*goja.Program, error) {
	hash := contentHash(source)

	p.cacheMu.Lock()
	if prog, ok := p.programs[hash]; ok {
		p.touchProgramLRULocked(hash)
		p.cacheMu.Unlock()
		return prog, nil
	}
	p.cacheMu.Unlock()

	wrapped := wrapStrict(source)
	prog, err := goja.Compile(name, wrapped, true)
	if err != nil {
		return nil, fmt.Errorf("compiling edge function %q: %w", name, err)
	}

	p.cacheMu.Lock()
	if existing, ok := p.programs[hash]; ok {
		p.touchProgramLRULocked(hash)
		p.cacheMu.Unlock()
		return existing, nil
	}
	if len(p.programs) >= p.cacheSizeLimit {
		p.evictLRULocked()
	}
	p.programs[hash] = prog
	p.touchProgramLRULocked(hash)
	p.cacheMu.Unlock()

	return prog, nil
}

// NamedInvoker invokes an edge function by name. Used by the ayb.functions.invoke() VM bridge.
type NamedInvoker func(ctx context.Context, name string, req Request) (Response, error)

// Execute compiles code (with caching) and runs it.
func (p *Pool) Execute(ctx context.Context, code, entryPoint string, req Request, envVars map[string]string, qe QueryExecutor, ni ...NamedInvoker) (Response, error) {
	prog, err := p.Compile("inline", code)
	if err != nil {
		return Response{}, err
	}
	var invoker NamedInvoker
	if len(ni) > 0 {
		invoker = ni[0]
	}
	return p.ExecuteProgram(ctx, prog, entryPoint, req, envVars, qe, invoker)
}

// ExecuteProgram runs a pre-compiled program with full isolation.
// Each invocation gets a fresh VM with frozen built-ins.
func (p *Pool) ExecuteProgram(ctx context.Context, prog *goja.Program, entryPoint string, req Request, envVars map[string]string, qe QueryExecutor, ni ...NamedInvoker) (Response, error) {
	if entryPoint == "" {
		entryPoint = "handler"
	}

	if err := p.TryAcquire(ctx); err != nil {
		return Response{}, err
	}
	defer p.releaseInvocation()

	// Acquire concurrency slot
	if err := p.acquire(ctx); err != nil {
		return Response{}, err
	}
	defer p.release()

	// Fresh VM per invocation — full isolation, no global leaks between calls.
	// The *goja.Program cache provides the performance win (skip parse/compile).
	vm := createVM(p.memoryLimitMB)

	// Wire context cancellation → VM interrupt for timeout support.
	if ctx.Done() != nil {
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				vm.Interrupt("execution timeout")
			case <-done:
			}
		}()
	}

	// Per-invocation console.log capture
	var stdout strings.Builder
	if err := setupConsole(vm, &stdout, p.stdoutLimitBytes); err != nil {
		return Response{}, err
	}

	// fetch() bridge
	if err := registerFetch(vm, ctx, p.httpClient); err != nil {
		return Response{}, fmt.Errorf("registering fetch: %w", err)
	}

	// ayb namespace (env vars + db bridge + functions)
	var invoker NamedInvoker
	if len(ni) > 0 {
		invoker = ni[0]
	}
	if err := setupAYB(vm, ctx, BridgeConfig{
		EnvVars:             envVars,
		QueryExecutor:       qe,
		SchemaCacheGetter:   p.schemaCacheGetter,
		NamedInvoker:        invoker,
		ProviderTokenGetter: p.providerTokenGetter,
		AIGenerate:          p.aiGenerate,
		AIRenderPrompt:      p.aiRenderPrompt,
		AIParseDocument:     p.aiParseDocument,
		EmailSend:           p.emailSend,
	}); err != nil {
		return Response{}, err
	}

	// Run compiled program (defines handler in VM scope)
	if _, err := vm.RunProgram(prog); err != nil {
		return Response{Stdout: stdout.String()}, fmt.Errorf("executing edge function: %w", err)
	}

	handler, ok := goja.AssertFunction(vm.Get(entryPoint))
	if !ok {
		return Response{Stdout: stdout.String()}, fmt.Errorf("edge function must export a '%s' function", entryPoint)
	}

	reqObj := buildRequestObject(vm, req)

	result, err := handler(goja.Undefined(), reqObj)
	if err != nil {
		return Response{Stdout: stdout.String()}, fmt.Errorf("executing handler: %w", err)
	}

	result, err = unwrapPromise(result)
	if err != nil {
		return Response{Stdout: stdout.String()}, err
	}

	resp, err := gojaResultToResponse(result, stdout.String())
	if err != nil {
		return Response{Stdout: stdout.String()}, err
	}

	return resp, nil
}

// TryAcquire performs non-blocking admission control before semaphore acquisition.
func (p *Pool) TryAcquire(ctx context.Context) error {
	if p.closed.Load() {
		return fmt.Errorf("pool is closed")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("acquiring invocation slot: %w", err)
	}
	limit := int64(p.maxConcurrent)
	for {
		current := p.inFlight.Load()
		if current >= limit {
			return ErrConcurrencyLimitExceeded
		}
		if p.inFlight.CompareAndSwap(current, current+1) {
			return nil
		}
	}
}

// acquire blocks until a concurrency slot is available or ctx is cancelled.
func (p *Pool) acquire(ctx context.Context) error {
	if p.closed.Load() {
		return fmt.Errorf("pool is closed")
	}
	select {
	case <-p.sem:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("acquiring VM: %w", ctx.Err())
	}
}

// release returns a concurrency slot to the semaphore.
// Safe to call even after Close() — the channel remains open (not closed),
// so sends never panic.
func (p *Pool) release() {
	p.sem <- struct{}{}
}

func (p *Pool) releaseInvocation() {
	for {
		current := p.inFlight.Load()
		if current <= 0 {
			return
		}
		if p.inFlight.CompareAndSwap(current, current-1) {
			return
		}
	}
}
