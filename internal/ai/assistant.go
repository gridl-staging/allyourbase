// Package ai Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/ai/assistant.go.
package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/google/uuid"
)

// AssistantMode controls which built-in assistant prompt template is used.
type AssistantMode string

const (
	AssistantModeSQL       AssistantMode = "sql"
	AssistantModeRLS       AssistantMode = "rls"
	AssistantModeMigration AssistantMode = "migration"
	AssistantModeGeneral   AssistantMode = "general"
)

// AssistantStatus captures execution status for history entries.
type AssistantStatus string

const (
	AssistantStatusSuccess   AssistantStatus = "success"
	AssistantStatusError     AssistantStatus = "error"
	AssistantStatusCancelled AssistantStatus = "cancelled"
)

var (
	// ErrAssistantDisabled indicates dashboard AI assistant endpoints are disabled.
	ErrAssistantDisabled = errors.New("dashboard ai assistant is disabled")
	// ErrAssistantNotConfigured indicates no AI providers are configured for assistant use.
	ErrAssistantNotConfigured = errors.New("dashboard ai assistant is not configured")
	// ErrAssistantSchemaCacheNotReady indicates the schema cache has not finished loading yet.
	ErrAssistantSchemaCacheNotReady = errors.New("schema cache not ready")
)

// AssistantRequest is a single assistant query request.
type AssistantRequest struct {
	Mode     AssistantMode `json:"mode"`
	Query    string        `json:"query"`
	Provider string        `json:"provider,omitempty"`
	Model    string        `json:"model,omitempty"`
}

// AssistantResponse is the structured assistant response payload.
type AssistantResponse struct {
	HistoryID    uuid.UUID       `json:"history_id"`
	Mode         AssistantMode   `json:"mode"`
	Status       AssistantStatus `json:"status"`
	Query        string          `json:"query"`
	Text         string          `json:"text"`
	SQL          string          `json:"sql,omitempty"`
	Explanation  string          `json:"explanation,omitempty"`
	Warning      string          `json:"warning,omitempty"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	DurationMs   int             `json:"duration_ms"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
	CreatedAt    time.Time       `json:"created_at"`
	FinishedAt   time.Time       `json:"finished_at"`
}

// AssistantHistoryEntry is a persisted assistant query and response record.
type AssistantHistoryEntry struct {
	ID           uuid.UUID       `json:"id"`
	Mode         AssistantMode   `json:"mode"`
	QueryText    string          `json:"query_text"`
	ResponseText string          `json:"response_text"`
	SQL          string          `json:"sql"`
	Explanation  string          `json:"explanation"`
	Warning      string          `json:"warning"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	Status       AssistantStatus `json:"status"`
	DurationMs   int             `json:"duration_ms"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
	CreatedAt    time.Time       `json:"created_at"`
}

// AssistantHistoryFilter controls assistant history pagination and filtering.
type AssistantHistoryFilter struct {
	Mode    AssistantMode
	Page    int
	PerPage int
}

// AssistantParsedResponse is the parsed text/sql/warning breakdown.
type AssistantParsedResponse struct {
	SQL         string
	Explanation string
	Warning     string
}

// AssistantServiceConfig configures AssistantService dependencies.
type AssistantServiceConfig struct {
	Enabled      bool
	AIConfig     config.AIConfig
	Registry     *Registry
	Schema       *schema.CacheHolder
	HistoryStore AssistantHistoryStore
	Logger       *slog.Logger
}

// AssistantService orchestrates assistant prompt assembly, provider execution, and history persistence.
type AssistantService struct {
	enabled      bool
	aiConfig     config.AIConfig
	registry     *Registry
	schema       *schema.CacheHolder
	historyStore AssistantHistoryStore
	logger       *slog.Logger
}

type assistantCallMetadata struct {
	mode         AssistantMode
	query        string
	providerName string
	model        string
	startedAt    time.Time
}

type preparedAssistantCall struct {
	meta     assistantCallMetadata
	provider Provider
	request  GenerateTextRequest
}

const (
	assistantSchemaContextMaxChars = 6000
	assistantHistoryWriteTimeout   = 5 * time.Second
	streamReadBufferSize           = 1024
)

// NewAssistantService creates a dashboard AI assistant service.
func NewAssistantService(cfg AssistantServiceConfig) *AssistantService {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &AssistantService{
		enabled:      cfg.Enabled,
		aiConfig:     cfg.AIConfig,
		registry:     cfg.Registry,
		schema:       cfg.Schema,
		historyStore: cfg.HistoryStore,
		logger:       logger,
	}
}

// IsValidAssistantMode reports whether mode is one of the supported assistant modes.
func IsValidAssistantMode(mode AssistantMode) bool {
	switch mode {
	case AssistantModeSQL, AssistantModeRLS, AssistantModeMigration, AssistantModeGeneral:
		return true
	default:
		return false
	}
}

// NormalizeAssistantMode normalizes unknown/empty modes to general.
func NormalizeAssistantMode(mode AssistantMode) AssistantMode {
	mode = AssistantMode(strings.TrimSpace(strings.ToLower(string(mode))))
	if !IsValidAssistantMode(mode) {
		return AssistantModeGeneral
	}
	return mode
}

