// Package cli Provides wiring functions that initialize and configure various subsystems including AI edge callbacks, email delivery, domain management, tenant services, and backup infrastructure.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/emailtemplates"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/tenant"
)

// wireAIEdgeCallbacks sets up AI callbacks on the edge function pool for text generation, prompt rendering, and document parsing by integrating with the AI registry and prompt caching services.
func wireAIEdgeCallbacks(edgePool *edgefunc.Pool, reg *ai.Registry, aiCfg config.AIConfig, promptCache *ai.PromptCache, promptStore *ai.PgPromptStore) {
	edgePool.SetAIGenerate(func(callCtx context.Context, messages []map[string]any, opts map[string]any) (string, error) {
		providerName, _ := opts["provider"].(string)
		model, _ := opts["model"].(string)
		provider, resolvedModel, err := ai.ResolveProvider(reg, providerName, model, aiCfg)
		if err != nil {
			return "", err
		}
		req := ai.GenerateTextRequest{Model: resolvedModel}
		if sp, ok := opts["systemPrompt"].(string); ok {
			req.SystemPrompt = sp
		}
		if mt, ok := opts["maxTokens"]; ok {
			switch v := mt.(type) {
			case int:
				req.MaxTokens = v
			case float64:
				req.MaxTokens = int(v)
			}
		}
		for _, m := range messages {
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			req.Messages = append(req.Messages, ai.Message{
				Role:    role,
				Content: ai.TextContent(content),
			})
		}
		resp, err := provider.GenerateText(callCtx, req)
		if err != nil {
			return "", err
		}
		return resp.Text, nil
	})

	edgePool.SetAIRenderPrompt(func(callCtx context.Context, name string, vars map[string]any) (string, error) {
		rendered, _, err := promptCache.GetOrRender(callCtx, name, vars, promptStore)
		return rendered, err
	})

	edgePool.SetAIParseDocument(func(callCtx context.Context, url string, opts map[string]any) (map[string]any, error) {
		req := ai.ParseDocumentRequest{URL: url}
		if v, ok := opts["provider"].(string); ok {
			req.Provider = v
		}
		if v, ok := opts["model"].(string); ok {
			req.Model = v
		}
		if v, ok := opts["prompt"].(string); ok {
			req.Prompt = v
		}
		if v, ok := opts["schema"].(map[string]any); ok {
			req.Schema = v
		}
		return ai.ParseDocument(callCtx, req, nil, reg)
	})
}

// wireAIEmbedding sets up the semantic search embedding function if an embedding-capable provider is available.
func wireAIEmbedding(srv *server.Server, reg *ai.Registry, aiCfg config.AIConfig, logger *slog.Logger) {
	embProviderName := aiCfg.EmbeddingProvider
	if embProviderName == "" {
		embProviderName = aiCfg.DefaultProvider
	}
	if embProviderName == "" {
		return
	}
	p, err := reg.Get(embProviderName)
	if err != nil {
		return
	}
	ep, ok := p.(ai.EmbeddingProvider)
	if !ok {
		return
	}
	embModel := aiCfg.EmbeddingModel
	if embModel == "" {
		if pc, ok := aiCfg.Providers[embProviderName]; ok && pc.DefaultModel != "" {
			embModel = pc.DefaultModel
		} else {
			embModel = aiCfg.DefaultModel
		}
	}
	finalEP := ep
	finalModel := embModel
	if configuredDim, ok := aiCfg.EmbeddingDimension(embProviderName, finalModel); ok {
		srv.SetEmbeddingConfiguredDimension(configuredDim)
	}
	srv.SetEmbedder(func(ctx context.Context, texts []string) ([][]float64, error) {
		resp, err := finalEP.GenerateEmbedding(ctx, ai.EmbeddingRequest{
			Model: finalModel,
			Input: texts,
		})
		if err != nil {
			return nil, err
		}
		return resp.Embeddings, nil
	})
	logger.Info("semantic search enabled", "provider", embProviderName, "model", finalModel)
}

