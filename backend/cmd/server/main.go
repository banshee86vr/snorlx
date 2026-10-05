package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"snorlx/backend/internal/config"
	"snorlx/backend/internal/github"
	"snorlx/backend/internal/handlers"
	"snorlx/backend/internal/httpmiddleware"
	"snorlx/backend/internal/scorer"
	"snorlx/backend/internal/storage"
	"snorlx/backend/internal/tokencrypt"
	"snorlx/backend/internal/version"
	"snorlx/backend/internal/websocket"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// sessionCleanupInterval bounds the growth of the sessions table.
const sessionCleanupInterval = time.Hour

func main() {
	// Load .env file if it exists (try current dir, then parent for monorepo setup)
	_ = godotenv.Load()          // ./backend/.env
	_ = godotenv.Load("../.env") // ./.env (project root)

	// Configure zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logLevel, err := zerolog.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)

	// Pretty logging for development
	if os.Getenv("LOG_FORMAT") != "json" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	log.Info().Str("version", version.Version).Msg("Starting Snorlx CI/CD Dashboard")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Token cipher: protects stored GitHub tokens; the key is derived from SESSION_SECRET
	cipher, err := tokencrypt.New(cfg.SessionSecret)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize token encryption")
	}

	// Initialize storage based on STORAGE_MODE (fails closed when the database is unreachable)
	store, err := storage.NewStorage(storage.StorageMode(cfg.StorageMode), cfg.DatabaseURL, cipher)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize storage")
	}
	defer func() { _ = store.Close() }()

	// Run migrations (no-op for memory storage)
	if err := store.Migrate(); err != nil {
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}

	// Initialize GitHub client
	ghClient, err := github.NewClient(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize GitHub client")
	}

	// Initialize WebSocket hub
	wsHub := websocket.NewHub()
	go wsHub.Run()

	// Initialize scorer and handlers
	sc := scorer.New(ghClient)
	h := handlers.New(cfg, store, ghClient, wsHub, sc)

	// Background maintenance stops with the process
	maintenanceCtx, stopMaintenance := context.WithCancel(context.Background())
	defer stopMaintenance()
	go runSessionCleanup(maintenanceCtx, store)

	// Live poller: keeps active runs current for watching users with conditional GitHub requests
	if cfg.RunPollInterval > 0 {
		log.Info().Dur("interval", cfg.RunPollInterval).Msg("Live run poller enabled")
		go h.RunLivePoller(maintenanceCtx, cfg.RunPollInterval)
	} else {
		log.Info().Msg("Live run poller disabled (RUN_POLL_INTERVAL=0); runs update through webhooks and manual refresh")
	}

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	// Client IP from proxy headers only when the peer is a configured trusted proxy
	r.Use(httpmiddleware.TrustedRealIP(cfg.TrustedProxyCIDRs))
	r.Use(httpmiddleware.RequestLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Global rate limit: 300 requests per minute per IP
	r.Use(httprate.LimitByIP(300, time.Minute))

	// Security headers for all responses
	r.Use(securityHeadersMiddleware)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.FrontendURL},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health: liveness is process-only, readiness also checks the storage backend
	r.Get("/health", h.Health)
	r.Get("/health/ready", h.Ready)

	// WebSocket endpoint (separate from /api for proper proxy handling)
	r.Get("/ws", h.WebSocketHandler)

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Set JSON content type for all API responses
		r.Use(jsonContentTypeMiddleware)

		// Auth routes (stricter rate limit: 20 requests per minute per IP)
		r.Route("/auth", func(r chi.Router) {
			r.Use(httprate.LimitByIP(20, time.Minute))
			r.Get("/login", h.Login)
			r.Get("/callback", h.Callback)
			r.Post("/logout", h.Logout)
			r.Get("/status", h.AuthStatus)
		})

		// Webhook routes (stricter rate limit: 60 per minute per IP)
		r.With(httprate.LimitByIP(60, time.Minute)).Post("/webhooks/github", h.HandleWebhook)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(h.AuthMiddleware)
			r.Use(h.RequireWriteScope)
			// CSRF: Origin check for cookie sessions; skipped for Bearer API tokens
			r.Use(csrfMiddleware(cfg.FrontendURL))

			// Pipelines (literal path first so it is not shadowed by /runs/{id})
			r.Get("/pipelines/active", h.ListActivePipelines)

			// Organizations
			r.Get("/organizations", h.ListOrganizations)
			r.Get("/organizations/{id}", h.GetOrganization)

			// Repositories
			r.Get("/repositories", h.ListRepositories)
			r.Get("/repositories/scores", h.ListRepositoryScores)
			r.Get("/repositories/{id}", h.GetRepository)
			r.Get("/repositories/{id}/score", h.GetRepositoryScore)
			r.Post("/repositories/sync", h.SyncRepositories)
			r.Post("/repositories/{id}/sync", h.SyncRepository)
			r.Post("/repositories/backfill-deployment-runs", h.BackfillDeploymentRuns)

			// Workflows
			r.Get("/workflows", h.ListWorkflows)
			r.Get("/workflows/{id}", h.GetWorkflow)
			r.Patch("/workflows/{id}", h.UpdateWorkflow)
			r.Get("/workflows/{id}/runs", h.GetWorkflowRuns)

			// Runs
			r.Get("/runs", h.ListRuns)
			r.Get("/runs/{id}", h.GetRun)
			r.Get("/runs/{id}/jobs", h.GetRunJobs)
			r.Get("/runs/{id}/logs", h.GetRunLogs)
			r.Get("/runs/{id}/annotations", h.GetRunAnnotations)
			r.Get("/runs/{id}/workflow-definition", h.GetRunWorkflowDefinition)
			r.Post("/runs/{id}/rerun", h.RerunWorkflow)
			r.Post("/runs/{id}/cancel", h.CancelRun)

			// Jobs
			r.Get("/jobs/{id}/logs", h.GetJobLogs)

			// Dashboard
			r.Get("/dashboard/summary", h.GetDashboardSummary)
			r.Get("/dashboard/trends", h.GetTrends)

			// Personal API tokens (MCP)
			r.Get("/tokens", h.ListApiTokens)
			r.Post("/tokens", h.CreateApiToken)
			r.Delete("/tokens/{id}", h.RevokeApiToken)

		})
	})

	// Create server
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      65 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Info().Str("port", cfg.Port).Msg("Server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")
	stopMaintenance()

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited properly")
}

// runSessionCleanup deletes expired sessions periodically until ctx is cancelled.
func runSessionCleanup(ctx context.Context, store storage.Storage) {
	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if err := store.CleanExpiredSessions(cleanupCtx); err != nil {
				log.Warn().Err(err).Msg("Failed to clean expired sessions")
			}
			cancel()
		}
	}
}

// securityHeadersMiddleware sets defensive HTTP headers on every response.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// jsonContentTypeMiddleware sets Content-Type: application/json for all /api responses.
func jsonContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// csrfMiddleware rejects state-changing requests whose Origin header doesn't match
// the configured frontend URL, protecting against cross-site request forgery.
func csrfMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip Origin checks only for requests authenticated with a validated API token.
			if handlers.IsBearerAuth(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
				origin := r.Header.Get("Origin")
				// Allow requests with no Origin (e.g. same-origin direct requests, curl in dev)
				if origin != "" && !strings.EqualFold(origin, allowedOrigin) {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