// Execute handles a synchronous assistant request.
func (s *AssistantService) Execute(ctx context.Context, req AssistantRequest) (AssistantResponse, error) {
	prepared, err := s.prepareCall(req)
	if err != nil {
		return AssistantResponse{}, err
	}

	resp, err := prepared.provider.GenerateText(ctx, prepared.request)
	finishedAt := time.Now().UTC()
	if err != nil {
		s.persistHistory(ctx, prepared.meta.nonSuccessHistoryEntry(err.Error(), prepared.meta.model, AssistantStatusError, finishedAt, Usage{}))
		return AssistantResponse{}, err
	}

	response, historyEntry := prepared.meta.successResult(resp.Text, firstNonEmpty(resp.Model, prepared.meta.model), finishedAt, resp.Usage)
	s.attachStoredHistory(ctx, &response, historyEntry)
	return response, nil
}

// ExecuteStream handles an assistant request with streaming callback support.
func (s *AssistantService) ExecuteStream(ctx context.Context, req AssistantRequest, onChunk func(string) error) (AssistantResponse, error) {
	prepared, err := s.prepareCall(req)
	if err != nil {
		return AssistantResponse{}, err
	}
	if streamProvider, ok := prepared.provider.(StreamingProvider); ok {
		return s.executeStreamWithStreamingProvider(ctx, prepared.meta, streamProvider, prepared.request, onChunk)
	}
	return s.executeStreamWithSyncFallback(ctx, prepared.meta, prepared.provider, prepared.request, onChunk)
}

// TODO: Document AssistantService.executeStreamWithSyncFallback.
func (s *AssistantService) executeStreamWithSyncFallback(
	ctx context.Context,
	meta assistantCallMetadata,
	provider Provider,
	callReq GenerateTextRequest,
	onChunk func(string) error,
) (AssistantResponse, error) {
	resp, err := provider.GenerateText(ctx, callReq)
	finishedAt := time.Now().UTC()
	if err != nil {
		s.persistHistory(ctx, meta.nonSuccessHistoryEntry(err.Error(), meta.model, AssistantStatusError, finishedAt, Usage{}))
		return AssistantResponse{}, err
	}
	if onChunk != nil && resp.Text != "" {
		if err := onChunk(resp.Text); err != nil {
			s.persistHistory(
				ctx,
				meta.nonSuccessHistoryEntry(
					resp.Text,
					firstNonEmpty(resp.Model, meta.model),
					statusFromContextErr(err),
					finishedAt,
					resp.Usage,
				),
			)
			return AssistantResponse{}, err
		}
	}
	return s.finalizeStreamSuccess(ctx, meta, resp.Text, firstNonEmpty(resp.Model, meta.model), finishedAt, resp.Usage)
}

// TODO: Document AssistantService.executeStreamWithStreamingProvider.
func (s *AssistantService) executeStreamWithStreamingProvider(
	ctx context.Context,
	meta assistantCallMetadata,
	provider StreamingProvider,
	callReq GenerateTextRequest,
	onChunk func(string) error,
) (AssistantResponse, error) {
	reader, err := provider.GenerateTextStream(ctx, callReq)
	if err != nil {
		s.persistHistory(ctx, meta.nonSuccessHistoryEntry(err.Error(), meta.model, AssistantStatusError, time.Now().UTC(), Usage{}))
		return AssistantResponse{}, err
	}
	defer reader.Close()

	var fullText strings.Builder
	buf := make([]byte, streamReadBufferSize)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			fullText.WriteString(chunk)
			if onChunk != nil {
				if err := onChunk(chunk); err != nil {
					s.persistHistory(
						ctx,
						meta.nonSuccessHistoryEntry(
							fullText.String(),
							meta.model,
							statusFromContextErr(err),
							time.Now().UTC(),
							Usage{},
						),
					)
					return AssistantResponse{}, err
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			s.persistHistory(
				ctx,
				meta.nonSuccessHistoryEntry(
					fullText.String(),
					meta.model,
					statusFromContextErr(readErr),
					time.Now().UTC(),
					Usage{},
				),
			)
			return AssistantResponse{}, readErr
		}
	}
	return s.finalizeStreamSuccess(ctx, meta, fullText.String(), meta.model, time.Now().UTC(), Usage{})
}

func (s *AssistantService) finalizeStreamSuccess(
	ctx context.Context,
	meta assistantCallMetadata,
	text string,
	model string,
	finishedAt time.Time,
	usage Usage,
) (AssistantResponse, error) {
	result, historyEntry := meta.successResult(text, model, finishedAt, usage)
	s.attachStoredHistory(ctx, &result, historyEntry)
	return result, nil
}

// ListHistory returns persisted assistant history rows.
func (s *AssistantService) ListHistory(ctx context.Context, filter AssistantHistoryFilter) ([]AssistantHistoryEntry, int, error) {
	if s.historyStore == nil {
		return []AssistantHistoryEntry{}, 0, nil
	}
	return s.historyStore.List(ctx, filter)
}

