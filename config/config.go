package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL          string
	JWTSecret            []byte
	Port                 string
	BaseURL              string
	UploadDir            string
	MaxUploadSize        int64
	MaxImageUploadSize   int64
	MaxVideoUploadSize   int64
	FacebookAppID        string
	FacebookAppSecret    string
	FacebookRedirectURI  string
	InstagramAppID       string
	InstagramAppSecret   string
	InstagramRedirectURI string
	FacebookVersion      string
	InstagramVersion     string
	TikTokClientKey      string
	TikTokClientSecret   string
	TikTokRedirectURI    string
	TwitterClientID      string
	TwitterClientSecret  string
	TwitterRedirectURI   string
	YouTubeClientID      string
	YouTubeClientSecret  string
	YouTubeRedirectURI   string
	TokenEncryptionKey   []byte
	TLSEnabled           bool
	TLSCertFile          string
	TLSKeyFile           string
	MediaSigningKey      []byte
	MediaURLExpiry       time.Duration

	// CORS
	CORSAllowedOrigins []string // Comma-separated list via CORS_ALLOWED_ORIGINS env var

	// Rate limiting
	RateLimitRPS         float64       // Sustained requests per second (global, per IP)
	RateLimitBurst       float64       // Max burst capacity (global, per IP)
	AuthRateLimitRPS     float64       // Sustained RPS for auth endpoints (login/register)
	AuthRateLimitBurst   float64       // Burst capacity for auth endpoints

	// Environment
	Env                  string        // "production", "staging", or "development" (default)
}

// insecureDefaults are the hard-coded fallback values that ship with the source
// code. If any secret still matches one of these in production, the server
// refuses to start.
var insecureDefaults = map[string]bool{
	"your-secret-key-change-in-production":                true,
	"your-secret-token-encryption-key-change-in-production": true,
}

func Load() *Config {
	cfg := &Config{
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/multiplatform?sslmode=disable"),
		JWTSecret:            []byte(getEnv("JWT_SECRET", "your-secret-key-change-in-production")),
		Port:                 getEnv("PORT", "8080"),
		BaseURL:              getEnv("BASE_URL", "http://localhost:8080"),
		UploadDir:            getEnv("UPLOAD_DIR", "./uploads"),
		MaxUploadSize:        100 << 20,                           // 100 MB (overall form limit)
		MaxImageUploadSize:   10 << 20,                            // 10 MB
		MaxVideoUploadSize:   100 << 20,                           // 100 MB
		FacebookAppID:        getEnv("FACEBOOK_APP_ID", ""),       //ADD LATER
		FacebookAppSecret:    getEnv("FACEBOOK_APP_SECRET", ""),   //ADD LATER
		FacebookRedirectURI:  getEnv("FACEBOOK_REDIRECT_URI", ""), //ADD LATER
		InstagramAppID:       getEnv("INSTAGRAM_APP_ID", getEnv("FACEBOOK_APP_ID", "")),
		InstagramAppSecret:   getEnv("INSTAGRAM_APP_SECRET", getEnv("FACEBOOK_APP_SECRET", "")),
		InstagramRedirectURI: getEnv("INSTAGRAM_REDIRECT_URI", ""),
		FacebookVersion:      getEnv("FACEBOOK_VERSION", "v25.0"),
		InstagramVersion:     getEnv("INSTAGRAM_VERSION", "v25.0"),
		TikTokClientKey:      getEnv("TIKTOK_CLIENT_KEY", ""),
		TikTokClientSecret:   getEnv("TIKTOK_CLIENT_SECRET", ""),
		TikTokRedirectURI:    getEnv("TIKTOK_REDIRECT_URI", ""),
		TwitterClientID:      getEnv("TWITTER_CLIENT_ID", ""),
		TwitterClientSecret:  getEnv("TWITTER_CLIENT_SECRET", ""),
		TwitterRedirectURI:   getEnv("TWITTER_REDIRECT_URI", ""),
		YouTubeClientID:      getEnv("YOUTUBE_CLIENT_ID", ""),
		YouTubeClientSecret:  getEnv("YOUTUBE_CLIENT_SECRET", ""),
		YouTubeRedirectURI:   getEnv("YOUTUBE_REDIRECT_URI", ""),
		TokenEncryptionKey:   []byte(getEnv("TOKEN_ENCRYPTION_KEY", "your-secret-token-encryption-key-change-in-production")),
		TLSEnabled:           getEnv("TLS_ENABLED", "false") == "true",
		TLSCertFile:          getEnv("TLS_CERT_FILE", "./certs/server.crt"),
		TLSKeyFile:           getEnv("TLS_KEY_FILE", "./certs/server.key"),
		MediaSigningKey:      []byte(getEnv("MEDIA_SIGNING_KEY", getEnv("JWT_SECRET", "your-secret-key-change-in-production"))),
		MediaURLExpiry:       getEnvDuration("MEDIA_URL_EXPIRY_HOURS", 1),

		CORSAllowedOrigins: getEnvList("CORS_ALLOWED_ORIGINS", nil),

		RateLimitRPS:       getEnvFloat("RATE_LIMIT_RPS", 10),
		RateLimitBurst:     getEnvFloat("RATE_LIMIT_BURST", 20),
		AuthRateLimitRPS:   getEnvFloat("AUTH_RATE_LIMIT_RPS", 1),
		AuthRateLimitBurst: getEnvFloat("AUTH_RATE_LIMIT_BURST", 5),

		Env: strings.ToLower(getEnv("GO_ENV", "development")),
	}

	cfg.validateSecrets()
	return cfg
}

