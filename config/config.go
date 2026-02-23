package config

import (
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
}

func Load() *Config {
	return &Config{
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
