package config

import (
	"testing"
)

func TestLoad_DevMode_DefaultCredentials(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("GITHUB_CLIENT_ID", "")
	t.Setenv("GITHUB_CLIENT_SECRET", "")
	t.Setenv("SESSION_SECRET", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error in dev mode, got: %v", err)
	}
	if cfg.GitHubClientID != "dev-client-id" {
		t.Errorf("expected dev-client-id, got %q", cfg.GitHubClientID)
	}
	if cfg.GitHubClientSecret != "dev-client-secret" {
		t.Errorf("expected dev-client-secret, got %q", cfg.GitHubClientSecret)
	}
}

func TestLoad_DevMode_PreservesExistingCredentials(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("GITHUB_CLIENT_ID", "my-real-id")
	t.Setenv("GITHUB_CLIENT_SECRET", "my-real-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitHubClientID != "my-real-id" {
		t.Errorf("expected my-real-id, got %q", cfg.GitHubClientID)
	}
}

func TestLoad_ProductionMode_MissingClientID(t *testing.T) {
	t.Setenv("DEV_MODE", "")
	t.Setenv("GITHUB_CLIENT_ID", "")
	t.Setenv("GITHUB_CLIENT_SECRET", "some-secret")
	t.Setenv("SESSION_SECRET", "secure-random-session-secret-value")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing GITHUB_CLIENT_ID in production")
	}
}

func TestLoad_ProductionMode_MissingClientSecret(t *testing.T) {
	t.Setenv("DEV_MODE", "")
	t.Setenv("GITHUB_CLIENT_ID", "some-id")
	t.Setenv("GITHUB_CLIENT_SECRET", "")
	t.Setenv("SESSION_SECRET", "secure-random-session-secret-value")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing GITHUB_CLIENT_SECRET in production")
	}
}

func TestLoad_ProductionMode_DefaultSessionSecret(t *testing.T) {
	t.Setenv("DEV_MODE", "")
	t.Setenv("GITHUB_CLIENT_ID", "some-id")
	t.Setenv("GITHUB_CLIENT_SECRET", "some-secret")
	t.Setenv("SESSION_SECRET", "change-me-in-production")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for default SESSION_SECRET in production")
	}
}

func TestLoad_ProductionMode_EmptySessionSecret(t *testing.T) {
	t.Setenv("DEV_MODE", "")
	t.Setenv("GITHUB_CLIENT_ID", "some-id")
	t.Setenv("GITHUB_CLIENT_SECRET", "some-secret")
	t.Setenv("SESSION_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for empty SESSION_SECRET in production")
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("FRONTEND_URL", "")
	t.Setenv("STORAGE_MODE", "")
	t.Setenv("SYNC_LIMIT", "")
	t.Setenv("SYNC_REPOS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %q", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default log level info, got %q", cfg.LogLevel)
	}
	if cfg.FrontendURL != "http://localhost:5173" {
		t.Errorf("expected default frontend URL, got %q", cfg.FrontendURL)
	}
	if cfg.StorageMode != StorageModeMemory {
		t.Errorf("expected memory storage mode, got %q", cfg.StorageMode)
	}
	if cfg.SyncLimit != 0 {
		t.Errorf("expected sync limit 0, got %d", cfg.SyncLimit)
	}
	if len(cfg.SyncRepos) != 0 {
		t.Errorf("expected empty sync repos, got %v", cfg.SyncRepos)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("FRONTEND_URL", "https://myapp.example.com")
	t.Setenv("STORAGE_MODE", "database")
	t.Setenv("SYNC_LIMIT", "50")
	t.Setenv("SYNC_REPOS", "owner/repo1, owner/repo2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("expected port 9090, got %q", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected debug, got %q", cfg.LogLevel)
	}
	if cfg.FrontendURL != "https://myapp.example.com" {
		t.Errorf("expected custom frontend URL, got %q", cfg.FrontendURL)
	}
	if cfg.StorageMode != StorageModeDatabase {
		t.Errorf("expected database storage mode, got %q", cfg.StorageMode)
	}
	if cfg.SyncLimit != 50 {
		t.Errorf("expected sync limit 50, got %d", cfg.SyncLimit)
	}
	if len(cfg.SyncRepos) != 2 {
		t.Fatalf("expected 2 sync repos, got %d: %v", len(cfg.SyncRepos), cfg.SyncRepos)
	}
	if cfg.SyncRepos[0] != "owner/repo1" {
		t.Errorf("expected owner/repo1, got %q", cfg.SyncRepos[0])
	}
	if cfg.SyncRepos[1] != "owner/repo2" {
		t.Errorf("expected owner/repo2, got %q", cfg.SyncRepos[1])
	}
}

func TestLoad_InvalidSyncLimit(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("SYNC_LIMIT", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Invalid sync limit should default to 0 (unlimited)
	if cfg.SyncLimit != 0 {
		t.Errorf("expected sync limit 0 for invalid value, got %d", cfg.SyncLimit)
	}
}

func TestLoad_ZeroSyncLimit(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("SYNC_LIMIT", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SyncLimit != 0 {
		t.Errorf("expected sync limit 0, got %d", cfg.SyncLimit)
	}
}

func TestLoad_SyncRepos_EmptyValues(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("SYNC_REPOS", "owner/repo1,,, owner/repo2,")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty entries should be skipped
	if len(cfg.SyncRepos) != 2 {
		t.Errorf("expected 2 sync repos after filtering empty entries, got %d: %v", len(cfg.SyncRepos), cfg.SyncRepos)
	}
}

func TestGetEnv_ExistingKey(t *testing.T) {
	t.Setenv("TEST_CONFIG_KEY", "test-value")

	if v := getEnv("TEST_CONFIG_KEY", "default"); v != "test-value" {
		t.Errorf("expected test-value, got %q", v)
	}
}

func TestGetEnv_MissingKey(t *testing.T) {
	if v := getEnv("NONEXISTENT_CONFIG_KEY_XYZ", "default-fallback"); v != "default-fallback" {
		t.Errorf("expected default-fallback, got %q", v)
	}
}

func TestLoad_StorageMode_CaseInsensitive(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("STORAGE_MODE", "DATABASE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StorageMode != StorageModeDatabase {
		t.Errorf("expected database storage mode for uppercase input, got %q", cfg.StorageMode)
	}
}