// ValidateSecret describes a single secret validation result.
type SecretIssue struct {
	Name    string // env var name (e.g. "JWT_SECRET")
	Message string // human-readable description of the problem
}

// AuditSecrets checks all critical secrets and returns a list of issues found.
// This is the public API that callers (e.g. main.go) use to decide whether to
// warn or fatally exit, using whatever logger they prefer.
func (c *Config) AuditSecrets() []SecretIssue {
	type secretCheck struct {
		name  string
		value string
	}

	checks := []secretCheck{
		{"JWT_SECRET", string(c.JWTSecret)},
		{"TOKEN_ENCRYPTION_KEY", string(c.TokenEncryptionKey)},
		{"MEDIA_SIGNING_KEY", string(c.MediaSigningKey)},
	}

	var issues []SecretIssue
	for _, s := range checks {
		if s.value == "" || insecureDefaults[s.value] {
			issues = append(issues, SecretIssue{
				Name:    s.name,
				Message: s.name + " is not set or is using the insecure default value. Set a strong, unique value via environment variable.",
			})
		} else if len(s.value) < 32 {
			issues = append(issues, SecretIssue{
				Name:    s.name,
				Message: s.name + " is shorter than 32 characters. Use at least 32 characters for adequate security.",
			})
		}
	}
	return issues
}

// validateSecrets checks that critical secrets are not left at their insecure
// defaults. In production (GO_ENV=production) this is fatal. In other
// environments it emits a loud warning so developers are aware.
// NOTE: uses stdlib log here to avoid an import cycle (utils imports config).
func (c *Config) validateSecrets() {
	issues := c.AuditSecrets()
	for _, issue := range issues {
		if c.Env == "production" || c.Env == "staging" {
			log.Fatalf("FATAL SECURITY: %s", issue.Message)
		} else {
			log.Printf("WARNING SECURITY: %s", issue.Message)
		}
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvList reads a comma-separated environment variable into a string slice.
// Leading/trailing whitespace around each element is trimmed. Empty entries are
// discarded. Returns defaultVal when the variable is unset or empty.
func getEnvList(key string, defaultVal []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return defaultVal
	}
	return out
}

// getEnvFloat reads an environment variable as a float64.
// Falls back to defaultVal when unset or invalid.
func getEnvFloat(key string, defaultVal float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return defaultVal
}

// getEnvDuration reads an environment variable as an integer number of hours
// and returns a time.Duration. Falls back to defaultHours when unset or invalid.
func getEnvDuration(key string, defaultHours int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h > 0 {
			return time.Duration(h) * time.Hour
		}
	}
	return time.Duration(defaultHours) * time.Hour
}
