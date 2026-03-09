package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/webhooks"
	"github.com/allyourbase/ayb/openapi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server is the main HTTP server for AYB.
type Server struct {
	cfg                 *config.Config
	router              *chi.Mux
	http                *http.Server
	logger              *slog.Logger
	schema              *schema.CacheHolder
	pool                *pgxpool.Pool
	authSvc             *auth.Service     // nil when auth disabled
	authRL              *auth.RateLimiter // nil when auth disabled
	appRL               *auth.AppRateLimiter
	adminRL             *auth.RateLimiter // admin login rate limiter
	hub                 *realtime.Hub
	webhookDispatcher   webhookDispatcher // nil when pool is nil
	jobService          *jobs.Service     // nil when jobs disabled or pool is nil
	matviewSvc          matviewAdmin      // nil when pool is nil
	emailTplSvc         emailTemplateAdmin // nil when pool is nil
	adminMu             sync.RWMutex
	adminAuth           *adminAuth // nil when admin.password not set
	startTime           time.Time
	logBuffer           *LogBuffer   // nil when not using buffered logging
	smsProvider         sms.Provider // nil when SMS disabled
	smsProviderName     string       // "twilio", "plivo", etc. — stored in messages for audit
	smsAllowedCountries []string     // country allowlist from config
	msgStore            messageStore // nil when pool is nil
}

type webhookDispatcher interface {
	Enqueue(event *realtime.Event)
	SetDeliveryStore(ds webhooks.DeliveryStore)
	StartPruner(interval, retention time.Duration)
	Close()
}

var newWebhookDispatcher = func(store webhooks.WebhookLister, logger *slog.Logger) webhookDispatcher {
	return webhooks.NewDispatcher(store, logger)
}

