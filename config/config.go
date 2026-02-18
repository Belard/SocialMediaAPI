package config

import "os"

type Config struct {
	DatabaseURL          string
	JWTSecret            []byte
	Port                 string
	BaseURL              string
	UploadDir            string
	MaxUploadSize        int64
	FacebookAppID        string
	FacebookAppSecret    string
	FacebookRedirectURI  string
	InstagramAppID       string
	InstagramAppSecret   string
	InstagramRedirectURI string
	FacebookVersion      string
	TokenEncryptionKey   []byte
}

func Load() *Config {
	return &Config{
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/multiplatform?sslmode=disable"),
		JWTSecret:            []byte(getEnv("JWT_SECRET", "your-secret-key-change-in-production")),
		Port:                 getEnv("PORT", "8080"),
		BaseURL:              getEnv("BASE_URL", "http://localhost:8080"),
		UploadDir:            getEnv("UPLOAD_DIR", "./uploads"),
		MaxUploadSize:        10 << 20,                            // 10 MB
		FacebookAppID:        getEnv("FACEBOOK_APP_ID", ""),       //ADD LATER
		FacebookAppSecret:    getEnv("FACEBOOK_APP_SECRET", ""),   //ADD LATER
		FacebookRedirectURI:  getEnv("FACEBOOK_REDIRECT_URI", ""), //ADD LATER
		InstagramAppID:       getEnv("INSTAGRAM_APP_ID", getEnv("FACEBOOK_APP_ID", "")),
		InstagramAppSecret:   getEnv("INSTAGRAM_APP_SECRET", getEnv("FACEBOOK_APP_SECRET", "")),
		InstagramRedirectURI: getEnv("INSTAGRAM_REDIRECT_URI", ""),
		FacebookVersion:      getEnv("FACEBOOK_VERSION", "v24.0"),
		TokenEncryptionKey:   []byte(getEnv("TOKEN_ENCRYPTION_KEY", "your-secret-token-encryption-key-change-in-production")),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
