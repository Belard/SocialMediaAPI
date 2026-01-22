package config

import "os"

type Config struct {
	DatabaseURL string
	JWTSecret   []byte
	Port        string
	BaseURL     string
	UploadDir   string
	MaxUploadSize int64
}

func Load() *Config {
	return &Config{
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/multiplatform?sslmode=disable"),
		JWTSecret:     []byte(getEnv("JWT_SECRET", "your-secret-key-change-in-production")),
		Port:          getEnv("PORT", "8080"),
		BaseURL:       getEnv("BASE_URL", "http://localhost:8080"),
		UploadDir:     getEnv("UPLOAD_DIR", "./uploads"),
		MaxUploadSize: 10 << 20, // 10 MB
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}