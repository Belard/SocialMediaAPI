package config

import "os"

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
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
