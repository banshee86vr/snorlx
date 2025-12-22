package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// StorageMode defines the storage backend type
type StorageMode string

const (
	StorageModeMemory   StorageMode = "memory"
	StorageModeDatabase StorageMode = "database"
)

// Config holds all application configuration
type Config struct {
	// Server
	Port        string
	LogLevel    string
	FrontendURL string

	// Storage
	StorageMode StorageMode
	DatabaseURL string

	// GitHub OAuth App (for user authentication)
	GitHubClientID     string
	GitHubClientSecret string

	// GitHub Webhooks (optional)
	GitHubWebhookSecret string

	// Session
	SessionSecret string

	// Sync Settings
	SyncLimit int      // Maximum number of repositories to sync (0 = unlimited)
	SyncRepos []string // Specific repos to sync (empty = all repos)
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	// Parse storage mode (defaults to memory for easy startup)
	storageMode := StorageModeMemory
	if mode := strings.ToLower(getEnv("STORAGE_MODE", "memory")); mode == "database" {
		storageMode = StorageModeDatabase
	}

	// Parse sync limit
	syncLimit := 0
	if limitStr := os.Getenv("SYNC_LIMIT"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			syncLimit = limit
		}
	}

	// Parse sync repos filter
	var syncRepos []string
	if reposStr := os.Getenv("SYNC_REPOS"); reposStr != "" {
		for _, repo := range strings.Split(reposStr, ",") {
			repo = strings.TrimSpace(repo)
			if repo != "" {
				syncRepos = append(syncRepos, repo)
			}
		}
	}

	cfg := &Config{
		Port:                getEnv("PORT", "8080"),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		FrontendURL:         getEnv("FRONTEND_URL", "http://localhost:5173"),
		StorageMode:         storageMode,
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		GitHubClientID:      os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:  os.Getenv("GITHUB_CLIENT_SECRET"),
		GitHubWebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		SessionSecret:       getEnv("SESSION_SECRET", "change-me-in-production"),
		SyncLimit:           syncLimit,
		SyncRepos:           syncRepos,
	}

	// Validate required fields for OAuth
	if cfg.GitHubClientID == "" || cfg.GitHubClientSecret == "" {
		// In development mode, allow running without GitHub OAuth credentials
		if os.Getenv("DEV_MODE") == "true" {
			if cfg.GitHubClientID == "" {
				cfg.GitHubClientID = "dev-client-id"
			}
			if cfg.GitHubClientSecret == "" {
				cfg.GitHubClientSecret = "dev-client-secret"
			}
		} else {
			if cfg.GitHubClientID == "" {
				return nil, errors.New("GITHUB_CLIENT_ID is required (set DEV_MODE=true to skip)")
			}
			if cfg.GitHubClientSecret == "" {
				return nil, errors.New("GITHUB_CLIENT_SECRET is required (set DEV_MODE=true to skip)")
			}
		}
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