// TODO: Document AssistantService.prepareCall.
func (s *AssistantService) prepareCall(req AssistantRequest) (preparedAssistantCall, error) {
	startedAt := time.Now().UTC()
	if !s.enabled {
		return preparedAssistantCall{}, ErrAssistantDisabled
	}
	if s.registry == nil || len(s.aiConfig.Providers) == 0 {
		return preparedAssistantCall{}, ErrAssistantNotConfigured
	}
	if s.schema == nil || s.schema.Get() == nil {
		return preparedAssistantCall{}, ErrAssistantSchemaCacheNotReady
	}

	mode := NormalizeAssistantMode(req.Mode)
	providerName := strings.TrimSpace(req.Provider)
	if providerName == "" {
		providerName = strings.TrimSpace(s.aiConfig.DefaultProvider)
	}
	provider, model, err := ResolveProvider(s.registry, providerName, strings.TrimSpace(req.Model), s.aiConfig)
	if err != nil {
		return preparedAssistantCall{}, fmt.Errorf("resolve assistant provider: %w", err)
	}
	schemaContext := CompactSchemaContext(s.schema.Get(), assistantSchemaContextMaxChars)
	tpl := PromptForMode(mode)
	prompt := assembleAssistantUserPrompt(mode, schemaContext, req.Query)
	return preparedAssistantCall{
		meta: assistantCallMetadata{
			mode:         mode,
			query:        req.Query,
			providerName: providerName,
			model:        model,
			startedAt:    startedAt,
		},
		provider: provider,
		request: GenerateTextRequest{
			Model:        model,
			SystemPrompt: tpl.SystemPrompt,
			Messages: []Message{{
				Role:    "user",
				Content: TextContent(prompt),
			}},
		},
	}, nil
}

func (s *AssistantService) persistHistory(ctx context.Context, entry AssistantHistoryEntry) (AssistantHistoryEntry, bool) {
	if s.historyStore == nil {
		return AssistantHistoryEntry{}, false
	}
	writeCtx, cancel := assistantHistoryWriteContext(ctx)
	defer cancel()

	stored, err := s.historyStore.Insert(writeCtx, entry)
	if err != nil {
		s.logger.Warn("assistant history insert failed", "error", err)
		return AssistantHistoryEntry{}, false
	}
	return stored, true
}

func (s *AssistantService) attachStoredHistory(ctx context.Context, response *AssistantResponse, historyEntry AssistantHistoryEntry) {
	stored, ok := s.persistHistory(ctx, historyEntry)
	if ok {
		response.HistoryID = stored.ID
		response.CreatedAt = stored.CreatedAt
	}
}

func assistantHistoryWriteContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), assistantHistoryWriteTimeout)
	}
	return context.WithTimeout(context.WithoutCancel(ctx), assistantHistoryWriteTimeout)
}

func statusFromContextErr(err error) AssistantStatus {
	if errors.Is(err, context.Canceled) {
		return AssistantStatusCancelled
	}
	return AssistantStatusError
}

func (meta assistantCallMetadata) durationMs(finishedAt time.Time) int {
	return int(finishedAt.Sub(meta.startedAt).Milliseconds())
}

// TODO: Document assistantCallMetadata.successResult.
func (meta assistantCallMetadata) successResult(text, model string, finishedAt time.Time, usage Usage) (AssistantResponse, AssistantHistoryEntry) {
	durationMs := meta.durationMs(finishedAt)
	parsed := ParseAssistantResponseText(text)
	response := AssistantResponse{
		Mode:         meta.mode,
		Status:       AssistantStatusSuccess,
		Query:        meta.query,
		Text:         text,
		SQL:          parsed.SQL,
		Explanation:  parsed.Explanation,
		Warning:      parsed.Warning,
		Provider:     meta.providerName,
		Model:        model,
		DurationMs:   durationMs,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		CreatedAt:    meta.startedAt,
		FinishedAt:   finishedAt,
	}
	historyEntry := AssistantHistoryEntry{
		Mode:         meta.mode,
		QueryText:    meta.query,
		ResponseText: text,
		SQL:          parsed.SQL,
		Explanation:  parsed.Explanation,
		Warning:      parsed.Warning,
		Provider:     meta.providerName,
		Model:        model,
		Status:       AssistantStatusSuccess,
		DurationMs:   durationMs,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
	}
	return response, historyEntry
}

// TODO: Document assistantCallMetadata.nonSuccessHistoryEntry.
func (meta assistantCallMetadata) nonSuccessHistoryEntry(
	responseText, model string,
	status AssistantStatus,
	finishedAt time.Time,
	usage Usage,
) AssistantHistoryEntry {
	return AssistantHistoryEntry{
		Mode:         meta.mode,
		QueryText:    meta.query,
		ResponseText: responseText,
		Provider:     meta.providerName,
		Model:        model,
		Status:       status,
		DurationMs:   meta.durationMs(finishedAt),
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func assembleAssistantUserPrompt(mode AssistantMode, schemaContext, query string) string {
	return strings.TrimSpace(fmt.Sprintf(`Assistant mode: %s

Schema context:
%s

User question:
%s`, mode, schemaContext, strings.TrimSpace(query)))
}
