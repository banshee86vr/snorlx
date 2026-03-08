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
	"snorlx/backend/internal/storage"
	"snorlx/backend/internal/websocket"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load .env file if it exists (try current dir, then parent for monorepo setup)
	_ = godotenv.Load()       // ./backend/.env
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

	log.Info().Msg("Starting Snorlx CI/CD Dashboard")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Initialize storage based on STORAGE_MODE
	storageMode := storage.StorageMode(cfg.StorageMode)
	store, err := storage.NewStorage(storageMode, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize storage")
	}
	defer storage.CloseStorage()

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

	// Initialize handlers
	h := handlers.New(cfg, store, ghClient, wsHub)

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Global rate limit: 300 requests per minute per IP
	r.Use(httprate.LimitByIP(300, time.Minute))

	// Security headers for all responses
	r.Use(securityHeadersMiddleware)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.FrontendURL},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

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
			// CSRF protection: validate Origin header on state-changing requests
			r.Use(csrfMiddleware(cfg.FrontendURL))

			// Organizations
			r.Get("/organizations", h.ListOrganizations)
			r.Get("/organizations/{id}", h.GetOrganization)

			// Repositories
			r.Get("/repositories", h.ListRepositories)
			r.Get("/repositories/{id}", h.GetRepository)
			r.Post("/repositories/sync", h.SyncRepositories)

			// Workflows
			r.Get("/workflows", h.ListWorkflows)
			r.Get("/workflows/{id}", h.GetWorkflow)
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

			// DevOps Metrics
			r.Route("/metrics", func(r chi.Router) {
				r.Get("/devops", h.GetDevOpsMetrics)
				r.Get("/devops/deployment-frequency", h.GetDeploymentFrequency)
				r.Get("/devops/lead-time", h.GetLeadTime)
				r.Get("/devops/change-failure-rate", h.GetChangeFailureRate)
				r.Get("/devops/mttr", h.GetMTTR)
			})

			// Dashboard
			r.Get("/dashboard/summary", h.GetDashboardSummary)
			r.Get("/dashboard/trends", h.GetTrends)

		})
	})

	// Create server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited properly")
}

// securityHeadersMiddleware sets defensive HTTP headers on every response.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
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
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
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