// New creates a new Server with middleware and routes configured.
// authSvc and storageSvc may be nil when their features are disabled.
func New(cfg *config.Config, logger *slog.Logger, schemaCache *schema.CacheHolder, pool *pgxpool.Pool, authSvc *auth.Service, storageSvc *storage.Service) *Server {
	r := chi.NewRouter()

	// Global middleware (applies to all routes including admin SPA).
	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(cfg.Server.CORSAllowedOrigins))

	hub := realtime.NewHub(logger)

	// Webhooks (always created when pool is available).
	var webhookDispatcher webhookDispatcher
	if pool != nil {
		whStore := webhooks.NewStore(pool)
		webhookDispatcher = newWebhookDispatcher(whStore, logger)
		webhookDispatcher.SetDeliveryStore(whStore)
		// When the job queue is enabled, the scheduled webhook_delivery_prune
		// job handles pruning. Only start the legacy timer-based pruner when
		// jobs are disabled (default) for backward compatibility.
		if !cfg.Jobs.Enabled {
			webhookDispatcher.StartPruner(1*time.Hour, 7*24*time.Hour)
		}
	}

	s := &Server{
		cfg:               cfg,
		router:            r,
		logger:            logger,
		schema:            schemaCache,
		pool:              pool,
		authSvc:           authSvc,
		hub:               hub,
		webhookDispatcher: webhookDispatcher,
		startTime:         time.Now(),
	}
	if authSvc != nil {
		s.appRL = auth.NewAppRateLimiter()
	}
	if pool != nil {
		s.msgStore = &pgMessageStore{pool: pool}
	}
	if cfg.Admin.Password != "" {
		s.adminAuth = newAdminAuth(cfg.Admin.Password)
	} else if pool != nil {
		logger.Warn("admin password not set — admin endpoints (SQL editor, RLS, user management) are unprotected. Set admin.password in ayb.toml for production use.")
	}

	// Admin login rate limiter (always created, independent of auth service).
	adminRateLimit := cfg.Admin.LoginRateLimit
	if adminRateLimit <= 0 {
		adminRateLimit = 20
	}
	s.adminRL = auth.NewRateLimiter(adminRateLimit, time.Minute)

	// Health check (no content-type restriction).
	r.Get("/health", s.handleHealth)

	// Favicon (prevent 404 errors in browser console).
	r.Get("/favicon.ico", handleFavicon)

	// OpenAPI spec (no auth, no content-type restriction).
	r.Get("/api/openapi.yaml", handleOpenAPISpec)

	r.Route("/api", func(r chi.Router) {
		// Admin auth endpoints (no content-type enforcement — login needs JSON, status is GET).
		r.Get("/admin/status", s.handleAdminStatus)
		r.With(s.adminRL.Middleware).Post("/admin/auth", s.handleAdminLogin)

		// Admin SQL editor and RLS policy management (admin-auth gated, requires pool).
		if pool != nil {
			logger.Info("registering admin SQL and RLS routes")
			r.Route("/admin/sql", func(r chi.Router) {
				r.Use(s.requireAdminToken)
				r.Post("/", handleAdminSQL(pool, schemaCache))
			})

			// Admin RLS policy management.
			r.Route("/admin/rls", func(r chi.Router) {
				r.Use(s.requireAdminToken)
				r.Get("/", handleListRlsPolicies(pool))
				r.Post("/", handleCreateRlsPolicy(pool))
				r.Get("/{table}", handleListRlsPolicies(pool))
				r.Get("/{table}/status", handleGetRlsStatus(pool))
				r.Post("/{table}/enable", handleEnableRls(pool))
				r.Post("/{table}/disable", handleDisableRls(pool))
				r.Delete("/{table}/{policy}", handleDeleteRlsPolicy(pool))
			})
		} else {
			logger.Warn("pool is nil, skipping admin SQL and RLS routes")
		}

		// Admin user management (admin-auth gated, requires auth to be enabled).
		if authSvc != nil {
			r.Route("/admin/users", func(r chi.Router) {
				r.Use(s.requireAdminToken)
				r.Get("/", handleAdminListUsers(authSvc))
				r.Delete("/{id}", handleAdminDeleteUser(authSvc))
			})

			// Admin API key management.
			r.Route("/admin/api-keys", func(r chi.Router) {
				r.Use(s.requireAdminToken)
				r.Get("/", handleAdminListAPIKeys(authSvc))
				r.Post("/", handleAdminCreateAPIKey(authSvc))
				r.Delete("/{id}", handleAdminRevokeAPIKey(authSvc))
			})

			// Admin app management.
			r.Route("/admin/apps", func(r chi.Router) {
				r.Use(s.requireAdminToken)
				r.Get("/", handleAdminListApps(authSvc))
				r.Post("/", handleAdminCreateApp(authSvc))
				r.Get("/{id}", handleAdminGetApp(authSvc))
				r.Put("/{id}", handleAdminUpdateApp(authSvc))
				r.Delete("/{id}", handleAdminDeleteApp(authSvc))
			})

			// Admin OAuth client management.
			r.Route("/admin/oauth/clients", func(r chi.Router) {
				r.Use(s.requireAdminToken)
				r.Get("/", handleAdminListOAuthClients(authSvc))
				r.Post("/", handleAdminCreateOAuthClient(authSvc))
				r.Get("/{clientId}", handleAdminGetOAuthClient(authSvc))
				r.Put("/{clientId}", handleAdminUpdateOAuthClient(authSvc))
				r.Delete("/{clientId}", handleAdminRevokeOAuthClient(authSvc))
				r.Post("/{clientId}/rotate-secret", handleAdminRotateOAuthClientSecret(authSvc))
			})
		}

		// Admin logs (admin-auth gated).
		r.Route("/admin/logs", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", s.handleAdminLogs)
		})

		// Admin stats (admin-auth gated).
		r.Route("/admin/stats", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", s.handleAdminStats)
		})

		// Admin secrets management (admin-auth gated, requires auth service).
		if authSvc != nil {
			r.Route("/admin/secrets", func(r chi.Router) {
				r.Use(s.requireAdminToken)
				r.Post("/rotate", s.handleAdminSecretsRotate)
			})
		}

		// Admin SMS (admin-auth gated).
		r.Route("/admin/sms", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/health", s.handleAdminSMSHealth)
			r.Get("/messages", s.handleAdminSMSMessages)
			r.With(middleware.AllowContentType("application/json")).Post("/send", s.handleAdminSMSSend)
		})

		// Admin job queue management (admin-auth gated, requires jobs service).
		// Routes are registered unconditionally; the SetJobService method
		// wires the actual service at startup when jobs.enabled = true.
		r.Route("/admin/jobs", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", s.handleJobsList)
			r.Get("/stats", s.handleJobsStats)
			r.Get("/{id}", s.handleJobsGet)
			r.Post("/{id}/retry", s.handleJobsRetry)
			r.Post("/{id}/cancel", s.handleJobsCancel)
		})

		r.Route("/admin/schedules", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", s.handleSchedulesList)
			r.Post("/", s.handleSchedulesCreate)
			r.Put("/{id}", s.handleSchedulesUpdate)
			r.Delete("/{id}", s.handleSchedulesDelete)
			r.Post("/{id}/enable", s.handleSchedulesEnable)
			r.Post("/{id}/disable", s.handleSchedulesDisable)
		})

		// Admin materialized view management (admin-auth gated).
		// Routes registered unconditionally; SetMatviewAdmin wires the service at startup.
		r.Route("/admin/matviews", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", s.handleMatviewsList)
			r.Post("/", s.handleMatviewsRegister)
			r.Get("/{id}", s.handleMatviewsGet)
			r.Put("/{id}", s.handleMatviewsUpdate)
			r.Delete("/{id}", s.handleMatviewsDelete)
			r.Post("/{id}/refresh", s.handleMatviewsRefresh)
		})

		// Admin email template management (admin-auth gated).
		// Routes registered unconditionally; SetEmailTemplateService wires the service at startup.
		r.Route("/admin/email/templates", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", s.handleEmailTemplatesList)
			r.Get("/{key}", s.handleEmailTemplatesGet)
			r.Put("/{key}", s.handleEmailTemplatesUpsert)
			r.Delete("/{key}", s.handleEmailTemplatesDelete)
			r.Patch("/{key}", s.handleEmailTemplatesPatch)
			r.Post("/{key}/preview", s.handleEmailTemplatesPreview)
		})
		r.With(s.requireAdminToken).Post("/admin/email/send", s.handleEmailSend)

		// Storage routes accept multipart/form-data, mounted outside JSON content-type enforcement.
		if storageSvc != nil {
			storageHandler := storage.NewHandler(storageSvc, logger, cfg.Storage.MaxFileSizeBytes())
			r.Route("/storage", func(r chi.Router) {
				if authSvc != nil {
					// Read operations: auth optional (supports signed URLs).
					r.Group(func(r chi.Router) {
						r.Use(auth.OptionalAuth(authSvc))
						r.Get("/{bucket}", storageHandler.HandleList)
						r.Get("/{bucket}/*", storageHandler.HandleServe)
					})
					// Write operations: admin or user auth required.
					r.Group(func(r chi.Router) {
						r.Use(s.requireAdminOrUserAuth(authSvc))
						r.Post("/{bucket}", storageHandler.HandleUpload)
						r.Delete("/{bucket}/*", storageHandler.HandleDelete)
						r.Post("/{bucket}/{name}/sign", storageHandler.HandleSign)
					})
				} else {
					r.Mount("/", storageHandler.Routes())
				}
			})
		}

		// SMS delivery webhook (Twilio sends form-encoded, not JSON).
		r.Post("/webhooks/sms/status", s.handleSMSDeliveryWebhook)

		// Auth endpoints (public, rate-limited). Token endpoint accepts form data.
		if authSvc != nil {
			authHandler := auth.NewHandler(authSvc, logger)
			// Configure OAuth providers from config.
			for name, p := range cfg.Auth.OAuth {
				if p.Enabled {
					authHandler.SetOAuthProvider(name, auth.OAuthClientConfig{
						ClientID:     p.ClientID,
						ClientSecret: p.ClientSecret,
					})
				}
			}
			if cfg.Auth.OAuthRedirectURL != "" {
				authHandler.SetOAuthRedirectURL(cfg.Auth.OAuthRedirectURL)
			}
			authHandler.SetOAuthPublisher(hub)
			if cfg.Auth.MagicLinkEnabled {
				authHandler.SetMagicLinkEnabled(true)
			}
			if cfg.Auth.SMSEnabled {
				authHandler.SetSMSEnabled(true)
			}
			rl := cfg.Auth.RateLimit
			if rl <= 0 {
				rl = 10
			}
			s.authRL = auth.NewRateLimiter(rl, time.Minute)
			r.Route("/auth", func(r chi.Router) {
				r.Use(s.authRL.Middleware)
				r.Use(middleware.AllowContentType("application/json", "application/x-www-form-urlencoded"))
				r.Mount("/", authHandler.Routes())
			})
		}

		// JSON API routes get content-type enforcement.
		r.Group(func(r chi.Router) {
			r.Use(middleware.AllowContentType("application/json"))

			if authSvc != nil {
				r.With(s.requireAdminOrUserAuth(authSvc)).Get("/schema", s.handleSchema)

				// Messaging SMS endpoints (user auth required).
				r.Route("/messaging/sms", func(r chi.Router) {
					r.Use(auth.RequireAuth(authSvc))
					r.Post("/send", s.handleMessagingSMSSend)
					r.Get("/messages", s.handleMessagingSMSList)
					r.Get("/messages/{id}", s.handleMessagingSMSGet)
				})
			} else {
				r.Get("/schema", s.handleSchema)
			}

			// Realtime SSE (handles its own auth for EventSource compatibility).
			rtHandler := realtime.NewHandler(hub, pool, authSvc, schemaCache, logger)
			r.Get("/realtime", rtHandler.ServeHTTP)

			// Webhook management (admin-only).
			if pool != nil {
				whStore := webhooks.NewStore(pool)
				whHandler := webhooks.NewHandler(whStore, whStore, logger)
				r.Route("/webhooks", func(r chi.Router) {
					r.Use(s.requireAdminToken)
					r.Mount("/", whHandler.Routes())
				})
			}

			// Mount auto-generated CRUD API.
			if pool != nil {
				apiHandler := api.NewHandler(pool, schemaCache, logger, hub, webhookDispatcher)
				if authSvc != nil {
					r.Group(func(r chi.Router) {
						// Accept either a valid admin HMAC token or a user JWT/API-key.
						r.Use(s.requireAdminOrUserAuth(authSvc))
						r.Mount("/", apiHandler.Routes())
					})
				} else {
					r.Mount("/", apiHandler.Routes())
				}
			}
		})
	})

	// Admin SPA (served from embedded UI assets).
	if cfg.Admin.Enabled {
		adminPath := cfg.Admin.Path
		if adminPath == "" {
			adminPath = "/admin"
		}
		spa := staticSPAHandler()
		// Mount under a Route group to avoid chi wildcard/redirect conflicts.
		r.Route(adminPath, func(sub chi.Router) {
			sub.Get("/", spa)
			sub.Get("/*", spa)
		})

		// OAuth consent page: served from the same SPA at /oauth/authorize.
		// The SPA detects this route and renders the consent UI.
		r.Route("/oauth", func(sub chi.Router) {
			sub.Get("/authorize", spa)
		})
	}

	return s
}