// TODO: Document wireDashboardAIAssistant.
func wireDashboardAIAssistant(
	srv *server.Server,
	cfg *config.Config,
	reg *ai.Registry,
	schemaCache *schema.CacheHolder,
	historyStore ai.AssistantHistoryStore,
	logger *slog.Logger,
) {
	assistantSvc := buildDashboardAIAssistantService(cfg, reg, schemaCache, historyStore, logger)
	if assistantSvc == nil {
		return
	}
	srv.SetAIAssistantService(assistantSvc)
	if reg == nil || len(cfg.AI.Providers) == 0 {
		logger.Info("dashboard AI assistant enabled without configured AI providers")
		return
	}
	logger.Info("dashboard AI assistant enabled")
}

// TODO: Document buildDashboardAIAssistantService.
func buildDashboardAIAssistantService(
	cfg *config.Config,
	reg *ai.Registry,
	schemaCache *schema.CacheHolder,
	historyStore ai.AssistantHistoryStore,
	logger *slog.Logger,
) *ai.AssistantService {
	if cfg == nil || !cfg.DashboardAI.Enabled {
		return nil
	}
	return ai.NewAssistantService(ai.AssistantServiceConfig{
		Enabled:      cfg.DashboardAI.Enabled,
		AIConfig:     cfg.AI,
		Registry:     reg,
		Schema:       schemaCache,
		HistoryStore: historyStore,
		Logger:       logger,
	})
}

// wireEdgeEmailBridge connects the edge function email send callback to the mailer and template service.
func wireEdgeEmailBridge(edgePool *edgefunc.Pool, ms mailer.Mailer, emailCfg config.EmailConfig, etSvc *emailtemplates.Service) {
	edgePool.SetEmailSend(func(ctx context.Context, to []string, subject, html, text, templateKey string, variables map[string]string, from string) (int, error) {
		// Enforce from-address whitelist.
		resolvedFrom := from
		if resolvedFrom == "" {
			resolvedFrom = emailCfg.From
		} else {
			allowed := emailCfg.EffectiveAllowedFrom()
			found := false
			for _, a := range allowed {
				if strings.EqualFold(a, resolvedFrom) {
					found = true
					break
				}
			}
			if !found {
				return 0, fmt.Errorf("from address not allowed: %s", resolvedFrom)
			}
		}

		// Enforce recipient cap.
		maxRecip := emailCfg.Policy.EffectiveMaxRecipients()
		if len(to) > maxRecip {
			return 0, fmt.Errorf("too many recipients: %d (max %d)", len(to), maxRecip)
		}

		// Resolve content (template or direct).
		var subj, htmlBody, textBody string
		if templateKey != "" && etSvc != nil {
			vars := variables
			if vars == nil {
				vars = map[string]string{}
			}
			rendered, err := etSvc.Render(ctx, templateKey, vars)
			if err != nil {
				return 0, err
			}
			subj = rendered.Subject
			htmlBody = rendered.HTML
			textBody = rendered.Text
			if subj == "" {
				subj = subject
			}
		} else {
			subj = subject
			htmlBody = html
			textBody = text
		}

		sent := 0
		for _, addr := range to {
			msg := &mailer.Message{
				To:      addr,
				Subject: subj,
				HTML:    htmlBody,
				Text:    textBody,
				From:    resolvedFrom,
			}
			if err := ms.Send(ctx, msg); err != nil {
				return sent, fmt.Errorf("send to %s: %w", addr, err)
			}
			sent++
		}
		return sent, nil
	})
}

