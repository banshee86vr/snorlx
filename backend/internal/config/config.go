package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
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

// defaultSessionSecret is the placeholder shipped in env.example. It is only accepted in DEV_MODE.
const defaultSessionSecret = "change-me-in-production"

// minSessionSecretLength is the minimum length accepted outside DEV_MODE. The secret derives the
// key that encrypts stored GitHub tokens, so a short value weakens that protection.
const minSessionSecretLength = 32

// Config holds all application configuration
type Config struct {
	// Server
	Port        string
	LogLevel    string
	FrontendURL string
	// CookieSecure sets the Secure flag on auth cookies. Defaults to true when FrontendURL is https.
	CookieSecure bool
	// TrustedProxyCIDRs lists the networks whose X-Forwarded-For / X-Real-IP headers are honoured.
	// Empty means the direct peer address is always used.
	TrustedProxyCIDRs []*net.IPNet

	// Storage
	StorageMode StorageMode
	DatabaseURL string

	// GitHub OAuth App (for user authentication)
	GitHubClientID     string
	GitHubClientSecret string

	// GitHub base URL (optional)
	GitHubBaseURL string

	// GitHub Webhooks (optional)
	GitHubWebhookSecret string

	// Login allowlist. When both are empty every GitHub account may sign in.
	AllowedGitHubUsers []string
	AllowedGitHubOrgs  []string

	// Session secret: derives the key that encrypts stored GitHub tokens at rest.
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

	isDevMode := os.Getenv("DEV_MODE") == "true"

	frontendURL, err := normalizeFrontendURL(getEnv("FRONTEND_URL", "http://localhost:5173"), isDevMode)
	if err != nil {
		return nil, err
	}

	cookieSecure := strings.HasPrefix(frontendURL, "https://")
	if raw := os.Getenv("COOKIE_SECURE"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("COOKIE_SECURE must be true or false, got %q", raw)
		}
		cookieSecure = parsed
	}

	trustedProxies, err := parseCIDRList(os.Getenv("TRUSTED_PROXY_CIDRS"))
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:                getEnv("PORT", "8080"),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		FrontendURL:         frontendURL,
		CookieSecure:        cookieSecure,
		TrustedProxyCIDRs:   trustedProxies,
		StorageMode:         storageMode,
		DatabaseURL:         strings.TrimSpace(os.Getenv("DATABASE_URL")),
		GitHubClientID:      os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:  os.Getenv("GITHUB_CLIENT_SECRET"),
		GitHubBaseURL:       os.Getenv("GITHUB_BASE_URL"),
		GitHubWebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		AllowedGitHubUsers:  splitList(os.Getenv("ALLOWED_GITHUB_USERS")),
		AllowedGitHubOrgs:   splitList(os.Getenv("ALLOWED_GITHUB_ORGS")),
		SessionSecret:       getEnv("SESSION_SECRET", defaultSessionSecret),
		SyncLimit:           syncLimit,
		SyncRepos:           splitList(os.Getenv("SYNC_REPOS")),
	}

	if !isDevMode {
		if cfg.SessionSecret == "" || cfg.SessionSecret == defaultSessionSecret {
			return nil, errors.New("SESSION_SECRET must be set to a secure random value in production (not the default)")
		}
		if len(cfg.SessionSecret) < minSessionSecretLength {
			return nil, fmt.Errorf("SESSION_SECRET must be at least %d characters (generate with: openssl rand -base64 32)", minSessionSecretLength)
		}
		if cfg.StorageMode == StorageModeDatabase && cfg.DatabaseURL == "" {
			return nil, errors.New("DATABASE_URL is required when STORAGE_MODE=database")
		}
	}

	// Validate required fields for OAuth
	if cfg.GitHubClientID == "" || cfg.GitHubClientSecret == "" {
		// In development mode, allow running without GitHub OAuth credentials
		if isDevMode {
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

// LoginRestricted reports whether an allowlist of users or organizations is configured.
func (c *Config) LoginRestricted() bool {
	return len(c.AllowedGitHubUsers) > 0 || len(c.AllowedGitHubOrgs) > 0
}

// normalizeFrontendURL requires an absolute http(s) origin and returns it as scheme://host so it
// compares byte for byte with the browser Origin header. Outside DEV_MODE a plain http URL is only
// accepted for loopback hosts, because FRONTEND_URL drives CORS, CSRF, WebSocket origin checks and
// the Secure cookie default.
func normalizeFrontendURL(raw string, isDevMode bool) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("FRONTEND_URL must be an absolute URL, got %q", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("FRONTEND_URL scheme must be http or https, got %q", u.Scheme)
	}
	hasPath := u.Path != "" && u.Path != "/"
	if hasPath || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("FRONTEND_URL must be an origin without path, query, fragment or credentials, got %q", raw)
	}
	if !isDevMode && u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		return "", fmt.Errorf("FRONTEND_URL %q uses http on a non-loopback host; use https in production or set DEV_MODE=true", raw)
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func parseCIDRList(raw string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, entry := range splitList(raw) {
		_, network, err := net.ParseCIDR(entry)
		if err != nil {
			// Accept a bare IP as a /32 or /128 network.
			ip := net.ParseIP(entry)
			if ip == nil {
				return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS entry %q is not a CIDR or IP", entry)
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			network = &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
		}
		nets = append(nets, network)
	}
	return nets, nil
}

func splitList(raw string) []string {
	var out []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