// SetLogBuffer attaches a log buffer for the /api/admin/logs endpoint.
func (s *Server) SetLogBuffer(lb *LogBuffer) {
	s.logBuffer = lb
}

// SetSMSProvider configures the SMS provider for the messaging API.
func (s *Server) SetSMSProvider(name string, p sms.Provider, allowedCountries []string) {
	s.smsProvider = p
	s.smsProviderName = name
	s.smsAllowedCountries = allowedCountries
	if s.pool != nil {
		s.msgStore = &pgMessageStore{pool: s.pool}
	}
}

// SetJobService wires the job queue service for admin API endpoints.
func (s *Server) SetJobService(svc *jobs.Service) {
	s.jobService = svc
}

// SetMatviewAdmin wires the matview admin facade for admin API endpoints.
func (s *Server) SetMatviewAdmin(svc matviewAdmin) {
	s.matviewSvc = svc
}

// SetEmailTemplateService wires the email template service for admin API endpoints.
func (s *Server) SetEmailTemplateService(svc emailTemplateAdmin) {
	s.emailTplSvc = svc
}

// Router returns the chi router for registering additional routes.
func (s *Server) Router() *chi.Mux {
	return s.router
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	s.http = &http.Server{
		Addr:              s.cfg.Address(),
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	s.logger.Info("server starting", "address", s.cfg.Address())
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// StartWithReady begins listening. It closes the ready channel once the
// listener is bound, then blocks serving requests.
func (s *Server) StartWithReady(ready chan<- struct{}) error {
	s.http = &http.Server{
		Addr:              s.cfg.Address(),
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln, err := net.Listen("tcp", s.cfg.Address())
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s.logger.Info("server starting", "address", s.cfg.Address())
	close(ready)

	if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// StartTLSWithReady begins serving TLS using the provided pre-created listener.
// The caller is responsible for creating the listener with the appropriate
// tls.Config (e.g. via certmagic or a self-signed cert for tests).
// It closes the ready channel once serving begins, then blocks until shutdown.
func (s *Server) StartTLSWithReady(ln net.Listener, ready chan<- struct{}) error {
	s.http = &http.Server{
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	s.logger.Info("server starting with TLS", "address", ln.Addr())
	close(ready)

	if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	timeout := time.Duration(s.cfg.Server.ShutdownTimeout) * time.Second
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s.logger.Info("shutting down server", "timeout", timeout)
	if s.authRL != nil {
		s.authRL.Stop()
	}
	if s.appRL != nil {
		s.appRL.Stop()
	}
	if s.adminRL != nil {
		s.adminRL.Stop()
	}
	if s.jobService != nil {
		s.jobService.Stop()
	}
	if s.webhookDispatcher != nil {
		s.webhookDispatcher.Close()
	}
	s.hub.Close()
	return s.http.Shutdown(shutdownCtx)
}

// jobsNotEnabled returns a 503 response when the job service is not running.
func jobsNotEnabled(w http.ResponseWriter) {
	httputil.WriteError(w, http.StatusServiceUnavailable, "job queue is not enabled")
}

func (s *Server) handleJobsList(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminListJobs(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleJobsGet(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminGetJob(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleJobsRetry(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminRetryJob(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleJobsCancel(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminCancelJob(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleJobsStats(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminJobStats(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesList(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminListSchedules(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesCreate(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminCreateSchedule(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesUpdate(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminUpdateSchedule(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesDelete(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminDeleteSchedule(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesEnable(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminEnableSchedule(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesDisable(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		jobsNotEnabled(w)
		return
	}
	handleAdminDisableSchedule(s.jobService).ServeHTTP(w, r)
}

// matviewsNotEnabled returns a 503 when the matview service is not wired.
func matviewsNotEnabled(w http.ResponseWriter) {
	httputil.WriteError(w, http.StatusServiceUnavailable, "materialized view management requires a database connection")
}

func (s *Server) handleMatviewsList(w http.ResponseWriter, r *http.Request) {
	if s.matviewSvc == nil {
		matviewsNotEnabled(w)
		return
	}
	handleAdminListMatviews(s.matviewSvc).ServeHTTP(w, r)
}

func (s *Server) handleMatviewsGet(w http.ResponseWriter, r *http.Request) {
	if s.matviewSvc == nil {
		matviewsNotEnabled(w)
		return
	}
	handleAdminGetMatview(s.matviewSvc).ServeHTTP(w, r)
}

func (s *Server) handleMatviewsRegister(w http.ResponseWriter, r *http.Request) {
	if s.matviewSvc == nil {
		matviewsNotEnabled(w)
		return
	}
	handleAdminRegisterMatview(s.matviewSvc).ServeHTTP(w, r)
}

func (s *Server) handleMatviewsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.matviewSvc == nil {
		matviewsNotEnabled(w)
		return
	}
	handleAdminUpdateMatview(s.matviewSvc).ServeHTTP(w, r)
}

func (s *Server) handleMatviewsDelete(w http.ResponseWriter, r *http.Request) {
	if s.matviewSvc == nil {
		matviewsNotEnabled(w)
		return
	}
	handleAdminDeleteMatview(s.matviewSvc).ServeHTTP(w, r)
}

func (s *Server) handleMatviewsRefresh(w http.ResponseWriter, r *http.Request) {
	if s.matviewSvc == nil {
		matviewsNotEnabled(w)
		return
	}
	handleAdminRefreshMatview(s.matviewSvc).ServeHTTP(w, r)
}

// emailTemplatesNotEnabled returns a 503 when the email template service is not wired.
func emailTemplatesNotEnabled(w http.ResponseWriter) {
	httputil.WriteError(w, http.StatusServiceUnavailable, "email template management requires a database connection")
}

func (s *Server) handleEmailTemplatesList(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		emailTemplatesNotEnabled(w)
		return
	}
	handleAdminListEmailTemplates(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailTemplatesGet(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		emailTemplatesNotEnabled(w)
		return
	}
	handleAdminGetEmailTemplate(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailTemplatesUpsert(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		emailTemplatesNotEnabled(w)
		return
	}
	handleAdminUpsertEmailTemplate(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailTemplatesDelete(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		emailTemplatesNotEnabled(w)
		return
	}
	handleAdminDeleteEmailTemplate(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailTemplatesPatch(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		emailTemplatesNotEnabled(w)
		return
	}
	handleAdminPatchEmailTemplate(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailTemplatesPreview(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		emailTemplatesNotEnabled(w)
		return
	}
	handleAdminPreviewEmailTemplate(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleEmailSend(w http.ResponseWriter, r *http.Request) {
	if s.emailTplSvc == nil {
		emailTemplatesNotEnabled(w)
		return
	}
	handleAdminSendEmail(s.emailTplSvc).ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	type healthResponse struct {
		Status   string `json:"status"`
		Database string `json:"database"`
	}

	if s.pool == nil {
		// No database pool — server is up but database-dependent endpoints will not work.
		httputil.WriteJSON(w, http.StatusOK, healthResponse{
			Status:   "ok",
			Database: "not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.pool.Ping(ctx); err != nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, healthResponse{
			Status:   "degraded",
			Database: "unreachable",
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, healthResponse{
		Status:   "ok",
		Database: "ok",
	})
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	// Return 204 No Content to prevent 404 errors in browser console.
	// Browsers request /favicon.ico by default; we don't have one embedded.
	w.WriteHeader(http.StatusNoContent)
}

func handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	w.Write(openapi.Spec)
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	sc := s.schema.Get()
	if sc == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sc)
}