// wireJobDomainHandlers registers domain-related job handlers and schedules.
func wireJobDomainHandlers(ctx context.Context, srv *server.Server, jobSvc *jobs.Service, logger *slog.Logger) {
	dm := srv.DomainManager()
	if dm == nil {
		return
	}

	jobSvc.RegisterHandler(server.JobTypeDomainDNSVerify, server.DomainDNSVerifyHandler(dm, server.NewNetDNSResolver(), jobSvc, logger))
	if cm := srv.CertManager(); cm != nil {
		jobSvc.RegisterHandler(server.JobTypeDomainCertProvision, server.DomainCertProvisionHandler(dm, cm, logger))
		jobSvc.RegisterHandler(server.JobTypeDomainCertRevoke, server.DomainCertRevokeHandler(cm, logger))
		jobSvc.RegisterHandler(server.JobTypeDomainCertRenew, server.DomainCertRenewHandler(dm, cm, logger))
		if err := server.RegisterDomainCertRenewSchedule(ctx, jobSvc); err != nil {
			logger.Warn("failed to register domain cert renewal schedule", "error", err)
		}
	}
	if rl, ok := dm.(server.DomainRouteLister); ok {
		jobSvc.RegisterHandler(server.JobTypeDomainRouteSync, server.DomainRouteSyncHandler(srv, rl, logger))
		if err := server.RegisterDomainRouteSyncSchedule(ctx, jobSvc); err != nil {
			logger.Warn("failed to register domain route sync schedule", "error", err)
		}
	}
	if tr, ok := dm.(server.DomainTombstoneReaper); ok {
		jobSvc.RegisterHandler(server.JobTypeDomainTombstoneReap, server.DomainTombstoneReapHandler(tr, logger))
		if err := server.RegisterDomainTombstoneReapSchedule(ctx, jobSvc); err != nil {
			logger.Warn("failed to register domain tombstone reap schedule", "error", err)
		}
	}
	if hc, ok := dm.(server.DomainHealthChecker); ok {
		if cm := srv.CertManager(); cm != nil {
			jobSvc.RegisterHandler(server.JobTypeDomainHealthCheck, server.DomainHealthCheckHandler(hc, cm, logger))
			if err := server.RegisterDomainHealthCheckSchedule(ctx, jobSvc); err != nil {
				logger.Warn("failed to register domain health check schedule", "error", err)
			}
		}
	}
	if rv, ok := dm.(server.DomainReverifier); ok {
		jobSvc.RegisterHandler(server.JobTypeDomainReverify, server.DomainReverifyHandler(rv, dm, server.NewNetDNSResolver(), logger))
		if err := server.RegisterDomainReverifySchedule(ctx, jobSvc); err != nil {
			logger.Warn("failed to register domain reverify schedule", "error", err)
		}
	}

	if err := srv.LoadRouteTable(ctx, logger); err != nil {
		logger.Warn("initial route table load failed", "error", err)
	}
}

// wireTenantServices sets up tenant usage, quota, rate limiting, breaker, and service.
func wireTenantServices(ctx context.Context, srv *server.Server, cfg *config.Config, pool *postgres.Pool, state *shutdownState, logger *slog.Logger) {
	if pool == nil {
		return
	}

	// Wire tenant usage accumulator and quota decision strategy.
	usageAcc := tenant.NewUsageAccumulator(pool.DB(), logger)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})
	go usageAcc.StartPeriodicFlush(ctx, time.Minute)

	// Wire tenant service (requires pool for tenant tables).
	state.tenantRateLimiter = tenant.NewTenantRateLimiter(time.Minute)
	srv.SetTenantRateLimiter(state.tenantRateLimiter)

	srv.SetTenantConnCounter(tenant.NewTenantConnCounter())

	tenantSvc := tenant.NewService(pool.DB(), logger)
	srv.SetTenantService(tenantSvc)

	orgStore := tenant.NewPostgresOrgStore(pool.DB(), logger)
	teamStore := tenant.NewPostgresTeamStore(pool.DB(), logger)
	orgMembershipStore := tenant.NewPostgresOrgMembershipStore(pool.DB(), logger)
	teamMembershipStore := tenant.NewPostgresTeamMembershipStore(pool.DB(), logger)
	srv.SetOrgStore(orgStore)
	srv.SetTeamStore(teamStore)
	srv.SetOrgMembershipStore(orgMembershipStore)
	srv.SetTeamMembershipStore(teamMembershipStore)
	srv.SetPermissionResolver(tenant.NewPermissionResolver(tenantSvc, orgMembershipStore, teamMembershipStore, teamStore))

	logger.Info("tenant service enabled")

	// Initialize per-tenant circuit breaker tracker with default config.
	state.tenantBreaker = tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{}, nil)
	if err := state.tenantBreaker.Restore(ctx, pool.DB()); err != nil {
		logger.Warn("failed to restore tenant breaker state", "error", err)
	}
	srv.SetTenantBreakerTracker(state.tenantBreaker)

	// Periodic breaker snapshot.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := state.tenantBreaker.Snapshot(ctx, pool.DB()); err != nil {
					logger.Warn("failed to snapshot tenant breaker state", "error", err)
				}
			}
		}
	}()
	logger.Info("tenant breaker tracker enabled")
}

// wireBackupServices sets up backup engine, PITR, and associated schedulers.
