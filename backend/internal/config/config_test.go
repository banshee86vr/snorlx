package config

import (
	"testing"
	"time"
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

func TestLoad_GitHubBaseURL(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("GITHUB_BASE_URL", "https://github.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitHubBaseURL != "https://github.example.com" {
		t.Errorf("GitHubBaseURL = %q", cfg.GitHubBaseURL)
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

func setProdEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DEV_MODE", "")
	t.Setenv("GITHUB_CLIENT_ID", "some-id")
	t.Setenv("GITHUB_CLIENT_SECRET", "some-secret")
	t.Setenv("SESSION_SECRET", "secure-random-session-secret-value-long-enough")
	t.Setenv("STORAGE_MODE", "memory")
	t.Setenv("COOKIE_SECURE", "")
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
}

func TestLoad_ProductionMode_ShortSessionSecret(t *testing.T) {
	setProdEnv(t)
	t.Setenv("SESSION_SECRET", "too-short")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for short SESSION_SECRET in production")
	}
}

func TestLoad_ProductionMode_DatabaseModeRequiresURL(t *testing.T) {
	setProdEnv(t)
	t.Setenv("STORAGE_MODE", "database")
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for database mode without DATABASE_URL")
	}
}

func TestLoad_DatabaseURL_TrimsSurroundingSpace(t *testing.T) {
	setProdEnv(t)
	t.Setenv("STORAGE_MODE", "database")
	t.Setenv("DATABASE_URL", "\npostgresql://postgres:secret@db:5432/snorlx?sslmode=disable\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgresql://postgres:secret@db:5432/snorlx?sslmode=disable" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
}

func TestLoad_FrontendURL_NormalizedToOrigin(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("FRONTEND_URL", "HTTPS://Dash.Example.com/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FrontendURL != "https://dash.example.com" {
		t.Errorf("FrontendURL = %q, want normalized origin", cfg.FrontendURL)
	}
	if !cfg.CookieSecure {
		t.Error("https frontend must default CookieSecure to true")
	}
}

func TestLoad_FrontendURL_Invalid(t *testing.T) {
	for _, raw := range []string{"not-a-url", "ftp://host", "https://host/app", "https://host?x=1", "https://user:pw@host"} {
		t.Setenv("DEV_MODE", "true")
		t.Setenv("FRONTEND_URL", raw)
		if _, err := Load(); err == nil {
			t.Errorf("expected error for FRONTEND_URL %q", raw)
		}
	}
}

func TestLoad_ProductionMode_RejectsPlainHTTPOnPublicHost(t *testing.T) {
	setProdEnv(t)
	t.Setenv("FRONTEND_URL", "http://dashboard.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for http FRONTEND_URL on a public host in production")
	}

	t.Setenv("FRONTEND_URL", "http://localhost:5174")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("loopback http must be accepted: %v", err)
	}
	if cfg.CookieSecure {
		t.Error("http frontend must default CookieSecure to false")
	}
}

func TestLoad_CookieSecureOverride(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("COOKIE_SECURE", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.CookieSecure {
		t.Error("COOKIE_SECURE=true must force the Secure flag")
	}

	t.Setenv("COOKIE_SECURE", "maybe")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-boolean COOKIE_SECURE")
	}
}

func TestLoad_TrustedProxyCIDRs(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("COOKIE_SECURE", "")
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 192.168.1.5, fd00::/8")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.TrustedProxyCIDRs) != 3 {
		t.Fatalf("expected 3 networks, got %d", len(cfg.TrustedProxyCIDRs))
	}
	if ones, _ := cfg.TrustedProxyCIDRs[1].Mask.Size(); ones != 32 {
		t.Errorf("bare IPv4 must become /32, got /%d", ones)
	}

	t.Setenv("TRUSTED_PROXY_CIDRS", "nonsense")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid TRUSTED_PROXY_CIDRS")
	}
}

func TestLoad_LoginAllowlist(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
	t.Setenv("ALLOWED_GITHUB_USERS", "")
	t.Setenv("ALLOWED_GITHUB_ORGS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LoginRestricted() {
		t.Error("no allowlist must not restrict login")
	}

	t.Setenv("ALLOWED_GITHUB_ORGS", "acme, ")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.LoginRestricted() || len(cfg.AllowedGitHubOrgs) != 1 || cfg.AllowedGitHubOrgs[0] != "acme" {
		t.Errorf("unexpected allowlist: %v", cfg.AllowedGitHubOrgs)
	}
}

func TestLoad_RunPollInterval(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("TRUSTED_PROXY_CIDRS", "")

	cases := []struct {
		name    string
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{name: "default", raw: "", want: defaultRunPollInterval},
		{name: "custom", raw: "30s", want: 30 * time.Second},
		{name: "minutes", raw: "1m", want: time.Minute},
		{name: "disabled", raw: "0", want: 0},
		{name: "disabled with unit", raw: "0s", want: 0},
		{name: "too short", raw: "1s", wantErr: true},
		{name: "negative", raw: "-10s", wantErr: true},
		{name: "not a duration", raw: "soon", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RUN_POLL_INTERVAL", tc.raw)
			cfg, err := Load()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for RUN_POLL_INTERVAL=%q", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.RunPollInterval != tc.want {
				t.Errorf("RunPollInterval = %s, want %s", cfg.RunPollInterval, tc.want)
			}
		})
	}
}
